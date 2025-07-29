package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/callmedenchick/callmebridge/internal/models"
	"github.com/segmentio/kafka-go"
	log "github.com/sirupsen/logrus"
)

type KafkaMessage struct {
	EventId   int64  `json:"event_id"`
	Message   []byte `json:"message"`
	ClientId  string `json:"client_id"`
	TTL       int64  `json:"ttl"`
	Timestamp int64  `json:"timestamp"`
}

type KafkaStorage struct {
	brokers       []string
	topic         string
	consumerGroup string
	writer        *kafka.Writer
	reader        *kafka.Reader
	messages      map[string][]models.SseMessage // In-memory cache for quick access
	messageMutex  sync.RWMutex
	ctx           context.Context
}

func NewKafkaStorage(brokers []string, topic, consumerGroup string) (*KafkaStorage, error) {
	log := log.WithField("prefix", "NewKafkaStorage")

	writer := &kafka.Writer{
		Addr:         kafka.TCP(brokers...),
		Topic:        topic,
		Balancer:     &kafka.LeastBytes{},
		RequiredAcks: kafka.RequireOne,
		Async:        false,
	}

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     brokers,
		Topic:       topic,
		GroupID:     consumerGroup,
		StartOffset: kafka.LastOffset,
		MinBytes:    1,
		MaxBytes:    10e6, // 10MB
	})

	storage := &KafkaStorage{
		brokers:       brokers,
		topic:         topic,
		consumerGroup: consumerGroup,
		writer:        writer,
		reader:        reader,
		messages:      make(map[string][]models.SseMessage),
		ctx:           context.Background(),
	}

	// Start background goroutines
	go storage.consumer()
	go storage.cleaner()

	log.Info("Kafka storage initialized successfully")
	return storage, nil
}

func (s *KafkaStorage) Add(ctx context.Context, key string, ttl int64, mes models.SseMessage) error {
	log := log.WithField("prefix", "KafkaStorage.Add")

	kafkaMsg := KafkaMessage{
		EventId:   mes.EventId,
		Message:   mes.Message,
		ClientId:  key,
		TTL:       ttl,
		Timestamp: time.Now().Unix(),
	}

	msgBytes, err := json.Marshal(kafkaMsg)
	if err != nil {
		log.Errorf("failed to marshal message: %v", err)
		return err
	}

	message := kafka.Message{
		Key:   []byte(key),
		Value: msgBytes,
		Time:  time.Now(),
	}

	err = s.writer.WriteMessages(ctx, message)
	if err != nil {
		log.Errorf("failed to write message to Kafka: %v", err)
		return err
	}

	// Also add to in-memory cache for immediate availability
	s.messageMutex.Lock()
	if s.messages[key] == nil {
		s.messages[key] = make([]models.SseMessage, 0)
	}
	s.messages[key] = append(s.messages[key], mes)
	s.messageMutex.Unlock()

	log.Debugf("message added to Kafka and cache for client %s", key)
	return nil
}

func (s *KafkaStorage) GetMessages(ctx context.Context, keys []string, lastEventId int64) ([]models.SseMessage, error) {
	log := log.WithField("prefix", "KafkaStorage.GetMessages")

	s.messageMutex.RLock()
	defer s.messageMutex.RUnlock()

	var result []models.SseMessage
	for _, key := range keys {
		if messages, exists := s.messages[key]; exists {
			for _, msg := range messages {
				if msg.EventId > lastEventId {
					result = append(result, msg)
				}
			}
		}
	}

	log.Debugf("retrieved %d messages for clients %v", len(result), keys)
	return result, nil
}

func (s *KafkaStorage) HealthCheck() error {
	log := log.WithField("prefix", "KafkaStorage.HealthCheck")

	// Try to create a test connection
	conn, err := kafka.Dial("tcp", s.brokers[0])
	if err != nil {
		log.Errorf("kafka health check failed: %v", err)
		return err
	}
	defer func() {
		if closeErr := conn.Close(); closeErr != nil {
			log.Errorf("failed to close connection: %v", closeErr)
		}
	}()

	// Check if topic exists
	partitions, err := conn.ReadPartitions(s.topic)
	if err != nil {
		log.Errorf("failed to read topic partitions: %v", err)
		return err
	}

	if len(partitions) == 0 {
		return fmt.Errorf("topic %s has no partitions", s.topic)
	}

	log.Info("kafka is healthy")
	return nil
}

// consumer runs in background to consume messages from Kafka and maintain the in-memory cache
func (s *KafkaStorage) consumer() {
	log := log.WithField("prefix", "KafkaStorage.consumer")
	log.Info("starting Kafka consumer")

	for {
		select {
		case <-s.ctx.Done():
			log.Info("stopping Kafka consumer")
			return
		default:
			message, err := s.reader.FetchMessage(s.ctx)
			if err != nil {
				log.Errorf("failed to fetch message: %v", err)
				time.Sleep(time.Second)
				continue
			}

			var kafkaMsg KafkaMessage
			err = json.Unmarshal(message.Value, &kafkaMsg)
			if err != nil {
				log.Errorf("failed to unmarshal message: %v", err)
				if err := s.reader.CommitMessages(s.ctx, message); err != nil {
					log.Errorf("failed to commit message: %v", err)
				}
				continue
			}
			// Check if message is still valid (TTL)
			if time.Now().Unix() > kafkaMsg.Timestamp+kafkaMsg.TTL {
				if err := s.reader.CommitMessages(s.ctx, message); err != nil {
					log.Errorf("failed to commit message: %v", err)
				}
				continue
			}

			// Add to in-memory cache
			s.messageMutex.Lock()
			if s.messages[kafkaMsg.ClientId] == nil {
				s.messages[kafkaMsg.ClientId] = make([]models.SseMessage, 0)
			}

			sseMsg := models.SseMessage{
				EventId: kafkaMsg.EventId,
				Message: kafkaMsg.Message,
			}

			// Check if message already exists to avoid duplicates
			exists := false
			for _, existingMsg := range s.messages[kafkaMsg.ClientId] {
				if existingMsg.EventId == sseMsg.EventId {
					exists = true
					break
				}
			}

			if !exists {
				s.messages[kafkaMsg.ClientId] = append(s.messages[kafkaMsg.ClientId], sseMsg)
			}
			s.messageMutex.Unlock()

			if err := s.reader.CommitMessages(s.ctx, message); err != nil {
				log.Errorf("failed to commit message: %v", err)
			}
		}
	}
}

// cleaner runs in background to remove expired messages from the in-memory cache
func (s *KafkaStorage) cleaner() {
	log := log.WithField("prefix", "KafkaStorage.cleaner")
	log.Info("starting Kafka storage cleaner")

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			log.Info("stopping Kafka storage cleaner")
			return
		case <-ticker.C:
			s.cleanExpiredMessages()
		}
	}
}

func (s *KafkaStorage) cleanExpiredMessages() {
	log := log.WithField("prefix", "KafkaStorage.cleanExpiredMessages")

	s.messageMutex.Lock()
	defer s.messageMutex.Unlock()

	// For simplicity, we'll clean messages older than 1 hour
	// In a real implementation, you might want to store TTL info per message
	cutoffTime := time.Now().Add(-time.Hour)
	cutoffEventId := cutoffTime.Unix() * 1000 // Assuming eventId is timestamp-based

	for clientId, messages := range s.messages {
		var validMessages []models.SseMessage
		for _, msg := range messages {
			if msg.EventId > cutoffEventId {
				validMessages = append(validMessages, msg)
			}
		}

		if len(validMessages) == 0 {
			delete(s.messages, clientId)
		} else {
			s.messages[clientId] = validMessages
		}
	}

	log.Debug("expired messages cleaned from cache")
}
