package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/callmedenchick/callmebridge/internal/models"
	"github.com/nats-io/nats.go"
	log "github.com/sirupsen/logrus"
)

type NatsStorage struct {
	nc *nats.Conn
	js nats.JetStreamContext
}

type natsMessage struct {
	Key       string    `json:"key"`
	EventId   int64     `json:"event_id"`
	Message   []byte    `json:"message"`
	ExpiresAt time.Time `json:"expires_at"`
}

const (
	streamName     = "BRIDGE_MESSAGES"
	subjectPattern = "bridge.messages.*"
)

func NewNatsStorage(natsURL string) (*NatsStorage, error) {
	log := log.WithField("prefix", "NewNatsStorage")

	nc, err := nats.Connect(natsURL)
	if err != nil {
		log.Errorf("failed to connect to NATS: %v", err)
		return nil, err
	}

	js, err := nc.JetStream()
	if err != nil {
		log.Errorf("failed to create JetStream context: %v", err)
		nc.Close()
		return nil, err
	}

	storage := &NatsStorage{
		nc: nc,
		js: js,
	}

	if err := storage.initStream(); err != nil {
		log.Errorf("failed to initialize stream: %v", err)
		nc.Close()
		return nil, err
	}

	log.Info("NATS JetStream storage initialized successfully")
	return storage, nil
}

func (s *NatsStorage) initStream() error {
	log := log.WithField("prefix", "NatsStorage.initStream")

	// Check if stream exists
	_, err := s.js.StreamInfo(streamName)
	if err != nil {
		// Stream doesn't exist, create it
		streamConfig := &nats.StreamConfig{
			Name:      streamName,
			Subjects:  []string{subjectPattern},
			Retention: nats.LimitsPolicy,
			MaxAge:    24 * time.Hour, // TODO make configurable
			Storage:   nats.FileStorage,
		}

		_, err = s.js.AddStream(streamConfig)
		if err != nil {
			log.Errorf("failed to create stream: %v", err)
			return err
		}
		log.Info("created JetStream stream")
	} else {
		log.Info("JetStream stream already exists")
	}

	return nil
}

func (s *NatsStorage) Add(ctx context.Context, key string, ttl int64, mes models.SseMessage) error {
	log := log.WithField("prefix", "NatsStorage.Add")

	natsMsg := natsMessage{
		Key:       key,
		EventId:   mes.EventId,
		Message:   mes.Message,
		ExpiresAt: time.Now().Add(time.Duration(ttl) * time.Second),
	}

	data, err := json.Marshal(natsMsg)
	if err != nil {
		log.Errorf("failed to marshal message: %v", err)
		return err
	}

	subject := fmt.Sprintf("bridge.messages.%s", key)

	_, err = s.js.Publish(subject, data)
	if err != nil {
		log.Errorf("failed to publish message: %v", err)
		return err
	}

	log.Debugf("message added for key %s with event ID %d", key, mes.EventId)
	return nil
}

func (s *NatsStorage) GetMessages(ctx context.Context, keys []string, lastEventId int64) ([]models.SseMessage, error) {
	log := log.WithField("prefix", "NatsStorage.GetMessages")

	var results []models.SseMessage
	now := time.Now()

	for _, key := range keys {
		subject := fmt.Sprintf("bridge.messages.%s", key)

		// Use simple subscription to get messages from the stream
		sub, err := s.js.SubscribeSync(subject, nats.DeliverAll())
		if err != nil {
			log.Errorf("failed to create subscription for key %s: %v", key, err)
			continue
		}

		for {
			msg, err := sub.NextMsg(500 * time.Millisecond)
			if err != nil {
				if err == nats.ErrTimeout {
					break // TODO fix it
				}
				log.Errorf("failed to get next message for key %s: %v", key, err)
				break
			}

			var natsMsg natsMessage
			if err := json.Unmarshal(msg.Data, &natsMsg); err != nil {
				log.Errorf("failed to unmarshal message: %v", err)
				continue
			}

			// Check if message is expired
			if natsMsg.ExpiresAt.Before(now) {
				continue
			}

			// Check if event ID is greater than lastEventId
			if natsMsg.EventId <= lastEventId {
				continue
			}

			results = append(results, models.SseMessage{
				EventId: natsMsg.EventId,
				Message: natsMsg.Message,
			})
		}

		sub.Unsubscribe() // TODO fix it
	}

	log.Debugf("retrieved %d messages for keys %v", len(results), keys)
	return results, nil
}

func (s *NatsStorage) HealthCheck() error {
	log := log.WithField("prefix", "NatsStorage.HealthCheck")

	if !s.nc.IsConnected() {
		err := fmt.Errorf("NATS connection is not active")
		log.Error(err)
		return err
	}

	_, err := s.js.AccountInfo()
	if err != nil {
		log.Errorf("JetStream health check failed: %v", err)
		return err
	}

	log.Info("NATS JetStream storage is healthy")
	return nil
}

func (s *NatsStorage) Close() error {
	if s.nc != nil {
		s.nc.Close()
	}
	return nil
}
