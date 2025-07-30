package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/callmedenchick/callmebridge/internal/models"
	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"
)

type ValkeyStorage struct {
	client *redis.Client
}

// NewValkeyStorage creates a new Valkey storage instance
func NewValkeyStorage(valkeyURI string) (*ValkeyStorage, error) {
	log := log.WithField("prefix", "NewValkeyStorage")

	opts := &redis.Options{
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,
	}

	if valkeyURI != "" && valkeyURI != "redis://" && valkeyURI != "valkey://" {
		parsedOpts, err := redis.ParseURL(valkeyURI)
		if err != nil {
			log.Errorf("failed to parse Valkey URI: %v", err)
			return nil, err
		}
		opts = parsedOpts
	}

	rdb := redis.NewClient(opts)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := rdb.Ping(ctx).Result()
	if err != nil {
		log.Errorf("failed to connect to Valkey: %v", err)
		return nil, err
	}

	log.Info("successfully connected to Valkey")

	storage := &ValkeyStorage{
		client: rdb,
	}

	// Start cleanup worker
	go storage.worker()

	return storage, nil
}

// worker runs periodically to clean up expired messages
func (s *ValkeyStorage) worker() {
	log := log.WithField("prefix", "ValkeyStorage.worker")
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		log.Info("running cleanup task")
		ctx := context.Background()

		// Get all client keys
		keys, err := s.client.Keys(ctx, "client:*").Result()
		if err != nil {
			log.Errorf("failed to get client keys: %v", err)
			continue
		}

		for _, key := range keys {
			// Remove expired messages from each client's list
			now := time.Now().Unix()
			_, err := s.client.ZRemRangeByScore(ctx, key, "0", strconv.FormatInt(now, 10)).Result()
			if err != nil {
				log.Errorf("failed to remove expired messages for key %s: %v", key, err)
			}
		}
	}
}

// Add stores a message for a specific client with TTL
func (s *ValkeyStorage) Add(ctx context.Context, key string, ttl int64, mes models.SseMessage) error {
	log := log.WithField("prefix", "ValkeyStorage.Add")

	messageData, err := json.Marshal(mes)
	if err != nil {
		log.Errorf("failed to marshal message: %v", err)
		return err
	}

	// Use sorted set with expiration time as score
	expireTime := time.Now().Add(time.Duration(ttl) * time.Second).Unix()
	clientKey := fmt.Sprintf("client:%s", key)

	// Add message to sorted set with expiration time as score
	err = s.client.ZAdd(ctx, clientKey, redis.Z{
		Score:  float64(expireTime),
		Member: messageData,
	}).Err()

	if err != nil {
		log.Errorf("failed to add message to Valkey: %v", err)
		return err
	}

	// TODO Maybe redundant?
	// Set expiration on the key itself (as a safety net)
	s.client.Expire(ctx, clientKey, time.Duration(ttl+60)*time.Second)

	log.Debugf("added message for client %s with TTL %d seconds", key, ttl)
	return nil
}

// GetMessages retrieves messages for multiple clients with event ID filtering
func (s *ValkeyStorage) GetMessages(ctx context.Context, keys []string, lastEventId int64) ([]models.SseMessage, error) {
	log := log.WithField("prefix", "ValkeyStorage.GetMessages")

	var allMessages []models.SseMessage
	now := time.Now().Unix()

	for _, key := range keys {
		clientKey := fmt.Sprintf("client:%s", key)

		// Get all non-expired messages from sorted set
		// Remove expired messages first
		s.client.ZRemRangeByScore(ctx, clientKey, "0", strconv.FormatInt(now, 10))

		// Get all remaining messages
		messages, err := s.client.ZRange(ctx, clientKey, 0, -1).Result()
		if err != nil {
			if err == redis.Nil {
				continue // No messages for this client
			}
			log.Errorf("failed to get messages for client %s: %v", key, err)
			return nil, err
		}

		// Parse and filter messages
		for _, msgData := range messages {
			var msg models.SseMessage
			err := json.Unmarshal([]byte(msgData), &msg)
			if err != nil {
				log.Errorf("failed to unmarshal message: %v", err)
				continue
			}

			// Filter by event ID
			if msg.EventId > lastEventId {
				allMessages = append(allMessages, msg)
			}
		}
	}

	log.Debugf("retrieved %d messages for %d clients", len(allMessages), len(keys))
	return allMessages, nil
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
