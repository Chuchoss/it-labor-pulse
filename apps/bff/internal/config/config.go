package config

import (
	"os"
	"strconv"
	"strings"
)

// Config holds BFF runtime settings from environment.
type Config struct {
	HTTPAddr                string
	DatabaseURL             string
	RedisURL                string
	AppEnv                  string
	LogLevel                string
	AssistantEnabled        bool
	AssistantDevAuthEnabled bool
}

// Load reads BFF config from env.
// Listen address precedence: BFF_HTTP_ADDR, then PORT / BFF_PORT (bare port or :port), default :8080.
// BFF is the public MVP edge (ADR 010). DATABASE_URL and REDIS_URL optional for health ping.
func Load() Config {
	return Config{
		HTTPAddr:                resolveHTTPAddr(),
		DatabaseURL:             strings.TrimSpace(os.Getenv("DATABASE_URL")),
		RedisURL:                strings.TrimSpace(os.Getenv("REDIS_URL")),
		AppEnv:                  envOr("APP_ENV", "local"),
		LogLevel:                envOr("LOG_LEVEL", "info"),
		AssistantEnabled:        envBool("ASSISTANT_ENABLED", false),
		AssistantDevAuthEnabled: envBool("ASSISTANT_DEV_AUTH_ENABLED", false),
	}
}

func envBool(key string, fallback bool) bool {
	value, err := strconv.ParseBool(strings.TrimSpace(os.Getenv(key)))
	if err != nil {
		return fallback
	}
	return value
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
