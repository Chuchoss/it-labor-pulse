package config

import (
	"fmt"
	"os"
	"strings"
)

// Config holds BFF runtime settings from environment.
type Config struct {
	HTTPAddr    string
	DatabaseURL string
	AppEnv      string
	LogLevel    string
}

// Load reads BFF config from env.
// Listen address precedence: BFF_HTTP_ADDR, then PORT / BFF_PORT (bare port or :port), default :8080.
// DATABASE_URL is optional; when set, GET /api/v1/health pings Postgres.
func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:    resolveHTTPAddr(),
		DatabaseURL: strings.TrimSpace(os.Getenv("DATABASE_URL")),
		AppEnv:      envOr("APP_ENV", "local"),
		LogLevel:    envOr("LOG_LEVEL", "info"),
	}
	if cfg.HTTPAddr == "" {
		return Config{}, fmt.Errorf("http listen address is empty")
	}
	return cfg, nil
}

func resolveHTTPAddr() string {
	if v := strings.TrimSpace(os.Getenv("BFF_HTTP_ADDR")); v != "" {
		return normalizeAddr(v)
	}
	if v := strings.TrimSpace(os.Getenv("BFF_PORT")); v != "" {
		return normalizeAddr(v)
	}
	if v := strings.TrimSpace(os.Getenv("PORT")); v != "" {
		return normalizeAddr(v)
	}
	return ":8080"
}

func normalizeAddr(v string) string {
	if strings.Contains(v, ":") {
		return v
	}
	return ":" + v
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
