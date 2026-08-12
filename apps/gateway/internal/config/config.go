package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

// Config holds gateway runtime settings from environment.
type Config struct {
	HTTPAddr    string
	BFFUpstream string
	AppEnv      string
	LogLevel    string
}

// Load reads gateway config from env.
// Listen: GATEWAY_HTTP_ADDR, then PORT / GATEWAY_PORT, default :8080.
// Upstream: BFF_UPSTREAM, default http://127.0.0.1:8081
func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:    resolveHTTPAddr(),
		BFFUpstream: envOr("BFF_UPSTREAM", "http://127.0.0.1:8081"),
		AppEnv:      envOr("APP_ENV", "local"),
		LogLevel:    envOr("LOG_LEVEL", "info"),
	}
	if cfg.HTTPAddr == "" {
		return Config{}, fmt.Errorf("http listen address is empty")
	}
	u, err := url.Parse(cfg.BFFUpstream)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return Config{}, fmt.Errorf("invalid BFF_UPSTREAM %q: need absolute URL", cfg.BFFUpstream)
	}
	return cfg, nil
}

func resolveHTTPAddr() string {
	if v := strings.TrimSpace(os.Getenv("GATEWAY_HTTP_ADDR")); v != "" {
		return normalizeAddr(v)
	}
	if v := strings.TrimSpace(os.Getenv("GATEWAY_PORT")); v != "" {
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
