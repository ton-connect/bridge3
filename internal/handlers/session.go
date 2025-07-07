package handlers

import (
	"context"
	"sync"

	"github.com/callmedenchick/callmebridge/internal/models"
	"github.com/callmedenchick/callmebridge/internal/storage"
	log "github.com/sirupsen/logrus"
)

// Session represents an SSE session
type Session struct {
	mux         sync.RWMutex
	ClientIds   []string
	MessageCh   chan models.SseMessage
	storage     storage.Storage
	Closer      chan interface{}
	lastEventId int64
}

// NewSession creates a new session
func NewSession(s storage.Storage, clientIds []string, lastEventId int64) *Session {
	return &Session{
		mux:         sync.RWMutex{},
		ClientIds:   clientIds,
		storage:     s,
		MessageCh:   make(chan models.SseMessage, 10),
		Closer:      make(chan interface{}),
		lastEventId: lastEventId,
	}
}

// Start starts the session worker
func (s *Session) Start() {
	go s.worker()
}

// worker handles loading existing messages and managing the session lifecycle
func (s *Session) worker() {
	log := log.WithField("prefix", "Session.worker")

	// Load existing messages
	queue, err := s.storage.GetMessages(context.TODO(), s.ClientIds, s.lastEventId)
	if err != nil {
		log.Info("get queue error: ", err)
	}

	// Send existing messages
	for _, m := range queue {
		select {
		case <-s.Closer:
			return
		default:
			s.MessageCh <- m
		}
	}

	// Wait for session to close
	<-s.Closer
	close(s.MessageCh)
}

// AddMessageToQueue adds a message to the session's queue
func (s *Session) AddMessageToQueue(ctx context.Context, mes models.SseMessage) {
	select {
	case <-s.Closer:
		// Session is closed, ignore message
	default:
		s.MessageCh <- mes
	}
}
