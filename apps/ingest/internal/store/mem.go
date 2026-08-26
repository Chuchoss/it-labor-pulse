package store

import (
	"context"
	"fmt"
	"sync"
)

// Memory is an in-memory Store for unit tests (no Postgres).
type Memory struct {
	mu                  sync.Mutex
	Runs                map[string]Run
	Checkpoints         map[string]string // source|scope → cursor
	Vacancies           map[string]VacancyWrite
	Cycles              map[string]Cycle
	DiscoveryPartitions map[string][]DiscoveryPartition
	Observations        map[string]DiscoveryObservation
	Errors              []RunError
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
		Runs:                make(map[string]Run),
		Checkpoints:         make(map[string]string),
		Vacancies:           make(map[string]VacancyWrite),
		Cycles:              make(map[string]Cycle),
		DiscoveryPartitions: make(map[string][]DiscoveryPartition),
		Observations:        make(map[string]DiscoveryObservation),
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
		"fetched":               stats.Fetched,
		"upserted":              stats.Upserted,
		"unchanged":             stats.Unchanged,
		"excluded_out_of_scope": stats.Excluded,
		"errors":                stats.Errors,
		"pages":                 stats.Pages,
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

func (m *Memory) StartCycle(_ context.Context, cycle Cycle) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, existing := range m.Cycles {
		if existing.Source == cycle.Source &&
			existing.ScopeHash == cycle.ScopeHash &&
			existing.CycleEnd.Equal(cycle.CycleEnd) {
			return id, nil
		}
	}
	cycle.ID = fmt.Sprintf("00000000-0000-4000-8000-%012d", len(m.Cycles)+1)
	cycle.Status = "running"
	m.Cycles[cycle.ID] = cycle
	return cycle.ID, nil
}

func (m *Memory) UpdateCycleProgress(_ context.Context, id string, completedPartitions int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cycle, ok := m.Cycles[id]
	if !ok {
		return fmt.Errorf("cycle not found: %s", id)
	}
	cycle.CompletedPartitions = completedPartitions
	m.Cycles[id] = cycle
	return nil
}

func (m *Memory) CompleteCycle(_ context.Context, id string, completedPartitions int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cycle, ok := m.Cycles[id]
	if !ok {
		return fmt.Errorf("cycle not found: %s", id)
	}
	if completedPartitions != cycle.PartitionCount {
		return fmt.Errorf("cycle incomplete: %d/%d", completedPartitions, cycle.PartitionCount)
	}
	cycle.CompletedPartitions = completedPartitions
	cycle.Status = "complete"
	m.Cycles[id] = cycle
	return nil
}

func (m *Memory) PendingDiscoveryCycle(
	_ context.Context,
	source, methodVersion string,
) (Cycle, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var selected Cycle
	found := false
	for _, cycle := range m.Cycles {
		if cycle.Source != source || cycle.Scope != "daily_discovery" ||
			cycle.MethodVersion != methodVersion || cycle.Status != "running" {
			continue
		}
		if !found || cycle.CycleDate.Before(selected.CycleDate) {
			selected, found = cycle, true
		}
	}
	return selected, found, nil
}

func (m *Memory) StartDiscoveryCycle(
	_ context.Context,
	cycle Cycle,
	partitions []DiscoveryPartition,
) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, existing := range m.Cycles {
		if existing.Source == cycle.Source && existing.Scope == "daily_discovery" &&
			existing.CycleDate.Equal(cycle.CycleDate) &&
			existing.MethodVersion == cycle.MethodVersion {
			return id, nil
		}
	}
	cycle.ID = fmt.Sprintf("10000000-0000-4000-8000-%012d", len(m.Cycles)+1)
	cycle.Scope = "daily_discovery"
	cycle.Status = "running"
	cycle.PartitionCount = len(partitions)
	m.Cycles[cycle.ID] = cycle
	m.DiscoveryPartitions[cycle.ID] = append([]DiscoveryPartition(nil), partitions...)
	return cycle.ID, nil
}

func (m *Memory) NextDiscoveryPartition(
	_ context.Context,
	cycleID string,
) (DiscoveryPartition, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, part := range m.DiscoveryPartitions[cycleID] {
		if part.Status != "complete" {
			return part, true, nil
		}
	}
	return DiscoveryPartition{}, false, nil
}

func (m *Memory) SetDiscoveryExpectedPages(
	_ context.Context,
	cycleID, partitionKey string,
	expectedPages int,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	parts := m.DiscoveryPartitions[cycleID]
	for i := range parts {
		if parts[i].Key != partitionKey || parts[i].NextPage != 0 {
			continue
		}
		delta := expectedPages - parts[i].ExpectedPages
		parts[i].ExpectedPages = expectedPages
		cycle := m.Cycles[cycleID]
		cycle.ExpectedPages += delta
		if expectedPages == 0 && parts[i].Status != "complete" {
			parts[i].Status = "complete"
			cycle.CompletedPartitions++
		}
		m.Cycles[cycleID] = cycle
	}
	m.DiscoveryPartitions[cycleID] = parts
	return nil
}

func (m *Memory) SaveDiscoveryPage(
	_ context.Context,
	cycleID string,
	part DiscoveryPartition,
	observations []DiscoveryObservation,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	parts := m.DiscoveryPartitions[cycleID]
	for i := range parts {
		if parts[i].Key != part.Key {
			continue
		}
		if part.NextPage < parts[i].NextPage {
			return nil
		}
		parts[i].NextPage = part.NextPage + 1
		if parts[i].NextPage == parts[i].ExpectedPages {
			parts[i].Status = "complete"
			cycle := m.Cycles[cycleID]
			cycle.CompletedPartitions++
			m.Cycles[cycleID] = cycle
		} else {
			parts[i].Status = "running"
		}
		cycle := m.Cycles[cycleID]
		cycle.CompletedPages++
		m.Cycles[cycleID] = cycle
		break
	}
	m.DiscoveryPartitions[cycleID] = parts
	for _, observation := range observations {
		m.Observations[cycleID+"|"+observation.ExternalID] = observation
	}
	return nil
}

func (m *Memory) CompleteDiscoveryCycle(_ context.Context, cycleID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cycle, ok := m.Cycles[cycleID]
	if !ok || cycle.CompletedPages != cycle.ExpectedPages ||
		cycle.CompletedPartitions != cycle.PartitionCount {
		return fmt.Errorf("discovery cycle incomplete")
	}
	cycle.Status = "complete"
	m.Cycles[cycleID] = cycle
	return nil
}

func (m *Memory) FailDiscoveryCycle(_ context.Context, cycleID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cycle := m.Cycles[cycleID]
	cycle.Status = "failed"
	m.Cycles[cycleID] = cycle
	return nil
}

func (m *Memory) SyncRoles(_ context.Context, source string, roles []SourceRole) (map[string]string, error) {
	result := make(map[string]string, len(roles))
	for _, role := range roles {
		result[role.ExternalID] = source + "-role-" + role.ExternalID
	}
	return result, nil
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
