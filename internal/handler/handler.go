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

type connect_client struct {
	client_id string
	ip        string
	referrer  string // normalized origin
	time      time.Time
}

type verifyRequest struct {
	Type     string `json:"type"`
	ClientID string `json:"client_id"`
	URL      string `json:"url"`
	Message  string `json:"message,omitempty"`
}

type verifyResponse struct {
	Status string `json:"status"`
}

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
	datamap           map[string][]connect_client // todo - use lru maps, add ttl 5 minutes
}

func NewHandler(s storage.Storage, heartbeatInterval time.Duration) *handler {
	h := handler{
		Mux:               sync.RWMutex{},
		Connections:       make(map[string]*stream),
		storage:           s,
		_eventIDs:         time.Now().UnixMicro(),
		heartbeatInterval: heartbeatInterval,
		datamap:           make(map[string][]connect_client),
	}
	return &h
}

// TODO - rewrite to extract from origin, not referrer
func extractOrigin(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	if u.Scheme == "" || u.Host == "" {
		return rawURL
	}
	return u.Scheme + "://" + u.Host
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

	ip := c.RealIP()
	referrer := extractOrigin(c.Request().Header.Get("Referer"))
	connect_client := connect_client{
		client_id: clientId[0],
		ip:        ip,
		referrer:  referrer,
		time:      time.Now(),
	}
	h.Mux.Lock()
	h.datamap[clientId[0]] = append(h.datamap[clientId[0]], connect_client)
	h.Mux.Unlock()

	ctx := c.Request().Context()
	notify := ctx.Done()
	go func() {
		<-notify
		close(session.Closer)
		h.removeConnection(session)
		log.Infof("connection: %v closed with error %v", session.ClientIds, ctx.Err())
	}()
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

func (h *handler) ConnectVerifyHandler(c echo.Context) error {
	ip := c.RealIP() // Todo - move all ip extraction to single function

	// Support new JSON POST format; fallback to legacy query params for backward compatibility
	var req verifyRequest
	if c.Request().Method == http.MethodPost {
		decoder := json.NewDecoder(c.Request().Body)
		if err := decoder.Decode(&req); err != nil {
			badRequestMetric.Inc()
			return c.JSON(utils.HttpResError("invalid JSON body", http.StatusBadRequest))
		}
	} else {
		params := c.QueryParams()
		clientId, ok := params["client_id"]
		if ok && len(clientId) > 0 {
			req.ClientID = clientId[0]
		}
		urls, ok := params["url"]
		if ok && len(urls) > 0 {
			req.URL = urls[0]
		}
		types, ok := params["type"]
		if ok && len(types) > 0 {
			req.Type = types[0]
		} else {
			req.Type = "connect"
		}
	}

	if req.ClientID == "" {
		badRequestMetric.Inc()
		return c.JSON(utils.HttpResError("param \"client_id\" not present", http.StatusBadRequest))
	}
	if req.URL == "" {
		badRequestMetric.Inc()
		return c.JSON(utils.HttpResError("param \"url\" not present", http.StatusBadRequest))
	}
	req.URL = extractOrigin(req.URL)
	if req.Type == "" {
		badRequestMetric.Inc()
		return c.JSON(utils.HttpResError("param \"type\" not present", http.StatusBadRequest))
	}

	// Default status
	status := "unknown"
	now := time.Now()

	switch strings.ToLower(req.Type) {
	case "connect":
		h.Mux.RLock()
		existingConnects := h.datamap[req.ClientID]
		h.Mux.RUnlock()
		for _, connect := range existingConnects {
			if connect.ip == ip && connect.referrer == req.URL && now.Sub(connect.time) < 5*time.Minute {
				status = "ok"
				break
			}
		}
	default:
		badRequestMetric.Inc()
		return c.JSON(utils.HttpResError("param \"type\" must be one of: connect, message", http.StatusBadRequest))
	}

	return c.JSON(http.StatusOK, verifyResponse{Status: status})
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

	referrer := extractOrigin(c.Request().Header.Get("Origin"))
	ip := c.RealIP()
	userAgent := c.Request().Header.Get("User-Agent")

	// Create request source metadata
	requestSource := models.BridgeRequestSource{
		Origin:    referrer,
		IP:        ip,
		Time:      time.Now().UTC().Format(time.RFC3339),
		ClientID:  clientId[0],
		UserAgent: userAgent,
	}

	// Encrypt the request source metadata using the wallet's public key
	encryptedRequestSource, err := utils.EncryptRequestSourceWithWalletID(
		requestSource,
		toId[0], // todo - check to id properly
	)
	if err != nil {
		badRequestMetric.Inc()
		log.Error(err)
		return c.JSON(utils.HttpResError(fmt.Sprintf("failed to encrypt request source: %v", err), http.StatusBadRequest))
	}

	mes, err := json.Marshal(models.BridgeMessage{
		From:                clientId[0],
		Message:             string(message),
		BridgeRequestSource: encryptedRequestSource,
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

	h.Mux.RLock()
	s, ok := h.Connections[toId[0]]
	h.Mux.RUnlock()
	if ok {
		s.mux.Lock()
		for _, ses := range s.Sessions {
			ses.AddMessageToQueue(ctx, sseMessage)
		}
		s.mux.Unlock()
	}
	go func() {
		log := log.WithField("prefix", "SendMessageHandler.storge.Add")
		err = h.storage.Add(context.Background(), toId[0], ttl, sseMessage)
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
