package handler

import (
	"context"
	"sync"

	"github.com/ton-connect/bridge3/internal/models"
	"github.com/ton-connect/bridge3/internal/storage"
	log "github.com/sirupsen/logrus"
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
	log := log.WithField("prefix", "Session.worker")
	queue, err := s.storage.GetMessages(context.TODO(), s.ClientIds, s.lastEventId)
	if err != nil {
		log.Info("get queue error: ", err)
	}
	for _, m := range queue {
		select {
		case <-s.Closer:
			break //nolint:staticcheck// TODO review golangci-lint issue
		default:
			s.MessageCh <- m
		}
	}

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
