package redisx

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// Pinger checks Redis connectivity for health.
type Pinger interface {
	Ping(ctx context.Context) error
	Close() error
}

type clientPinger struct {
	rdb *redis.Client
}

// Open builds a Redis client from REDIS_URL without requiring connectivity.
// ParseURL enables TLS for rediss:// (any managed provider). Never log the URL (password).
// Optional REDIS_TLS_CA_FILE: PEM CA for providers with private CA (e.g. Yandex Managed Valkey).
// Health reports degraded if Ping fails; Phase 0 startup must not require Redis.
func Open(redisURL string) (Pinger, error) {
	return OpenWithTLSCA(redisURL, strings.TrimSpace(os.Getenv("REDIS_TLS_CA_FILE")))
}

// OpenWithTLSCA is like Open but takes an explicit CA PEM path (empty = system roots only).
func OpenWithTLSCA(redisURL, tlsCAFile string) (Pinger, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}
	opts.MaxRetries = 1
	if tlsCAFile != "" {
		if err := applyTLSCAFile(opts, tlsCAFile); err != nil {
			return nil, err
		}
	}
	return &clientPinger{rdb: redis.NewClient(opts)}, nil
}

func applyTLSCAFile(opts *redis.Options, tlsCAFile string) error {
	pemBytes, err := os.ReadFile(tlsCAFile)
	if err != nil {
		return fmt.Errorf("read redis tls ca file: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pemBytes) {
		return fmt.Errorf("parse redis tls ca file: no certificates found")
	}
	if opts.TLSConfig == nil {
		opts.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	opts.TLSConfig.RootCAs = pool
	return nil
}

func (p *clientPinger) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := p.rdb.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("ping redis: %w", err)
	}
	return nil
}

func (p *clientPinger) Close() error {
	return p.rdb.Close()
}
