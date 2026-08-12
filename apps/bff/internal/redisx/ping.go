package redisx

import (
	"context"
	"fmt"
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
// ParseURL enables TLS for rediss:// (Upstash / managed). Never log the URL (password).
// Health reports degraded if Ping fails; Phase 0 startup must not require Redis.
func Open(redisURL string) (Pinger, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}
	opts.MaxRetries = 1
	return &clientPinger{rdb: redis.NewClient(opts)}, nil
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
