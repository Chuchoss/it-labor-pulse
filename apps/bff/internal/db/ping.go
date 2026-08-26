package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Pinger checks database connectivity for health.
type Pinger interface {
	Ping(ctx context.Context) error
	Close() error
}

type Client struct {
	pool *pgxpool.Pool
}

// Open opens a Postgres pool from DATABASE_URL without requiring connectivity.
// Never log the DSN (may contain password). Health reports degraded if Ping fails.
func Open(databaseURL string) (*Client, error) {
	if strings.TrimSpace(databaseURL) == "" {
		return nil, fmt.Errorf("open postgres: DATABASE_URL is required")
	}
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open postgres: invalid database configuration")
	}
	cfg.MaxConns = 5
	cfg.MinConns = 0
	cfg.MaxConnIdleTime = time.Minute
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.HealthCheckPeriod = 30 * time.Second
	cfg.ConnConfig.RuntimeParams["application_name"] = "lma-bff"
	cfg.ConnConfig.RuntimeParams["statement_timeout"] = "5s"
	if cfg.ConnConfig.ConnectTimeout == 0 {
		cfg.ConnConfig.ConnectTimeout = 5 * time.Second
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		return nil, fmt.Errorf("open postgres pool: %w", err)
	}
	return &Client{pool: pool}, nil
}

func (p *Client) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := p.pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping postgres: %w", err)
	}
	return nil
}

func (p *Client) Close() error {
	p.pool.Close()
	return nil
}

func (p *Client) Pool() *pgxpool.Pool {
	return p.pool
}
