package main

import (
	"context"
	"sync"

	log "github.com/sirupsen/logrus"
	"github.com/callmedenchick/callmebridge/internal/models"
)

type Session struct {
	mux         sync.RWMutex
	ClientIds   []string
	MessageCh   chan models.SseMessage
	storage     db
	Closer      chan interface{}
	lastEventId int64
}

func NewSession(s db, clientIds []string, lastEventId int64) *Session {
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
			break
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
