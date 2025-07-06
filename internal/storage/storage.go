package storage

import (
	"context"

	"github.com/callmedenchick/callmebridge/internal/models"
)



type Storage interface {
	GetMessages(ctx context.Context, keys []string, lastEventId int64) ([]models.SseMessage, error)
	Add(ctx context.Context, key string, ttl int64, mes models.SseMessage) error
	HealthCheck() error
}


func NewStorage(dbURI string) (Storage, error) {
	if dbURI != "" {
		return NewPgStorage(dbURI)
	}
	return NewMemStorage(), nil
}
