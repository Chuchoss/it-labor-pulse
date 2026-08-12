package store

import (
	"context"
	"fmt"
	"sync"
)

// Memory is an in-memory Store for unit tests (no Postgres).
type Memory struct {
	mu          sync.Mutex
	Runs        map[string]Run
	Checkpoints map[string]string // source|scope → cursor
	Vacancies   map[string]VacancyWrite
	Errors      []RunError
}

// RunError is a recorded ingest_run_errors row.
type RunError struct {
	RunID      string
	ExternalID string
	Stage      string
	Message    string
}

// NewMemory returns an empty Memory store.
func NewMemory() *Memory {
	return &Memory{
		Runs:        make(map[string]Run),
		Checkpoints: make(map[string]string),
		Vacancies:   make(map[string]VacancyWrite),
	}
}

func (m *Memory) CreateRun(_ context.Context, run Run) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.Runs[run.ID]; ok {
		return fmt.Errorf("run exists: %s", run.ID)
	}
	m.Runs[run.ID] = run
	return nil
}

func (m *Memory) FinishRun(_ context.Context, id, status string, stats Stats, errMsg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	run, ok := m.Runs[id]
	if !ok {
		return fmt.Errorf("run not found: %s", id)
	}
	run.Status = status
	run.Stats = map[string]any{
		"fetched":   stats.Fetched,
		"upserted":  stats.Upserted,
		"unchanged": stats.Unchanged,
		"errors":    stats.Errors,
		"pages":     stats.Pages,
	}
	run.ErrorMsg = errMsg
	m.Runs[id] = run
	return nil
}

func (m *Memory) RecordError(_ context.Context, runID, externalID, stage, message string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Errors = append(m.Errors, RunError{RunID: runID, ExternalID: externalID, Stage: stage, Message: message})
	return nil
}

func (m *Memory) GetCheckpoint(_ context.Context, source, scopeHash string) (string, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cur, ok := m.Checkpoints[source+"|"+scopeHash]
	return cur, ok, nil
}

func (m *Memory) SavePage(_ context.Context, source, scopeHash, nextCursor string, items []VacancyWrite) (int, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	upserted, unchanged := 0, 0
	for _, item := range items {
		key := item.Vacancy.Source + "|" + item.Vacancy.ExternalID
		if prev, ok := m.Vacancies[key]; ok && prev.Vacancy.ContentHash == item.Vacancy.ContentHash {
			unchanged++
			prev.Vacancy.CollectedAt = item.Vacancy.CollectedAt
			m.Vacancies[key] = prev
			continue
		}
		m.Vacancies[key] = item
		upserted++
	}
	m.Checkpoints[source+"|"+scopeHash] = nextCursor
	return upserted, unchanged, nil
}

func (m *Memory) Close() error { return nil }
