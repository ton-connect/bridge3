package storage

import (
	"context"
	"sync"
	"time"

	"github.com/ton-connect/bridge3/internal/models"
)

type MemStorage struct {
	db          map[string][]message
	subscribers map[string][]chan<- models.SseMessage
	lock        sync.Mutex
}

type message struct {
	models.SseMessage
	expireAt time.Time
}

func (m message) isExpired(now time.Time) bool {
	return m.expireAt.Before(now)
}

func NewMemStorage() *MemStorage {
	s := MemStorage{
		db:          map[string][]message{},
		subscribers: make(map[string][]chan<- models.SseMessage),
	}
	go s.watcher()
	return &s
}

func removeExpiredMessages(ms []message, now time.Time) []message {
	results := make([]message, 0)
	for _, m := range ms {
		if !m.isExpired(now) {
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

// Pub publishes a message to all subscribers and stores it with TTL
func (s *MemStorage) Pub(ctx context.Context, key string, ttl int64, mes models.SseMessage) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	// Store message with TTL
	s.db[key] = append(s.db[key], message{
		SseMessage: mes,
		expireAt:   time.Now().Add(time.Duration(ttl) * time.Second),
	})

	// Send to all subscribers for this key
	if subscribers, exists := s.subscribers[key]; exists {
		for _, ch := range subscribers {
			select {
			case ch <- mes:
			default:
				// Channel is full or closed, skip
			}
		}
	}

	return nil
}

// Sub subscribes to messages for the given keys and sends historical messages after lastEventId
func (s *MemStorage) Sub(ctx context.Context, keys []string, lastEventId int64, messageCh chan<- models.SseMessage) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	// Add to subscribers
	for _, key := range keys {
		if s.subscribers[key] == nil {
			s.subscribers[key] = make([]chan<- models.SseMessage, 0)
		}
		s.subscribers[key] = append(s.subscribers[key], messageCh)
	}

	// Retrieve messages
	now := time.Now()
	for _, key := range keys {
		messages, exists := s.db[key]
		if !exists {
			continue
		}

		for _, msg := range messages {
			if msg.isExpired(now) {
				continue
			}
			if msg.EventId <= lastEventId {
				continue
			}

			select {
			case messageCh <- msg.SseMessage:
			default:
				// Channel is full or closed, skip
			}
		}
	}

	return nil
}

// Unsub unsubscribes from messages for the given keys
func (s *MemStorage) Unsub(ctx context.Context, keys []string) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	for _, key := range keys {
		delete(s.subscribers, key)
	}

	return nil
}

// HealthCheck should be implemented
func (s *MemStorage) HealthCheck() error {
	return nil // Always healthy
}
