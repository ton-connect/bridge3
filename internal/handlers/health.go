package handlers

import (
	"net/http"

	"github.com/callmedenchick/callmebridge/internal/storage"
	"github.com/labstack/echo/v4"
	log "github.com/sirupsen/logrus"
)

// HealthHandler handles health check endpoints
type HealthHandler struct {
	storage storage.Storage
}

// NewHealthHandler creates a new health handler
func NewHealthHandler(storage storage.Storage) *HealthHandler {
	return &HealthHandler{
		storage: storage,
	}
}

// Register registers health routes
func (h *HealthHandler) Register(e *echo.Echo) {
	e.GET("/health", h.Health)
	e.GET("/ready", h.Ready)
}

// Health returns basic health status
func (h *HealthHandler) Health(c echo.Context) error {
	log := log.WithField("prefix", "HealthHandler")
	log.Debug("health check request received")

	response := map[string]string{
		"status": "ok",
	}

	log.Debug("health check response sent")
	return c.JSON(http.StatusOK, response)
}

// Ready returns readiness status including database connectivity
func (h *HealthHandler) Ready(c echo.Context) error {
	log := log.WithField("prefix", "ReadyHandler")
	log.Debug("readiness check request received")

	if err := h.storage.HealthCheck(); err != nil {
		log.Errorf("database connection error: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"status": "not ready",
			"error":  "database not accessible",
		})
	}

	response := map[string]string{
		"status": "ready",
	}

	log.Debug("readiness check response sent")
	return c.JSON(http.StatusOK, response)
}
