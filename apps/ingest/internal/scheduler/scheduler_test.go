package scheduler

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type fakeTimer struct {
	ch     chan time.Time
	resets chan time.Duration
}

func newFakeTimer() *fakeTimer {
	return &fakeTimer{ch: make(chan time.Time, 8), resets: make(chan time.Duration, 16)}
}
func (t *fakeTimer) C() <-chan time.Time   { return t.ch }
func (t *fakeTimer) Reset(d time.Duration) { t.resets <- d }
func (t *fakeTimer) Stop()                 {}
func (t *fakeTimer) fire(at time.Time)     { t.ch <- at }

type fakeClock struct {
	now    time.Time
	mu     sync.Mutex
	timers []*fakeTimer
}

func (c *fakeClock) Now() time.Time { return c.now }
func (c *fakeClock) NewTimer(time.Duration) Timer {
	c.mu.Lock()
	defer c.mu.Unlock()
	timer := newFakeTimer()
	c.timers = append(c.timers, timer)
	return timer
}
func (c *fakeClock) timer(index int) *fakeTimer {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.timers[index]
}

type fixedRandom float64

func (r fixedRandom) Float64() float64 { return float64(r) }

func testConfig() Config {
	return Config{
		Interval: time.Minute, RunOnStart: true,
		BackoffInitial: time.Second, BackoffMax: 8 * time.Second,
		JitterPercent: 20, ShutdownTimeout: time.Second,
	}
}

func TestEngineRunOnStartAndPeriodicTick(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC)}
	calls := make(chan string, 2)
	engine := Engine{
		Config: testConfig(), Clock: clock, Random: fixedRandom(.5),
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Batch: func(_ context.Context, id string) (BatchResult, error) {
			calls <- id
			return BatchResult{}, nil
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- engine.Run(ctx) }()

	require.NotEmpty(t, receive(t, calls))
	clock.timer(0).fire(clock.now.Add(time.Minute))
	require.NotEmpty(t, receive(t, calls))
	cancel()
	require.NoError(t, receive(t, done))
}

func TestEngineSkipsOverlapAndNeverRunsConcurrently(t *testing.T) {
	clock := &fakeClock{now: time.Now()}
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	var active atomic.Int32
	var maxActive atomic.Int32
	engine := Engine{
		Config: testConfig(), Clock: clock, Random: fixedRandom(.5),
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Batch: func(ctx context.Context, _ string) (BatchResult, error) {
			current := active.Add(1)
			if current > maxActive.Load() {
				maxActive.Store(current)
			}
			started <- struct{}{}
			select {
			case <-release:
			case <-ctx.Done():
			}
			active.Add(-1)
			return BatchResult{}, nil
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- engine.Run(ctx) }()
	receive(t, started)
	clock.timer(0).fire(clock.now.Add(time.Minute))
	select {
	case <-started:
		t.Fatal("overlapping batch started")
	default:
	}
	close(release)
	cancel()
	require.NoError(t, receive(t, done))
	require.Equal(t, int32(1), maxActive.Load())
}

func TestEngineCancelsActiveBatchOnShutdown(t *testing.T) {
	clock := &fakeClock{now: time.Now()}
	started := make(chan struct{})
	canceled := make(chan struct{})
	engine := Engine{
		Config: testConfig(), Clock: clock,
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Batch: func(ctx context.Context, _ string) (BatchResult, error) {
			close(started)
			<-ctx.Done()
			close(canceled)
			return BatchResult{}, ctx.Err()
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- engine.Run(ctx) }()
	receive(t, started)
	cancel()
	receive(t, canceled)
	require.NoError(t, receive(t, done))
}

func TestWithLockReleaseOnSuccessErrorAndCancel(t *testing.T) {
	tests := []struct {
		name     string
		batchErr error
	}{
		{name: "success"},
		{name: "error", batchErr: errors.New("failed")},
		{name: "cancel", batchErr: context.Canceled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			released := 0
			batch := WithLock(
				func(context.Context) (ReleaseLock, bool, error) {
					return func(context.Context) error { released++; return nil }, true, nil
				},
				func(context.Context, string) (BatchResult, error) {
					return BatchResult{}, tt.batchErr
				},
			)
			_, err := batch(context.Background(), "run")
			require.ErrorIs(t, err, tt.batchErr)
			require.Equal(t, 1, released)
		})
	}
}

func TestWithLockUnavailableSkipsBatch(t *testing.T) {
	called := false
	batch := WithLock(
		func(context.Context) (ReleaseLock, bool, error) { return nil, false, nil },
		func(context.Context, string) (BatchResult, error) {
			called = true
			return BatchResult{}, nil
		},
	)
	_, err := batch(context.Background(), "run")
	require.ErrorIs(t, err, ErrLockUnavailable)
	require.False(t, called)
}

func TestBackoffProgressionResetAndJitterBounds(t *testing.T) {
	cfg := testConfig()
	require.Equal(t, time.Second, backoff(cfg, 1, .5))
	require.Equal(t, 2*time.Second, backoff(cfg, 2, .5))
	require.Equal(t, 8*time.Second, backoff(cfg, 8, .5))
	require.Equal(t, 800*time.Millisecond, backoff(cfg, 1, 0))
	require.Equal(t, 1200*time.Millisecond, backoff(cfg, 1, 1))
}

func TestConfigValidationRejectsDangerousInterval(t *testing.T) {
	cfg := testConfig()
	require.Error(t, cfg.Validate(false))
	require.NoError(t, cfg.Validate(true))
	cfg.JitterPercent = 101
	require.Error(t, cfg.Validate(true))
}

func receive[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for test event")
		var zero T
		return zero
	}
}
