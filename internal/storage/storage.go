package storage

import (
	"context"
	"fmt"
	"strings"

	"github.com/callmedenchick/callmebridge/internal/models"
)

type Storage interface {
	GetMessages(ctx context.Context, keys []string, lastEventId int64) ([]models.SseMessage, error)
	Add(ctx context.Context, key string, ttl int64, mes models.SseMessage) error
	HealthCheck() error
}

// StorageConfig holds configuration for different storage types
type StorageConfig struct {
	Type               string
	PostgresURI        string
	KafkaBrokers       []string
	KafkaTopic         string
	KafkaConsumerGroup string
}

func NewStorage(config StorageConfig) (Storage, error) {
	switch strings.ToLower(config.Type) {
	case "postgres", "pg":
		if config.PostgresURI == "" {
			return nil, fmt.Errorf("postgres URI is required for postgres storage")
		}
		return NewPgStorage(config.PostgresURI)
	case "kafka":
		if len(config.KafkaBrokers) == 0 {
			return nil, fmt.Errorf("kafka brokers are required for kafka storage")
		}
		if config.KafkaTopic == "" {
			config.KafkaTopic = "bridge-messages"
		}
		if config.KafkaConsumerGroup == "" {
			config.KafkaConsumerGroup = "bridge-consumer"
		}
		return NewKafkaStorage(config.KafkaBrokers, config.KafkaTopic, config.KafkaConsumerGroup)
	case "memory", "mem", "":
		return NewMemStorage(), nil
	default:
		return nil, fmt.Errorf("unsupported storage type: %s", config.Type)
	}
}

// Deprecated: Use NewStorage with StorageConfig instead
func NewStorageDeprecated(dbURI string) (Storage, error) {
	if dbURI != "" {
		return NewPgStorage(dbURI)
	}
	return NewMemStorage(), nil
}
