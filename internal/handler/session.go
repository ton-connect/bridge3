package handler

import (
	"context"
	"sync"

	"github.com/ton-connect/bridge3/internal/models"
	"github.com/ton-connect/bridge3/internal/storage"
)

type Session struct {
	mux         sync.RWMutex
	ClientIds   []string
	storage     storage.Storage
	messageCh   chan models.SseMessage // Internal channel for storage notifications
	Closer      chan interface{}
	lastEventId int64
	isActive    bool
}

func NewSession(s storage.Storage, clientIds []string, lastEventId int64) *Session {
	session := Session{
		mux:         sync.RWMutex{},
		ClientIds:   clientIds,
		storage:     s,
		messageCh:   make(chan models.SseMessage, 100), // Larger buffer for pub-sub
		Closer:      make(chan interface{}),
		lastEventId: lastEventId,
		isActive:    false,
	}
	return &session
}

// GetMessages returns the read-only channel for receiving messages
func (s *Session) GetMessages() <-chan models.SseMessage {
	return s.messageCh
}

// Close stops the session and cleans up resources
func (s *Session) Close() {
	s.mux.Lock()
	defer s.mux.Unlock()

	if !s.isActive {
		return
	}

	s.isActive = false

	// Unsubscribe from storage
	err := s.storage.Unsub(context.Background(), s.ClientIds)
	if err != nil {
		// Log error but don't fail - this is cleanup
	}

	// Close channels
	close(s.Closer)
	close(s.messageCh)
}

// Start begins the session by subscribing to storage
func (s *Session) Start() {
	s.mux.Lock()
	defer s.mux.Unlock()

	if s.isActive {
		return
	}

	s.isActive = true

	// Subscribe to storage for real-time messages
	err := s.storage.Sub(context.Background(), s.ClientIds, s.messageCh)
	if err != nil {
		// If subscription fails, mark as inactive
		s.isActive = false
		close(s.messageCh)
		return
	}
}
