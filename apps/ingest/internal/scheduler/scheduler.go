// Package scheduler periodically starts bounded ingest batches.
package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"
)

// ErrLockUnavailable means another scheduler process owns the HH run lock.
var ErrLockUnavailable = errors.New("scheduler advisory lock unavailable")

// Config controls scheduler cadence and failure handling.
type Config struct {
	Interval        time.Duration
	RunOnStart      bool
	BackoffInitial  time.Duration
	BackoffMax      time.Duration
	JitterPercent   float64
	ShutdownTimeout time.Duration
}

// Validate rejects unsafe scheduler settings.
func (c Config) Validate(testMode bool) error {
	minInterval := 10 * time.Minute
	if testMode {
		minInterval = time.Millisecond
	}
	if c.Interval < minInterval {
		return fmt.Errorf("scheduler interval must be at least %s", minInterval)
	}
	if c.BackoffInitial <= 0 {
		return fmt.Errorf("scheduler initial backoff must be positive")
	}
	if c.BackoffMax < c.BackoffInitial {
		return fmt.Errorf("scheduler max backoff must be at least initial backoff")
	}
	if c.JitterPercent < 0 || c.JitterPercent > 100 {
		return fmt.Errorf("scheduler jitter percent must be between 0 and 100")
	}
	if c.ShutdownTimeout <= 0 {
		return fmt.Errorf("scheduler shutdown timeout must be positive")
	}
	return nil
}

// BatchResult contains aggregate, non-PII observability fields.
type BatchResult struct {
	IngestRunID   string
	Stats         Stats
	CycleComplete bool
}

// Stats are aggregate counters for one bounded batch.
type Stats struct {
	Fetched   int
	Upserted  int
	Unchanged int
	Excluded  int
	Errors    int
	Pages     int
}

// Batch runs one bounded resumable unit of work.
type Batch func(context.Context, string) (BatchResult, error)

// ReleaseLock releases one acquired scheduler mutex.
type ReleaseLock func(context.Context) error

// AcquireLock tries to acquire a process-external mutex without waiting.
type AcquireLock func(context.Context) (ReleaseLock, bool, error)

// WithLock holds a dedicated external lock for the complete batch.
func WithLock(acquire AcquireLock, batch Batch) Batch {
	return func(ctx context.Context, schedulerRunID string) (result BatchResult, err error) {
		release, acquired, err := acquire(ctx)
		if err != nil {
			return BatchResult{}, err
		}
		if !acquired {
			return BatchResult{}, ErrLockUnavailable
		}
		defer func() {
			if releaseErr := release(ctx); releaseErr != nil {
				err = errors.Join(err, releaseErr)
			}
		}()
		return batch(ctx, schedulerRunID)
	}
}

// Timer is the resettable scheduling clock surface.
type Timer interface {
	C() <-chan time.Time
	Reset(time.Duration)
	Stop()
}

// Clock creates timers and supplies timestamps.
type Clock interface {
	Now() time.Time
	NewTimer(time.Duration) Timer
}

// Random supplies deterministic jitter in tests.
type Random interface {
	Float64() float64
}

// Engine coordinates ticks and guarantees at most one local batch.
type Engine struct {
	Config Config
	Batch  Batch
	Log    *slog.Logger
	Clock  Clock
	Random Random

	mu     sync.Mutex
	active bool
}

type runOutcome struct {
	id       string
	trigger  string
	started  time.Time
	result   BatchResult
	err      error
	category string
}

// Run blocks until ctx is canceled, then cancels and boundedly waits for a batch.
func (e *Engine) Run(ctx context.Context) error {
	if e.Batch == nil {
		return fmt.Errorf("scheduler: batch is required")
	}
	log := e.Log
	if log == nil {
		log = slog.Default()
	}
	clock := e.Clock
	if clock == nil {
		clock = realClock{}
	}
	random := e.Random
	if random == nil {
		random = realRandom{}
	}

	timer := clock.NewTimer(e.Config.Interval)
	defer timer.Stop()
	done := make(chan runOutcome, 1)
	var cancelRun context.CancelFunc
	failures := 0

	start := func(trigger string) {
		e.mu.Lock()
		if e.active {
			e.mu.Unlock()
			next := clock.Now().Add(e.Config.Interval)
			log.Warn("scheduler_trigger_skipped",
				"reason", "overlap",
				"trigger", trigger,
				"skipped_overlap", true,
				"next_run_at", next.UTC().Format(time.RFC3339),
			)
			timer.Reset(e.Config.Interval)
			return
		}
		e.active = true
		e.mu.Unlock()

		runCtx, cancel := context.WithCancel(ctx)
		cancelRun = cancel
		schedulerRunID := newSchedulerRunID(clock.Now())
		started := clock.Now()
		log.Info("scheduler_run_started",
			"scheduler_run_id", schedulerRunID,
			"trigger", trigger,
			"source", "hh",
		)
		go func() {
			result, err := e.Batch(runCtx, schedulerRunID)
			category := errorCategory(err)
			done <- runOutcome{
				id: schedulerRunID, trigger: trigger, started: started,
				result: result, err: err, category: category,
			}
		}()
	}

	if e.Config.RunOnStart {
		start("startup")
	}

	for {
		select {
		case <-ctx.Done():
			timer.Stop()
			if cancelRun != nil {
				cancelRun()
			}
			e.mu.Lock()
			active := e.active
			e.mu.Unlock()
			if !active {
				return nil
			}
			wait := clock.NewTimer(e.Config.ShutdownTimeout)
			defer wait.Stop()
			select {
			case outcome := <-done:
				e.finish(log, clock, outcome)
				return nil
			case <-wait.C():
				log.Error("scheduler_shutdown_timeout",
					"timeout_ms", e.Config.ShutdownTimeout.Milliseconds(),
				)
				return context.DeadlineExceeded
			}
		case <-timer.C():
			start("interval")
			timer.Reset(e.Config.Interval)
		case outcome := <-done:
			e.mu.Lock()
			e.active = false
			e.mu.Unlock()
			cancelRun = nil
			e.finish(log, clock, outcome)
			delay := e.Config.Interval
			if errors.Is(outcome.err, ErrLockUnavailable) {
				failures = 0
			} else if outcome.err != nil {
				failures++
				delay = backoff(e.Config, failures, random.Float64())
			} else {
				failures = 0
			}
			next := clock.Now().Add(delay)
			log.Info("scheduler_next_run_scheduled",
				"next_run_at", next.UTC().Format(time.RFC3339),
				"delay_ms", delay.Milliseconds(),
				"consecutive_failures", failures,
			)
			timer.Reset(delay)
		}
	}
}

func (e *Engine) finish(log *slog.Logger, clock Clock, outcome runOutcome) {
	e.mu.Lock()
	if e.active {
		e.active = false
	}
	e.mu.Unlock()
	attrs := []any{
		"scheduler_run_id", outcome.id,
		"ingest_run_id", outcome.result.IngestRunID,
		"trigger", outcome.trigger,
		"duration_ms", clock.Now().Sub(outcome.started).Milliseconds(),
		"fetched", outcome.result.Stats.Fetched,
		"upserted", outcome.result.Stats.Upserted,
		"unchanged", outcome.result.Stats.Unchanged,
		"excluded_out_of_scope", outcome.result.Stats.Excluded,
		"errors", outcome.result.Stats.Errors,
		"pages", outcome.result.Stats.Pages,
		"cycle_complete", outcome.result.CycleComplete,
	}
	if errors.Is(outcome.err, ErrLockUnavailable) {
		log.Info("scheduler_trigger_skipped",
			"scheduler_run_id", outcome.id,
			"trigger", outcome.trigger,
			"reason", "lock",
			"skipped_locked", true,
		)
		return
	}
	if outcome.err != nil {
		attrs = append(attrs, "error_category", outcome.category, "stage", "batch")
		log.Error("scheduler_run_failed", attrs...)
		return
	}
	log.Info("scheduler_run_finished", attrs...)
}

func backoff(cfg Config, failures int, value float64) time.Duration {
	delay := cfg.BackoffInitial
	for i := 1; i < failures && delay < cfg.BackoffMax; i++ {
		if delay > cfg.BackoffMax/2 {
			delay = cfg.BackoffMax
			break
		}
		delay *= 2
	}
	if delay > cfg.BackoffMax {
		delay = cfg.BackoffMax
	}
	spread := float64(delay) * cfg.JitterPercent / 100
	jittered := float64(delay) - spread + 2*spread*value
	if jittered < 0 {
		return 0
	}
	return time.Duration(jittered)
}

func errorCategory(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	default:
		return "dependency"
	}
}

func newSchedulerRunID(now time.Time) string {
	return fmt.Sprintf("sched_%d", now.UTC().UnixNano())
}

type realClock struct{}

func (realClock) Now() time.Time                 { return time.Now() }
func (realClock) NewTimer(d time.Duration) Timer { return &realTimer{timer: time.NewTimer(d)} }

type realTimer struct{ timer *time.Timer }

func (t *realTimer) C() <-chan time.Time { return t.timer.C }
func (t *realTimer) Reset(d time.Duration) {
	if !t.timer.Stop() {
		select {
		case <-t.timer.C:
		default:
		}
	}
	t.timer.Reset(d)
}
func (t *realTimer) Stop() { t.timer.Stop() }

type realRandom struct{}

func (realRandom) Float64() float64 { return rand.Float64() }
