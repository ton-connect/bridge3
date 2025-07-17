package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"strings"
	"time"

	"github.com/callmedenchick/callmebridge/internal/config"
	"github.com/callmedenchick/callmebridge/internal/handler"
	bridge_middleware "github.com/callmedenchick/callmebridge/internal/middleware"
	"github.com/callmedenchick/callmebridge/internal/storage"

	"github.com/callmedenchick/callmebridge/internal/utils"
	"github.com/labstack/echo-contrib/prometheus"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	client_prometheus "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"golang.org/x/exp/slices"
	"golang.org/x/time/rate"
)

var (
	tokenUsageMetric = promauto.NewCounterVec(client_prometheus.CounterOpts{
		Name: "bridge_token_usage",
	}, []string{"token"})

	healthMetric = client_prometheus.NewGauge(client_prometheus.GaugeOpts{
		Name: "bridge_health_status",
		Help: "Health status of the bridge (1 = healthy, 0 = unhealthy)",
	})
	readyMetric = client_prometheus.NewGauge(client_prometheus.GaugeOpts{
		Name: "bridge_ready_status",
		Help: "Ready status of the bridge (1 = ready, 0 = not ready)",
	})
)

func skipRateLimitsByToken(request *http.Request) bool {
	if request == nil {
		return false
	}
	authorization := request.Header.Get("Authorization")
	if authorization == "" {
		return false
	}
	token := strings.TrimPrefix(authorization, "Bearer ")
	exist := slices.Contains(config.Config.RateLimitsByPassToken, token)
	if exist {
		tokenUsageMetric.WithLabelValues(token).Inc()
		return true
	}
	return false
}

func connectionsLimitMiddleware(counter *bridge_middleware.ConnectionsLimiter, skipper func(c echo.Context) bool) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if skipper(c) {
				return next(c)
			}
			release, err := counter.LeaseConnection(c.Request())
			if err != nil {
				return c.JSON(utils.HttpResError(err.Error(), http.StatusTooManyRequests))
			}
			defer release()
			return next(c)
		}
	}
}

func main() {
	log.Info("Bridge is running")
	config.LoadConfig()

	dbConn, err := storage.NewStorage(config.Config.DbURI)

	if err != nil {
		log.Fatalf("failed to create storage: %v", err)
	}
	if _, ok := dbConn.(*storage.MemStorage); ok {
		log.Info("Using in-memory storage")
	} else {
		log.Info("Using PostgreSQL storage")
	}

	http.Handle("/metrics", promhttp.Handler())
	http.Handle("/health", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log := log.WithField("prefix", "HealthHandler")
		log.Debug("health check request received")

		healthMetric.Set(1)

		w.Header().Set("Content-Type", "application/json")
		response := map[string]string{
			"status": "ok",
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			log.Errorf("failed to encode health check response: %v", err)
			healthMetric.Set(0)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		log.Debug("health check response sent")
	}))
	http.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		log := log.WithField("prefix", "ReadyHandler")
		log.Debug("readiness check request received")

		if err := dbConn.HealthCheck(); err != nil {
			log.Errorf("database connection error: %v", err)
			readyMetric.Set(0)
			http.Error(w, "Database not ready", http.StatusInternalServerError)
			return
		}

		readyMetric.Set(1)

		w.Header().Set("Content-Type", "application/json")
		response := map[string]string{
			"status": "ready",
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			log.Errorf("failed to encode readiness check response: %v", err)
			readyMetric.Set(0)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		log.Debug("readiness check response sent")
	})

	healthMetric.Set(1)
	if err := dbConn.HealthCheck(); err != nil {
		readyMetric.Set(0)
	} else {
		readyMetric.Set(1)
	}

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			if err := dbConn.HealthCheck(); err != nil {
				readyMetric.Set(0)
			} else {
				readyMetric.Set(1)
			}
		}
	}()

	go func() {
		log.Fatal(http.ListenAndServe(":9103", nil))
	}()

	e := echo.New()
	e.Use(middleware.RecoverWithConfig(middleware.RecoverConfig{
		Skipper:           nil,
		DisableStackAll:   true,
		DisablePrintStack: false,
	}))
	e.Use(middleware.Logger())
	e.Use(middleware.RateLimiterWithConfig(middleware.RateLimiterConfig{
		Skipper: func(c echo.Context) bool {
			if skipRateLimitsByToken(c.Request()) || c.Path() != "/bridge/message" {
				return true
			}
			return false
		},
		Store: middleware.NewRateLimiterMemoryStore(rate.Limit(config.Config.RPSLimit)),
	}))
	e.Use(connectionsLimitMiddleware(bridge_middleware.NewConnectionLimiter(config.Config.ConnectionsLimit), func(c echo.Context) bool {
		if skipRateLimitsByToken(c.Request()) || c.Path() != "/bridge/events" {
			return true
		}
		return false
	}))

	if config.Config.CorsEnable {
		corsConfig := middleware.CORSWithConfig(middleware.CORSConfig{
			AllowOrigins:     []string{"*"},
			AllowMethods:     []string{echo.GET, echo.POST, echo.OPTIONS},
			AllowHeaders:     []string{"DNT", "X-CustomHeader", "Keep-Alive", "User-Agent", "X-Requested-With", "If-Modified-Since", "Cache-Control", "Content-Type", "Authorization"},
			AllowCredentials: true,
			MaxAge:           86400,
		})
		e.Use(corsConfig)
	}

	h := handler.NewHandler(dbConn, time.Duration(config.Config.HeartbeatInterval)*time.Second)

	e.GET("/bridge/events", h.EventRegistrationHandler)
	e.POST("/bridge/message", h.SendMessageHandler)

	var existedPaths []string
	for _, r := range e.Routes() {
		existedPaths = append(existedPaths, r.Path)
	}
	p := prometheus.NewPrometheus("http", func(c echo.Context) bool {
		return !slices.Contains(existedPaths, c.Path())
	})
	e.Use(p.HandlerFunc)
	if config.Config.SelfSignedTLS {
		cert, key, err := utils.GenerateSelfSignedCertificate()
		if err != nil {
			log.Fatalf("failed to generate self signed certificate: %v", err)
		}
		log.Fatal(e.StartTLS(fmt.Sprintf(":%v", config.Config.Port), cert, key))
	} else {
		log.Fatal(e.Start(fmt.Sprintf(":%v", config.Config.Port)))
	}
}
