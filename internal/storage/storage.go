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

func NewStorage(pgURI, natsURI string) (Storage, error) {
	if natsURI != "" && pgURI != "" {
		return nil, fmt.Errorf("both NATS and PostgreSQL URIs are provided.")
	}

	if natsURI != "" {
		return NewNatsStorage(natsURI)
	}

	if pgURI != "" {
		return NewPgStorage(pgURI)
	}

	return NewMemStorage(), nil
}
