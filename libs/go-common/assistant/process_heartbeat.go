package assistant

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

const (
	WorkerHeartbeatInterval = 10 * time.Second
	WorkerAvailabilityTTL   = 40 * time.Second
)

type WorkerProcessHeartbeat struct {
	InstanceID string
	StartedAt  time.Time
	Version    string
	Mode       string
	State      string
}

type WorkerProcessHeartbeatStore interface {
	UpsertWorkerProcessHeartbeat(context.Context, WorkerProcessHeartbeat) error
}

type WorkerProcessStopStore interface {
	StopWorkerProcess(context.Context, string) error
}

type WorkerProcessLockStore interface {
	TryProcessLock(context.Context) (release func() error, acquired bool, err error)
}

type ProcessHeartbeatOptions struct {
	InstanceID string
	StartedAt  time.Time
	Version    string
	Mode       string
	Interval   time.Duration
	RetryDelay time.Duration
	Log        *slog.Logger
}

type ProcessHeartbeat struct {
	store      WorkerProcessHeartbeatStore
	stopStore  WorkerProcessStopStore
	info       WorkerProcessHeartbeat
	interval   time.Duration
	retryDelay time.Duration
	log        *slog.Logger
	cancel     context.CancelFunc
	done       chan struct{}
	mu         sync.RWMutex
	stopOnce   sync.Once
}

func StartProcessHeartbeat(
	ctx context.Context,
	store WorkerProcessHeartbeatStore,
	opts ProcessHeartbeatOptions,
) (*ProcessHeartbeat, error) {
	if store == nil {
		return nil, fmt.Errorf("worker process heartbeat store is required")
	}
	if opts.InstanceID == "" {
		var err error
		opts.InstanceID, err = newWorkerInstanceID()
		if err != nil {
			return nil, err
		}
	}
	if opts.StartedAt.IsZero() {
		opts.StartedAt = time.Now().UTC()
	}
	if opts.Version == "" {
		opts.Version = "dev"
	}
	if opts.Mode == "" {
		opts.Mode = "continuous"
	}
	if opts.Interval <= 0 {
		opts.Interval = WorkerHeartbeatInterval
	}
	if opts.RetryDelay <= 0 || opts.RetryDelay > opts.Interval {
		opts.RetryDelay = 2 * time.Second
	}
	heartbeatCtx, cancel := context.WithCancel(ctx)
	h := &ProcessHeartbeat{
		store: store,
		info: WorkerProcessHeartbeat{
			InstanceID: opts.InstanceID,
			StartedAt:  opts.StartedAt.UTC(),
			Version:    opts.Version,
			Mode:       opts.Mode,
			State:      "idle",
		},
		interval: opts.Interval, retryDelay: opts.RetryDelay, log: opts.Log,
		cancel: cancel, done: make(chan struct{}),
	}
	h.stopStore, _ = store.(WorkerProcessStopStore)
	go h.loop(heartbeatCtx)
	return h, nil
}

func (h *ProcessHeartbeat) InstanceID() string {
	return h.info.InstanceID
}

func (h *ProcessHeartbeat) SetState(state string) {
	switch state {
	case "idle", "processing", "backoff":
	default:
		state = "idle"
	}
	h.mu.Lock()
	h.info.State = state
	h.mu.Unlock()
}

func (h *ProcessHeartbeat) Stop() {
	h.stopOnce.Do(func() {
		h.cancel()
		<-h.done
		if h.stopStore == nil {
			return
		}
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := h.stopStore.StopWorkerProcess(stopCtx, h.info.InstanceID); err != nil && h.log != nil {
			h.log.Warn("assistant_worker_process_stop_failed", "category", "database")
		}
	})
}

func (h *ProcessHeartbeat) loop(ctx context.Context) {
	defer close(h.done)
	delay := time.Duration(0)
	for {
		if delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
		h.mu.RLock()
		info := h.info
		h.mu.RUnlock()
		beatCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := h.store.UpsertWorkerProcessHeartbeat(beatCtx, info)
		cancel()
		if err != nil {
			if h.log != nil {
				h.log.Warn("assistant_worker_process_heartbeat_failed", "category", "database")
			}
			delay = h.retryDelay
		} else {
			delay = h.interval
		}
	}
}

func newWorkerInstanceID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("create worker instance id: %w", err)
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		value[0:4], value[4:6], value[6:8], value[8:10], value[10:16]), nil
}
