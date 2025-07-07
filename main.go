package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"time"

	"github.com/callmedenchick/callmebridge/internal/config"
	"github.com/callmedenchick/callmebridge/internal/crypto"
	"github.com/callmedenchick/callmebridge/internal/storage"
	"github.com/labstack/echo-contrib/prometheus"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"golang.org/x/exp/slices"
	"golang.org/x/time/rate"
)

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
		w.Header().Set("Content-Type", "application/json")
		response := map[string]string{
			"status": "ok",
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			log.Errorf("failed to encode health check response: %v", err)
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
			http.Error(w, "Database not ready", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		response := map[string]string{
			"status": "ready",
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			log.Errorf("failed to encode readiness check response: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		log.Debug("readiness check response sent")
	})
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
	e.Use(connectionsLimitMiddleware(newConnectionLimiter(config.Config.ConnectionsLimit), func(c echo.Context) bool {
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

	h := newHandler(dbConn, time.Duration(config.Config.HeartbeatInterval)*time.Second)

	registerHandlers(e, h)
	var existedPaths []string
	for _, r := range e.Routes() {
		existedPaths = append(existedPaths, r.Path)
	}
	p := prometheus.NewPrometheus("http", func(c echo.Context) bool {
		return !slices.Contains(existedPaths, c.Path())
	})
	e.Use(p.HandlerFunc)
	if config.Config.SelfSignedTLS {
		cert, key, err := crypto.GenerateSelfSignedCertificate()
		if err != nil {
			log.Fatalf("failed to generate self signed certificate: %v", err)
		}
		log.Fatal(e.StartTLS(fmt.Sprintf(":%v", config.Config.Port), cert, key))
	} else {
		log.Fatal(e.Start(fmt.Sprintf(":%v", config.Config.Port)))
	}
}
