package handlers

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

	"github.com/callmedenchick/callmebridge/internal/config"
	"github.com/callmedenchick/callmebridge/internal/models"
	"github.com/callmedenchick/callmebridge/internal/storage"
	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	log "github.com/sirupsen/logrus"
)

// SendMessageRequest represents a send message request
type SendMessageRequest struct {
	From    string
	To      string
	Content string
	TTL     int64
	Topic   string
}

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
)

type stream struct {
	Sessions []*Session
	mux      sync.RWMutex
}

// BridgeHandler handles bridge-related HTTP requests
type BridgeHandler struct {
	mux               sync.RWMutex
	connections       map[string]*stream
	storage           storage.Storage
	eventIDs          int64
	heartbeatInterval time.Duration
}

// NewBridgeHandler creates a new bridge handler
func NewBridgeHandler(storage storage.Storage, heartbeatInterval time.Duration) *BridgeHandler {
	return &BridgeHandler{
		mux:               sync.RWMutex{},
		connections:       make(map[string]*stream),
		storage:           storage,
		eventIDs:          time.Now().UnixMicro(),
		heartbeatInterval: heartbeatInterval,
	}
}

// Register registers bridge routes with Echo
func (h *BridgeHandler) Register(e *echo.Echo) {
	e.GET("/bridge/events", h.EventRegistration)
	e.POST("/bridge/message", h.SendMessage)
}

// EventRegistration handles SSE connections for receiving messages
func (h *BridgeHandler) EventRegistration(c echo.Context) error {
	log := log.WithField("prefix", "EventRegistrationHandler")

	// Check if streaming is supported
	_, ok := c.Response().Writer.(http.Flusher)
	if !ok {
		http.Error(c.Response().Writer, "streaming unsupported", http.StatusInternalServerError)
		return c.JSON(http.StatusBadRequest, ErrorResponse("streaming unsupported", http.StatusBadRequest))
	}

	// Set SSE headers
	h.setSSEHeaders(c)
	params := c.QueryParams()

	// Parse last event ID
	lastEventId, err := h.parseLastEventID(c, params)
	if err != nil {
		badRequestMetric.Inc()
		log.Error(err.Error())
		return c.JSON(http.StatusBadRequest, ErrorResponse(err.Error(), http.StatusBadRequest))
	}

	// Parse client IDs
	clientIds, err := h.parseClientIDs(params)
	if err != nil {
		badRequestMetric.Inc()
		log.Error(err.Error())
		return c.JSON(http.StatusBadRequest, ErrorResponse(err.Error(), http.StatusBadRequest))
	}

	clientIdsPerConnectionMetric.Observe(float64(len(clientIds)))
	session := h.createSession(clientIds[0], clientIds, lastEventId)

	// Handle connection cleanup
	ctx := c.Request().Context()
	notify := ctx.Done()
	go func() {
		<-notify
		close(session.Closer)
		h.removeConnection(session)
		log.Infof("connection: %v closed with error %v", session.ClientIds, ctx.Err())
	}()

	// Start message streaming
	return h.streamMessages(c, session)
}

// SendMessage handles sending messages to clients
func (h *BridgeHandler) SendMessage(c echo.Context) error {
	ctx := c.Request().Context()
	log := log.WithContext(ctx).WithField("prefix", "SendMessageHandler")

	// Parse request parameters
	req, err := h.parseSendMessageRequest(c)
	if err != nil {
		badRequestMetric.Inc()
		log.Error(err.Error())
		return c.JSON(http.StatusBadRequest, ErrorResponse(err.Error(), http.StatusBadRequest))
	}

	// Create SSE message
	messageContent, err := h.createMessageContent(req)
	if err != nil {
		badRequestMetric.Inc()
		log.Error(err.Error())
		return c.JSON(http.StatusBadRequest, ErrorResponse(err.Error(), http.StatusBadRequest))
	}

	sseMessage := models.SseMessage{
		EventId: h.nextID(),
		Message: messageContent,
	}

	// Send to active sessions
	h.sendToActiveSessions(ctx, req.To, sseMessage)

	// Store message for persistence
	go func() {
		err = h.storage.Add(context.Background(), req.To, req.TTL, sseMessage)
		if err != nil {
			log.Errorf("db error: %v", err)
		}
	}()

	// Handle external integrations
	h.handleExternalIntegrations(c, req)

	transferedMessagesNumMetric.Inc()
	return c.JSON(http.StatusOK, SuccessResponse())
}

// Helper methods

func (h *BridgeHandler) setSSEHeaders(c echo.Context) {
	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("Connection", "keep-alive")
	c.Response().Header().Set("Transfer-Encoding", "chunked")
	c.Response().WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(c.Response(), "\n") // TODO check error here
	c.Response().Flush()
}

func (h *BridgeHandler) parseLastEventID(c echo.Context, params map[string][]string) (int64, error) {
	var lastEventId int64
	var err error

	// Check header first
	lastEventIDStr := c.Request().Header.Get("Last-Event-ID")
	if lastEventIDStr != "" {
		lastEventId, err = strconv.ParseInt(lastEventIDStr, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("Last-Event-ID should be int")
		}
	}

	// Check query parameter
	lastEventIdQuery, ok := params["last_event_id"]
	if ok && lastEventId == 0 {
		lastEventId, err = strconv.ParseInt(lastEventIdQuery[0], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("last_event_id should be int")
		}
	}

	return lastEventId, nil
}

func (h *BridgeHandler) parseClientIDs(params map[string][]string) ([]string, error) {
	clientId, ok := params["client_id"]
	if !ok {
		return nil, fmt.Errorf("param \"client_id\" not present")
	}
	return strings.Split(clientId[0], ","), nil
}

func (h *BridgeHandler) streamMessages(c echo.Context, session *Session) error {
	ticker := time.NewTicker(h.heartbeatInterval)
	defer ticker.Stop()
	session.Start()

loop:
	for {
		select {
		case msg, ok := <-session.MessageCh:
			if !ok {
				log.Errorf("can't read from channel")
				break loop
			}
			_, err := fmt.Fprintf(c.Response(), "event: %v\nid: %v\ndata: %v\n\n", "message", msg.EventId, string(msg.Message))
			if err != nil {
				log.Errorf("msg can't write to connection: %v", err)
				break loop
			}
			c.Response().Flush()
			deliveredMessagesMetric.Inc()
		case <-ticker.C:
			_, err := fmt.Fprintf(c.Response(), "event: heartbeat\n\n")
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

func (h *BridgeHandler) nextID() int64 {
	return atomic.AddInt64(&h.eventIDs, 1)
}

// Additional helper methods for BridgeHandler

func (h *BridgeHandler) createSession(sessionId string, clientIds []string, lastEventId int64) *Session {
	log := log.WithField("prefix", "CreateSession")
	log.Infof("make new session with ids: %v", clientIds)
	session := NewSession(h.storage, clientIds, lastEventId)
	activeConnectionMetric.Inc()
	for _, id := range clientIds {
		h.mux.RLock()
		s, ok := h.connections[id]
		h.mux.RUnlock()
		if ok {
			s.mux.Lock()
			s.Sessions = append(s.Sessions, session)
			s.mux.Unlock()
		} else {
			h.mux.Lock()
			h.connections[id] = &stream{
				mux:      sync.RWMutex{},
				Sessions: []*Session{session},
			}
			h.mux.Unlock()
		}
		activeSubscriptionsMetric.Inc()
	}
	return session
}

func (h *BridgeHandler) removeConnection(ses *Session) {
	log := log.WithField("prefix", "removeConnection")
	log.Infof("remove session: %v", ses.ClientIds)
	for _, id := range ses.ClientIds {
		h.mux.RLock()
		s, ok := h.connections[id]
		h.mux.RUnlock()
		if !ok {
			log.Info("already removed")
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
			h.mux.Lock()
			delete(h.connections, id)
			h.mux.Unlock()
		}
		activeSubscriptionsMetric.Dec()
	}
}

func (h *BridgeHandler) parseSendMessageRequest(c echo.Context) (*SendMessageRequest, error) {
	params := c.QueryParams()

	clientId, ok := params["client_id"]
	if !ok {
		return nil, fmt.Errorf("param \"client_id\" not present")
	}

	toId, ok := params["to"]
	if !ok {
		return nil, fmt.Errorf("param \"to\" not present")
	}

	ttlParam, ok := params["ttl"]
	if !ok {
		return nil, fmt.Errorf("param \"ttl\" not present")
	}

	ttl, err := strconv.ParseInt(ttlParam[0], 10, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid ttl parameter: %w", err)
	}

	if ttl > 300 {
		return nil, fmt.Errorf("param \"ttl\" too high")
	}

	message, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read message body: %w", err)
	}

	topic := ""
	if topicParam, ok := params["topic"]; ok {
		topic = topicParam[0]
	}

	return &SendMessageRequest{
		From:    clientId[0],
		To:      toId[0],
		Content: string(message),
		TTL:     ttl,
		Topic:   topic,
	}, nil
}

func (h *BridgeHandler) createMessageContent(req *SendMessageRequest) ([]byte, error) {
	return json.Marshal(models.BridgeMessage{
		From:    req.From,
		Message: req.Content,
	})
}

func (h *BridgeHandler) sendToActiveSessions(ctx context.Context, clientID string, message models.SseMessage) {
	h.mux.RLock()
	s, ok := h.connections[clientID]
	h.mux.RUnlock()
	if ok {
		s.mux.Lock()
		for _, ses := range s.Sessions {
			ses.AddMessageToQueue(ctx, message)
		}
		s.mux.Unlock()
	}
}

func (h *BridgeHandler) handleExternalIntegrations(c echo.Context, req *SendMessageRequest) {
	params := c.QueryParams()

	// Handle copy to URL
	if config.Config.CopyToURL != "" {
		go func() {
			u, err := url.Parse(config.Config.CopyToURL)
			if err != nil {
				return
			}
			u.RawQuery = params.Encode()
			httpReq, err := http.NewRequest(http.MethodPost, u.String(), bytes.NewReader([]byte(req.Content)))
			if err != nil {
				return
			}
			// TODO check error here
			_, _ = http.DefaultClient.Do(httpReq)
		}()
	}

	// Handle webhook
	if req.Topic != "" {
		go func(clientID, topic, message string) {
			SendWebhook(clientID, WebhookData{Topic: topic, Hash: message})
		}(req.From, req.Topic, req.Content)
	}
}

// Temporary webhook implementation - these should be moved to services later
type WebhookData struct {
	Topic string `json:"topic"`
	Hash  string `json:"hash"`
}

func SendWebhook(clientID string, data WebhookData) {
	if config.Config.WebhookURL == "" {
		return
	}
	webhooks := strings.Split(config.Config.WebhookURL, ",")
	for _, webhook := range webhooks {
		go func(webhook string) {
			err := sendWebhook(clientID, data, webhook)
			if err != nil {
				log.Errorf("failed to trigger webhook '%s': %v", webhook, err)
			}
		}(webhook)
	}
}

func sendWebhook(clientID string, body WebhookData, webhook string) error {
	postBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal body: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, webhook+"/"+clientID, bytes.NewReader(postBody))
	if err != nil {
		return fmt.Errorf("failed to init request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed send request: %w", err)
	}
	defer func() {
		if closeErr := res.Body.Close(); closeErr != nil {
			log.Errorf("failed to close response body: %v", closeErr)
		}
	}()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status code: %v", res.StatusCode)
	}
	return nil
}
