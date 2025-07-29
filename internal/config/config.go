package config

import (
	"os"
	"time"
)

type Config struct {
	Port           string
	DatabasePath   string
	MaxPayloadSize int64
	SessionTimeout time.Duration
	ReadTimeout    time.Duration
	MaxConnections int

	MaxRequestsPerSecond    int
	RateLimitWindowDuration time.Duration
	CooldownDuration        time.Duration
	MaxCooldownDuration     time.Duration
	MaxViolations           int
}

func Load() *Config {
	return &Config{
		Port:           getEnv("PORT", ":8080"),
		DatabasePath:   getEnv("DATABASE_PATH", "./channels.db"),
		MaxPayloadSize: 16 << 20, // 16MB
		SessionTimeout: 10 * time.Second,
		ReadTimeout:    5 * time.Second,
		MaxConnections: 50000,

		MaxRequestsPerSecond:    10,
		RateLimitWindowDuration: 1 * time.Second,
		CooldownDuration:        10 * time.Second,
		MaxCooldownDuration:     1 * time.Hour,
		MaxViolations:           3,
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
