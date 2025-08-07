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
	MessageCh   chan models.SseMessage
	storage     storage.Storage
	Closer      chan interface{}
	lastEventId int64
}

func NewSession(s storage.Storage, clientIds []string, lastEventId int64) *Session {
	session := Session{
		mux:         sync.RWMutex{},
		ClientIds:   clientIds,
		storage:     s,
		MessageCh:   make(chan models.SseMessage, 10),
		Closer:      make(chan interface{}),
		lastEventId: lastEventId,
	}
	return &session
}

func (s *Session) worker() {
	// TODO: Replace with pub-sub subscription
	// For now, just wait for closer to maintain compatibility
	<-s.Closer
	close(s.MessageCh)
}

func (s *Session) AddMessageToQueue(ctx context.Context, mes models.SseMessage) {
	select {
	case <-s.Closer:
	default:
		s.MessageCh <- mes
	}
}

func (s *Session) Start() {
	go s.worker()
}
