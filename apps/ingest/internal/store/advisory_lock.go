package store

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SchedulerAdvisoryLockKey is the stable Phase 1 mutex for HH scheduler runs.
// Decimal is used deliberately so the same key is easy to inspect in pg_locks.
const SchedulerAdvisoryLockKey int64 = 549_004_801

// AdvisoryLock owns the dedicated PostgreSQL session holding a lock.
type AdvisoryLock struct {
	conn *pgxpool.Conn
	once sync.Once
}

// TrySchedulerLock acquires the session-level lock without waiting.
// The returned connection is dedicated until Release.
func (s *PG) TrySchedulerLock(ctx context.Context) (*AdvisoryLock, bool, error) {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return nil, false, sanitizeDBError("acquire scheduler lock connection", err)
	}
	var acquired bool
	if err := conn.QueryRow(ctx,
		`SELECT pg_try_advisory_lock($1)`,
		SchedulerAdvisoryLockKey,
	).Scan(&acquired); err != nil {
		conn.Release()
		return nil, false, sanitizeDBError("try scheduler advisory lock", err)
	}
	if !acquired {
		conn.Release()
		return nil, false, nil
	}
	return &AdvisoryLock{conn: conn}, true, nil
}

// Release unlocks and returns the dedicated connection to the pool.
func (l *AdvisoryLock) Release(ctx context.Context) error {
	if l == nil || l.conn == nil {
		return nil
	}
	var releaseErr error
	l.once.Do(func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		var unlocked bool
		if err := l.conn.QueryRow(cleanupCtx,
			`SELECT pg_advisory_unlock($1)`,
			SchedulerAdvisoryLockKey,
		).Scan(&unlocked); err != nil {
			releaseErr = sanitizeDBError("release scheduler advisory lock", err)
			raw := l.conn.Hijack()
			_ = raw.Close(cleanupCtx)
		} else if !unlocked {
			releaseErr = fmt.Errorf("store release scheduler advisory lock: lock was not held")
			raw := l.conn.Hijack()
			_ = raw.Close(cleanupCtx)
		} else {
			l.conn.Release()
		}
		l.conn = nil
	})
	return releaseErr
}
