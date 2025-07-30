package storage

import (
	"context"
	"strings"

	"github.com/callmedenchick/callmebridge/internal/models"
)

type Storage interface {
	GetMessages(ctx context.Context, keys []string, lastEventId int64) ([]models.SseMessage, error)
	Add(ctx context.Context, key string, ttl int64, mes models.SseMessage) error
	HealthCheck() error
}

func NewStorage(dbURI string, redisURI string) (Storage, error) {
	// Priority: Redis/Valkey first, then PostgreSQL, then in-memory
	if redisURI != "" {
		// For simplicity in PoC, use localhost:6379 if redisURI is just "redis://" or "valkey://"
		addr := "localhost:6379"
		if redisURI != "redis://" && redisURI != "valkey://" {
			// Parse the URI properly in production
			addr = strings.TrimPrefix(strings.TrimPrefix(redisURI, "redis://"), "valkey://")
		}
		return NewValkeyStorage(addr, "", 0)
	}
	if dbURI != "" {
		return NewPgStorage(dbURI)
	}
	return NewMemStorage(), nil
}
