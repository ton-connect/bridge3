package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	log "github.com/sirupsen/logrus"
	"github.com/ton-connect/bridge3/internal/config"
	"github.com/ton-connect/bridge3/internal/models"
	"github.com/ton-connect/bridge3/internal/storage"
	"github.com/ton-connect/bridge3/internal/utils"
)

var (
	activeConnectionMetric = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "number_of_acitve_connections",
		Help: "The number of active connections",
	})
	activeSubscriptionsMetric = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "number_of_active_subscriptions",
		Help: "The number of active subscriptions",
	})
	transferedMessagesNumMetric = promauto.NewCounter(prometheus.CounterOpts{
		Name: "number_of_transfered_messages",
		Help: "The total number of transfered_messages",
	})
	deliveredMessagesMetric = promauto.NewCounter(prometheus.CounterOpts{
		Name: "number_of_delivered_messages",
		Help: "The total number of delivered_messages",
	})
	badRequestMetric = promauto.NewCounter(prometheus.CounterOpts{
		Name: "number_of_bad_requests",
		Help: "The total number of bad requests",
	})
	clientIdsPerConnectionMetric = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "number_of_client_ids_per_connection",
		Buckets: []float64{1, 2, 3, 4, 5, 10, 20, 30, 40, 50, 100},
	})
	// TODO implement
	// expiredMessagesMetric = promauto.NewCounter(prometheus.CounterOpts{
	// 	Name: "number_of_expired_messages",
	// 	Help: "The total number of expired messages",
	// })
)

type stream struct {
	Sessions []*Session
	mux      sync.RWMutex
}
type handler struct {
	Mux               sync.RWMutex
	Connections       map[string]*stream
	storage           storage.Storage
	_eventIDs         int64
	heartbeatInterval time.Duration
}

func NewHandler(s storage.Storage, heartbeatInterval time.Duration) *handler {
	h := handler{
		Mux:               sync.RWMutex{},
		Connections:       make(map[string]*stream),
		storage:           s,
		_eventIDs:         time.Now().UnixMicro(),
		heartbeatInterval: heartbeatInterval,
	}
	return &h
}

func (h *handler) EventRegistrationHandler(c echo.Context) error {
	log := log.WithField("prefix", "EventRegistrationHandler")
	_, ok := c.Response().Writer.(http.Flusher)
	if !ok {
		http.Error(c.Response().Writer, "streaming unsupported", http.StatusInternalServerError)
		return c.JSON(utils.HttpResError("streaming unsupported", http.StatusBadRequest))
	}
	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("Connection", "keep-alive")
	c.Response().Header().Set("Transfer-Encoding", "chunked")
	c.Response().WriteHeader(http.StatusOK)
	if _, err := fmt.Fprint(c.Response(), "\n"); err != nil {
		log.Errorf("failed to write initial newline: %v", err)
		return err
	}
	c.Response().Flush()
	params := c.QueryParams()

	var lastEventId int64
	var err error
	lastEventIDStr := c.Request().Header.Get("Last-Event-ID")
	if lastEventIDStr != "" {
		lastEventId, err = strconv.ParseInt(lastEventIDStr, 10, 64)
		if err != nil {
			badRequestMetric.Inc()
			errorMsg := "Last-Event-ID should be int"
			log.Error(errorMsg)
			return c.JSON(utils.HttpResError(errorMsg, http.StatusBadRequest))
		}
	}
	lastEventIdQuery, ok := params["last_event_id"]
	if ok && lastEventId == 0 {
		lastEventId, err = strconv.ParseInt(lastEventIdQuery[0], 10, 64)
		if err != nil {
			badRequestMetric.Inc()
			errorMsg := "last_event_id should be int"
			log.Error(errorMsg)
			return c.JSON(utils.HttpResError(errorMsg, http.StatusBadRequest))
		}
	}
	clientId, ok := params["client_id"]
	if !ok {
		badRequestMetric.Inc()
		errorMsg := "param \"client_id\" not present"
		log.Error(errorMsg)
		return c.JSON(utils.HttpResError(errorMsg, http.StatusBadRequest))
	}
	clientIds := strings.Split(clientId[0], ",")
	clientIdsPerConnectionMetric.Observe(float64(len(clientIds)))
	session := h.CreateSession(clientId[0], clientIds, lastEventId)

	ctx := c.Request().Context()
	notify := ctx.Done()
	go func() {
		<-notify
		session.Close()
		h.removeConnection(session)
		log.Infof("connection: %v closed with error %v", session.ClientIds, ctx.Err())
	}()
	ticker := time.NewTicker(h.heartbeatInterval)
	defer ticker.Stop()
	session.Start()
loop:
	for {
		select {
		case msg, ok := <-session.GetMessages():
			if !ok {
				log.Errorf("can't read from channel")
				break loop
			}
			_, err = fmt.Fprintf(c.Response(), "event: %v\nid: %v\ndata: %v\n\n", "message", msg.EventId, string(msg.Message))
			if err != nil {
				log.Errorf("msg can't write to connection: %v", err)
				break loop
			}
			c.Response().Flush()
			deliveredMessagesMetric.Inc()
		case <-ticker.C:
			_, err = fmt.Fprintf(c.Response(), "event: heartbeat\n\n")
			if err != nil {
				log.Errorf("ticker can't write to connection: %v", err)
				break loop
			}
			c.Response().Flush()
		}
	}
	activeConnectionMetric.Dec()
	log.Info("connection closed")
	return nil
}

func (h *handler) SendMessageHandler(c echo.Context) error {
	ctx := c.Request().Context()
	log := log.WithContext(ctx).WithField("prefix", "SendMessageHandler")

	params := c.QueryParams()
	clientId, ok := params["client_id"]
	if !ok {
		badRequestMetric.Inc()
		errorMsg := "param \"client_id\" not present"
		log.Error(errorMsg)
		return c.JSON(utils.HttpResError(errorMsg, http.StatusBadRequest))
	}

	toId, ok := params["to"]
	if !ok {
		badRequestMetric.Inc()
		errorMsg := "param \"to\" not present"
		log.Error(errorMsg)
		return c.JSON(utils.HttpResError(errorMsg, http.StatusBadRequest))
	}

	ttlParam, ok := params["ttl"]
	if !ok {
		badRequestMetric.Inc()
		errorMsg := "param \"ttl\" not present"
		log.Error(errorMsg)
		return c.JSON(utils.HttpResError(errorMsg, http.StatusBadRequest))
	}
	ttl, err := strconv.ParseInt(ttlParam[0], 10, 32)
	if err != nil {
		badRequestMetric.Inc()
		log.Error(err)
		return c.JSON(utils.HttpResError(err.Error(), http.StatusBadRequest))
	}
	if ttl > 300 { // TODO: config
		badRequestMetric.Inc()
		errorMsg := "param \"ttl\" too high"
		log.Error(errorMsg)
		return c.JSON(utils.HttpResError(errorMsg, http.StatusBadRequest))
	}
	message, err := io.ReadAll(c.Request().Body)
	if err != nil {
		badRequestMetric.Inc()
		log.Error(err)
		return c.JSON(utils.HttpResError(err.Error(), http.StatusBadRequest))
	}
	mes, err := json.Marshal(models.BridgeMessage{
		From:    clientId[0],
		Message: string(message),
	})
	if err != nil {
		badRequestMetric.Inc()
		log.Error(err)
		return c.JSON(utils.HttpResError(err.Error(), http.StatusBadRequest))
	}
	if config.Config.CopyToURL != "" {
		go func() {
			u, err := url.Parse(config.Config.CopyToURL)
			if err != nil {
				return
			}
			u.RawQuery = params.Encode()
			req, err := http.NewRequest(http.MethodPost, u.String(), bytes.NewReader(message))
			if err != nil {
				return
			}
			http.DefaultClient.Do(req) //nolint:errcheck// TODO review golangci-lint issue
		}()
	}
	topic, ok := params["topic"]
	if ok {
		go func(clientID, topic, message string) {
			SendWebhook(clientID, WebhookData{Topic: topic, Hash: message})
		}(clientId[0], topic[0], string(message))
	}

	sseMessage := models.SseMessage{
		EventId: h.nextID(),
		Message: mes,
	}

	// Send message only to storage - pub-sub will handle distribution
	go func() {
		log := log.WithField("prefix", "SendMessageHandler.storage.Pub")
		err = h.storage.Pub(context.Background(), toId[0], ttl, sseMessage)
		if err != nil {
			log.Errorf("db error: %v", err)
		}
	}()

	transferedMessagesNumMetric.Inc()
	return c.JSON(http.StatusOK, utils.HttpResOk())

}

func (h *handler) removeConnection(ses *Session) {
	log := log.WithField("prefix", "removeConnection")
	log.Infof("remove session: %v", ses.ClientIds)
	for _, id := range ses.ClientIds {
		h.Mux.RLock()
		s, ok := h.Connections[id]
		h.Mux.RUnlock()
		if !ok {
			log.Info("alredy removed")
			continue
		}
		s.mux.Lock()
		for i := range s.Sessions {
			if s.Sessions[i] == ses {
				s.Sessions[i] = s.Sessions[len(s.Sessions)-1]
				s.Sessions = s.Sessions[:len(s.Sessions)-1]
				break
			}
		}
		s.mux.Unlock()

		if len(s.Sessions) == 0 {
			h.Mux.Lock()
			delete(h.Connections, id)
			h.Mux.Unlock()
		}
		activeSubscriptionsMetric.Dec()
	}
}

func (h *handler) CreateSession(sessionId string, clientIds []string, lastEventId int64) *Session {
	log := log.WithField("prefix", "CreateSession")
	log.Infof("make new session with ids: %v", clientIds)
	session := NewSession(h.storage, clientIds, lastEventId)
	activeConnectionMetric.Inc()
	for _, id := range clientIds {
		h.Mux.RLock()
		s, ok := h.Connections[id]
		h.Mux.RUnlock()
		if ok {
			s.mux.Lock()
			s.Sessions = append(s.Sessions, session)
			s.mux.Unlock()
		} else {
			h.Mux.Lock()
			h.Connections[id] = &stream{
				mux:      sync.RWMutex{},
				Sessions: []*Session{session},
			}
			h.Mux.Unlock()
		}

		activeSubscriptionsMetric.Inc()
	}
	return session
}

func (h *handler) nextID() int64 {
	return atomic.AddInt64(&h._eventIDs, 1)
}
