package pipeline_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Chuchoss/it-labor-pulse/apps/ingest/internal/hh"
	"github.com/Chuchoss/it-labor-pulse/apps/ingest/internal/pipeline"
	"github.com/Chuchoss/it-labor-pulse/apps/ingest/internal/store"
)

type resumableITSource struct {
	detail       []byte
	catalogCalls int
	detailCalls  int
	failFirst    bool
}

func (s *resumableITSource) ProfessionalRoles(context.Context, string) ([]hh.ProfessionalRole, error) {
	s.catalogCalls++
	return allowedCatalog(), nil
}

func (s *resumableITSource) SearchVacancies(_ context.Context, q hh.SearchQuery) (hh.SearchPage, error) {
	found := 0
	items := []hh.SearchItem(nil)
	if q.ProfessionalRole == "96" {
		found = 1
		if q.PerPage > 1 {
			items = []hh.SearchItem{{ID: "900001"}}
		}
	}
	return hh.SearchPage{Found: found, Page: q.Page, Pages: 1, PerPage: q.PerPage, Items: items}, nil
}

func (s *resumableITSource) GetVacancyRaw(context.Context, string) ([]byte, error) {
	s.detailCalls++
	if s.failFirst && s.detailCalls == 1 {
		return nil, errors.New("temporary detail failure")
	}
	return s.detail, nil
}

func TestRunITBatchResumesCycleCheckpointAfterFailure(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "testdata", "hh", "vacancy_detail.json"))
	require.NoError(t, err)
	src := &resumableITSource{detail: raw, failFirst: true}
	st := store.NewMemory()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	opts := pipeline.ITBatchOptions{
		Area: "113", PerPage: 2, MaxDepth: 4, MaxPartitions: 20,
		MaxBatchParts: 20, MaxPagesPerPart: 5, MaxRequests: 30,
		Now: time.Date(2026, 8, 26, 9, 0, 0, 0, time.UTC),
	}

	first, err := pipeline.RunITBatch(context.Background(), src, st, log, opts)
	require.Error(t, err)
	require.False(t, first.CycleComplete)
	require.Equal(t, 1, src.catalogCalls)

	second, err := pipeline.RunITBatch(context.Background(), src, st, log, opts)
	require.NoError(t, err)
	require.True(t, second.CycleComplete)
	require.Equal(t, 1, src.catalogCalls, "persisted plan must be reused")
	require.Equal(t, 2, src.detailCalls)
}
