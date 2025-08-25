package config

import (
	"log"
	"strings"

	"github.com/caarlos0/env/v6"
	"github.com/sirupsen/logrus"
)

var Config = struct {
	LogLevel              string   `env:"LOG_LEVEL" envDefault:"info"`
	Port                  int      `env:"PORT" envDefault:"8081"`
	Storage               string   `env:"STORAGE" envDefault:"memory"`
	PostgresURI           string   `env:"POSTGRES_URI"`
	ValkeyURI             string   `env:"VALKEY_URI"`
	WebhookURL            string   `env:"WEBHOOK_URL"`
	CopyToURL             string   `env:"COPY_TO_URL"`
	CorsEnable            bool     `env:"CORS_ENABLE"`
	HeartbeatInterval     int      `env:"HEARTBEAT_INTERVAL" envDefault:"10"`
	RPSLimit              int      `env:"RPS_LIMIT" envDefault:"1000"`
	RateLimitsByPassToken []string `env:"RATE_LIMITS_BY_PASS_TOKEN"`
	ConnectionsLimit      int      `env:"CONNECTIONS_LIMIT" envDefault:"200"`
	SelfSignedTLS         bool     `env:"SELF_SIGNED_TLS" envDefault:"false"`
}{}

func LoadConfig() {
	if err := env.Parse(&Config); err != nil {
		log.Fatalf("config parsing failed: %v\n", err)
	}

	// Set logrus log level from LOG_LEVEL environment variable
	level, err := logrus.ParseLevel(strings.ToLower(Config.LogLevel))
	if err != nil {
		log.Printf("Invalid LOG_LEVEL '%s', using default 'info'. Valid levels: panic, fatal, error, warn, info, debug, trace", Config.LogLevel)
		level = logrus.InfoLevel
	}
	logrus.SetLevel(level)
}
