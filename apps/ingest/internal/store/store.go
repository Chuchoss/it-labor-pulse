package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Chuchoss/it-labor-pulse/libs/go-common/normalize"
)

// RunStatus values for ingest_runs.status.
const (
	StatusQueued  = "queued"
	StatusRunning = "running"
	StatusSuccess = "success"
	StatusPartial = "partial"
	StatusFailed  = "failed"
)

// Run is a row in ingest_runs.
type Run struct {
	ID          string
	Source      string
	Mode        string
	Status      string
	Params      map[string]any
	RequestedBy string
	StartedAt   *time.Time
	FinishedAt  *time.Time
	Stats       map[string]any
	ErrorMsg    string
}

// VacancyWrite is a normalized vacancy ready for UPSERT.
type VacancyWrite struct {
	Vacancy    normalize.CanonicalVacancy
	RegionName string
	RawPayload json.RawMessage
}

// SourceRole is one canonical role derived from an official source taxonomy.
type SourceRole struct {
	ExternalID string
	Title      string
	Family     string
}

// Stats accumulates run counters.
type Stats struct {
	Fetched   int `json:"fetched"`
	Upserted  int `json:"upserted"`
	Unchanged int `json:"unchanged"`
	Excluded  int `json:"excluded_out_of_scope"`
	Errors    int `json:"errors"`
	Pages     int `json:"pages"`
}

// Cycle is the durable proof of one complete all-IT coverage pass.
type Cycle struct {
	ID                  string
	Source              string
	Scope               string
	ScopeHash           string
	CycleEnd            time.Time
	PartitionCount      int
	CompletedPartitions int
	Status              string
}

// Store persists ingest runs, checkpoints and vacancies.
type Store interface {
	CreateRun(ctx context.Context, run Run) error
	FinishRun(ctx context.Context, id, status string, stats Stats, errMsg string) error
	RecordError(ctx context.Context, runID, externalID, stage, message string) error
	GetCheckpoint(ctx context.Context, source, scopeHash string) (cursor string, ok bool, err error)
	StartCycle(ctx context.Context, cycle Cycle) (string, error)
	UpdateCycleProgress(ctx context.Context, id string, completedPartitions int) error
	CompleteCycle(ctx context.Context, id string, completedPartitions int) error
	SyncRoles(ctx context.Context, source string, roles []SourceRole) (map[string]string, error)
	// SavePage upserts all vacancies and advances checkpoint in one transaction.
	SavePage(ctx context.Context, source, scopeHash, nextCursor string, items []VacancyWrite) (upserted, unchanged int, err error)
	Close() error
}
