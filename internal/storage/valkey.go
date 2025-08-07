package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"
	"github.com/ton-connect/bridge3/internal/models"
)

type ValkeyStorage struct {
	client      *redis.Client
	pubSubConn  *redis.PubSub
	subscribers map[string][]chan<- models.SseMessage
	subMutex    sync.RWMutex
}

// NewValkeyStorage creates a new Valkey storage instance
func NewValkeyStorage(valkeyURI string) (*ValkeyStorage, error) {
	log := log.WithField("prefix", "NewValkeyStorage")

	opts, err := redis.ParseURL(valkeyURI)
	if err != nil {
		log.Errorf("failed to parse Valkey URI: %v", err)
		return nil, err
	}

	rdb := redis.NewClient(opts)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = rdb.Ping(ctx).Result()
	if err != nil {
		log.Errorf("failed to connect to Valkey: %v", err)
		return nil, err
	}

	log.Info("successfully connected to Valkey")

	storage := &ValkeyStorage{
		client:      rdb,
		subscribers: make(map[string][]chan<- models.SseMessage),
	}

	return storage, nil
}

// Pub publishes a message to Redis and stores it with TTL
func (s *ValkeyStorage) Pub(ctx context.Context, key string, ttl int64, message models.SseMessage) error {
	log := log.WithField("prefix", "ValkeyStorage.Pub")

	// Publish to Redis channel
	channel := fmt.Sprintf("client:%s", key)
	messageData, err := json.Marshal(message)
	if err != nil {
		log.Errorf("failed to marshal message: %v", err)
		return err
	}

	err = s.client.Publish(ctx, channel, messageData).Err()
	if err != nil {
		log.Errorf("failed to publish message to channel %s: %v", channel, err)
		return err
	}

	// Store message with TTL as backup for offline clients
	expireTime := time.Now().Add(time.Duration(ttl) * time.Second).Unix()
	err = s.client.ZAdd(ctx, channel, redis.Z{
		Score:  float64(expireTime),
		Member: messageData,
	}).Err()

	if err != nil {
		log.Errorf("failed to store message in Valkey: %v", err)
		return err
	}

	// Set expiration on the key itself
	s.client.Expire(ctx, channel, time.Duration(ttl+60)*time.Second)

	log.Debugf("published and stored message for client %s with TTL %d seconds", key, ttl)
	return nil
}

// Sub subscribes to Redis channels for the given keys and sends historical messages after lastEventId
func (s *ValkeyStorage) Sub(ctx context.Context, keys []string, lastEventId int64, messageCh chan<- models.SseMessage) error {
	log := log.WithField("prefix", "ValkeyStorage.Sub")

	s.subMutex.Lock()
	defer s.subMutex.Unlock()

	// Add messageCh to subscribers for each key
	for _, key := range keys {
		if s.subscribers[key] == nil {
			s.subscribers[key] = make([]chan<- models.SseMessage, 0)
		}
		s.subscribers[key] = append(s.subscribers[key], messageCh)
	}

	// Send historical messages for each key
	now := time.Now().Unix()
	for _, key := range keys {
		clientKey := fmt.Sprintf("client:%s", key)

		// Remove expired messages first
		s.client.ZRemRangeByScore(ctx, clientKey, "0", fmt.Sprintf("%d", now))

		// Get all remaining messages
		messages, err := s.client.ZRange(ctx, clientKey, 0, -1).Result()
		if err != nil {
			if err != redis.Nil {
				log.Errorf("failed to get historical messages for client %s: %v", key, err)
			}
			continue // No messages for this client or error occurred
		}

		// Parse and send historical messages
		for _, msgData := range messages {
			var msg models.SseMessage
			err := json.Unmarshal([]byte(msgData), &msg)
			if err != nil {
				log.Errorf("failed to unmarshal historical message: %v", err)
				continue
			}

			// Filter by event ID - only send messages after lastEventId
			if msg.EventId > lastEventId {
				select {
				case messageCh <- msg:
				default:
					// Channel is full or closed, skip
				}
			}
		}
	}

	// Create channels list for subscription
	channels := make([]string, len(keys))
	for i, key := range keys {
		channels[i] = fmt.Sprintf("client:%s", key)
	}

	// If this is the first subscription, start the pub-sub connection
	if s.pubSubConn == nil {
		s.pubSubConn = s.client.Subscribe(ctx, channels...)
		go s.handlePubSub()
	} else {
		// Subscribe to additional channels
		err := s.pubSubConn.Subscribe(ctx, channels...)
		if err != nil {
			log.Errorf("failed to subscribe to additional channels: %v", err)
		}
	}

	log.Debugf("subscribed to channels for keys: %v", keys)
	return nil
}

// Unsub unsubscribes from Redis channels for the given keys
func (s *ValkeyStorage) Unsub(ctx context.Context, keys []string) error {
	log := log.WithField("prefix", "ValkeyStorage.Unsub")

	s.subMutex.Lock()
	defer s.subMutex.Unlock()

	channels := make([]string, 0)
	for _, key := range keys {
		channel := fmt.Sprintf("client:%s", key)
		channels = append(channels, channel)
		delete(s.subscribers, key)
	}

	if s.pubSubConn != nil {
		err := s.pubSubConn.Unsubscribe(ctx, channels...)
		if err != nil {
			log.Errorf("failed to unsubscribe from channels: %v", err)
			return err
		}
	}

	log.Debugf("unsubscribed from channels for keys: %v", keys)
	return nil
}

// handlePubSub processes incoming Redis pub-sub messages
func (s *ValkeyStorage) handlePubSub() {
	log := log.WithField("prefix", "ValkeyStorage.handlePubSub")

	for msg := range s.pubSubConn.Channel() {
		// Parse channel name to get client key
		var key string
		if len(msg.Channel) > 7 && msg.Channel[:7] == "client:" {
			key = msg.Channel[7:]
		} else {
			continue
		}

		// Parse message
		var sseMessage models.SseMessage
		err := json.Unmarshal([]byte(msg.Payload), &sseMessage)
		if err != nil {
			log.Errorf("failed to unmarshal pub-sub message: %v", err)
			continue
		}

		// Send to all subscribers for this key
		s.subMutex.RLock()
		subscribers, exists := s.subscribers[key]
		if exists {
			for _, ch := range subscribers {
				select {
				case ch <- sseMessage:
				default:
					// Channel is full or closed, skip
				}
			}
		}
		s.subMutex.RUnlock()
	}
}

// HealthCheck verifies the connection to Valkey
func (s *ValkeyStorage) HealthCheck() error {
	log := log.WithField("prefix", "ValkeyStorage.HealthCheck")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := s.client.Ping(ctx).Result()
	if err != nil {
		log.Errorf("Valkey health check failed: %v", err)
		return err
	}

	log.Info("Valkey is healthy")
	return nil
}
