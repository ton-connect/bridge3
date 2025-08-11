package storage

import (
	"context"
	"fmt"

	"github.com/ton-connect/bridge3/internal/models"
)

type Storage interface {
	Pub(ctx context.Context, key string, ttl int64, message models.SseMessage) error
	Sub(ctx context.Context, keys []string, lastEventId int64, messageCh chan<- models.SseMessage) error
	Unsub(ctx context.Context, keys []string) error
	HealthCheck() error
}

func NewStorage(storageType string, uri string) (Storage, error) {
	switch storageType {
	case "valkey", "redis":
		return NewValkeyStorage(uri)
	case "postgres":
		return nil, fmt.Errorf("postgres storage does not support pub-sub functionality yet")
	case "memory":
		return NewMemStorage(), nil
	default:
		return nil, fmt.Errorf("unsupported storage type: %s", storageType)
	}
}
