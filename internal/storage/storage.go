package storage

import (
	"context"
	"fmt"

	"github.com/callmedenchick/callmebridge/internal/models"
)

type Storage interface {
	GetMessages(ctx context.Context, keys []string, lastEventId int64) ([]models.SseMessage, error)
	Add(ctx context.Context, key string, ttl int64, mes models.SseMessage) error
	HealthCheck() error
}

func NewStorage(storageType string, uri string) (Storage, error) {
	switch storageType {
	case "valkey", "redis":
		return NewValkeyStorage(uri)
	case "postgres":
		return NewPgStorage(uri)
	case "memory":
		return NewMemStorage(), nil
	default:
		return nil, fmt.Errorf("unsupported storage type: %s", storageType)
	}
}
