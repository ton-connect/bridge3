package storage

import (
	"context"
	"sync"
	"time"

	"github.com/ton-connect/bridge3/internal/models"
)

type MemStorage struct {
	db   map[string][]message
	lock sync.Mutex
}

type message struct {
	models.SseMessage
	expireAt time.Time
}

func (m message) IsExpired(now time.Time) bool {
	return m.expireAt.Before(now)
}

func NewMemStorage() *MemStorage {
	s := MemStorage{
		db: map[string][]message{},
	}
	go s.watcher()
	return &s
}

func removeExpiredMessages(ms []message, now time.Time) []message {
	results := make([]message, 0)
	for _, m := range ms {
		if !m.IsExpired(now) {
			results = append(results, m)
		}
	}
	return results
}

func (s *MemStorage) watcher() {
	for {
		s.lock.Lock()
		for key, ms := range s.db {
			s.db[key] = removeExpiredMessages(ms, time.Now())
		}
		s.lock.Unlock()
		time.Sleep(time.Second)
	}
}

func (s *MemStorage) GetMessages(ctx context.Context, keys []string, lastEventId int64) ([]models.SseMessage, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	now := time.Now()
	results := make([]models.SseMessage, 0)
	for _, key := range keys {
		messages, ok := s.db[key]
		if !ok {
			continue
		}
		for _, m := range messages {
			if m.IsExpired(now) {
				continue
			}
			if m.EventId <= lastEventId {
				continue
			}
			results = append(results, m.SseMessage)
		}
	}
	return results, nil
}

func (s *MemStorage) Add(ctx context.Context, key string, ttl int64, mes models.SseMessage) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.db[key] = append(s.db[key], message{SseMessage: mes, expireAt: time.Now().Add(time.Duration(ttl) * time.Second)})
	return nil
}

func (s *MemStorage) HealthCheck() error {
	// In-memory storage does not require health checks.
	return nil
}
