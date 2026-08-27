package assistant

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type processHeartbeatFake struct {
	mu        sync.Mutex
	beats     []WorkerProcessHeartbeat
	failures  int
	stoppedID string
}

func (f *processHeartbeatFake) UpsertWorkerProcessHeartbeat(
	_ context.Context,
	heartbeat WorkerProcessHeartbeat,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failures > 0 {
		f.failures--
		return errors.New("temporary database failure")
	}
	f.beats = append(f.beats, heartbeat)
	return nil
}

func (f *processHeartbeatFake) StopWorkerProcess(_ context.Context, instanceID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stoppedID = instanceID
	return nil
}

func (f *processHeartbeatFake) snapshot() ([]WorkerProcessHeartbeat, string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]WorkerProcessHeartbeat(nil), f.beats...), f.stoppedID
}

func TestProcessHeartbeatContinuesAcrossIdleProcessingAndBackoff(t *testing.T) {
	store := &processHeartbeatFake{}
	heartbeat, err := StartProcessHeartbeat(context.Background(), store, ProcessHeartbeatOptions{
		InstanceID: "00000000-0000-4000-8000-000000000001",
		StartedAt:  time.Unix(100, 0),
		Version:    "test",
		Mode:       "continuous",
		Interval:   10 * time.Millisecond,
		RetryDelay: time.Millisecond,
	})
	require.NoError(t, err)
	t.Cleanup(heartbeat.Stop)

	require.Eventually(t, func() bool {
		beats, _ := store.snapshot()
		return len(beats) >= 2 && beats[len(beats)-1].State == "idle"
	}, time.Second, time.Millisecond)

	heartbeat.SetState("processing")
	require.Eventually(t, func() bool {
		beats, _ := store.snapshot()
		return len(beats) >= 3 && beats[len(beats)-1].State == "processing"
	}, time.Second, time.Millisecond)

	// A provider wait does not block the independent heartbeat goroutine.
	before, _ := store.snapshot()
	time.Sleep(35 * time.Millisecond)
	after, _ := store.snapshot()
	require.GreaterOrEqual(t, len(after)-len(before), 3)

	heartbeat.SetState("backoff")
	require.Eventually(t, func() bool {
		beats, _ := store.snapshot()
		return beats[len(beats)-1].State == "backoff"
	}, time.Second, time.Millisecond)
}

func TestProcessHeartbeatRetriesDatabaseFailureAndMarksStopping(t *testing.T) {
	store := &processHeartbeatFake{failures: 2}
	heartbeat, err := StartProcessHeartbeat(context.Background(), store, ProcessHeartbeatOptions{
		InstanceID: "00000000-0000-4000-8000-000000000002",
		Interval:   10 * time.Millisecond,
		RetryDelay: time.Millisecond,
	})
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		beats, _ := store.snapshot()
		return len(beats) > 0
	}, time.Second, time.Millisecond)

	heartbeat.Stop()
	_, stoppedID := store.snapshot()
	require.Equal(t, heartbeat.InstanceID(), stoppedID)
}

func TestProcessHeartbeatRestartUsesNewInstance(t *testing.T) {
	first, err := StartProcessHeartbeat(context.Background(), &processHeartbeatFake{}, ProcessHeartbeatOptions{
		Interval: 10 * time.Millisecond,
	})
	require.NoError(t, err)
	first.Stop()

	second, err := StartProcessHeartbeat(context.Background(), &processHeartbeatFake{}, ProcessHeartbeatOptions{
		Interval: 10 * time.Millisecond,
	})
	require.NoError(t, err)
	second.Stop()
	require.NotEqual(t, first.InstanceID(), second.InstanceID())
}
