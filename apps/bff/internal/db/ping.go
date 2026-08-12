package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// Pinger checks database connectivity for health.
type Pinger interface {
	Ping(ctx context.Context) error
	Close() error
}

type sqlPinger struct {
	db *sql.DB
}

// Open opens a Postgres pool from DATABASE_URL without requiring connectivity.
// Never log the DSN (may contain password). Health reports degraded if Ping fails.
func Open(databaseURL string) (Pinger, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(time.Minute)
	return &sqlPinger{db: db}, nil
}

func (p *sqlPinger) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := p.db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping postgres: %w", err)
	}
	return nil
}

func (p *sqlPinger) Close() error {
	return p.db.Close()
}
