package pipeline_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Chuchoss/it-labor-pulse/apps/ingest/internal/hh"
	"github.com/Chuchoss/it-labor-pulse/apps/ingest/internal/pipeline"
	"github.com/Chuchoss/it-labor-pulse/apps/ingest/internal/store"
)

type pagingSource struct {
	pages  int
	detail []byte
	seen   []int
}

func (s *pagingSource) SearchVacancies(_ context.Context, q hh.SearchQuery) (hh.SearchPage, error) {
	s.seen = append(s.seen, q.Page)
	return hh.SearchPage{
		Found:   s.pages,
		Page:    q.Page,
		Pages:   s.pages,
		PerPage: q.PerPage,
		Items:   []hh.SearchItem{{ID: "900001", Name: "Senior Go Developer"}},
	}, nil
}

func (s *pagingSource) GetVacancyRaw(context.Context, string) ([]byte, error) {
	return s.detail, nil
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, dir, parent)
		dir = parent
	}
}

func TestRunner_FixtureSource(t *testing.T) {
	mem := store.NewMemory()
	src := hh.NewFixtureSource(filepath.Join(moduleRoot(t), "testdata", "hh"))
	r := &pipeline.Runner{
		Source: src,
		Store:  mem,
		Now:    func() time.Time { return time.Date(2026, 8, 12, 15, 0, 0, 0, time.UTC) },
	}
	res, err := r.Run(context.Background(), pipeline.Params{
		Area:     "113",
		Mode:     "incremental",
		MaxPages: 2,
	})
	require.NoError(t, err)
	require.Equal(t, store.StatusSuccess, res.Status)
	require.Equal(t, 2, res.Stats.Fetched)
	require.Equal(t, 2, res.Stats.Upserted)
	require.Contains(t, mem.Vacancies, "hh|900001")
	require.Contains(t, mem.Vacancies, "hh|900002")
	require.Equal(t, "Senior Go Developer", mem.Vacancies["hh|900001"].Vacancy.Title)
	require.Equal(t, "1", mem.Vacancies["hh|900001"].Vacancy.RegionExternalID)
	require.Equal(t, "Москва", mem.Vacancies["hh|900001"].RegionName)
	require.Equal(t, "2", mem.Vacancies["hh|900002"].Vacancy.RegionExternalID)
	require.Equal(t, "Санкт-Петербург", mem.Vacancies["hh|900002"].RegionName)

	// A Russia-wide search area is only the search scope. Repeating the same
	// vacancy payloads under another checkpoint scope remains idempotent and
	// preserves each vacancy's own HH area.
	repeated, err := r.Run(context.Background(), pipeline.Params{
		Area:     "113",
		Text:     "idempotency-check",
		Mode:     "incremental",
		MaxPages: 2,
	})
	require.NoError(t, err)
	require.Zero(t, repeated.Stats.Upserted)
	require.Equal(t, 2, repeated.Stats.Unchanged)
	require.Len(t, mem.Vacancies, 2)
}

func TestRunner_httptest_WithBackoffPath(t *testing.T) {
	root := filepath.Join(moduleRoot(t), "testdata", "hh")
	search, err := os.ReadFile(filepath.Join(root, "vacancy_search_page.json"))
	require.NoError(t, err)
	detail, err := os.ReadFile(filepath.Join(root, "vacancy_detail.json"))
	require.NoError(t, err)

	// Minimal search with one item so detail map is enough.
	miniSearch := []byte(`{"found":1,"page":0,"pages":1,"per_page":20,"items":[{"id":"900001","name":"Senior Go Developer"}]}`)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/vacancies":
			_, _ = w.Write(miniSearch)
		case "/vacancies/900001":
			_, _ = w.Write(detail)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	_ = search

	client, err := hh.NewClient(hh.ClientOptions{
		BaseURL:   srv.URL,
		UserAgent: "LMATest/0.1 (+test@example.com)",
	})
	require.NoError(t, err)

	mem := store.NewMemory()
	r := &pipeline.Runner{Source: client, Store: mem}
	res, err := r.Run(context.Background(), pipeline.Params{
		Area: "1", Text: "go", MaxPages: 1,
	})
	require.NoError(t, err)
	require.Equal(t, store.StatusSuccess, res.Status)
	require.Equal(t, 1, res.Stats.Upserted)
}

func TestRunner_PageErrorDoesNotAdvanceCheckpoint(t *testing.T) {
	root := filepath.Join(moduleRoot(t), "testdata", "hh")
	search, err := os.ReadFile(filepath.Join(root, "vacancy_search_page.json"))
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/vacancies" {
			_, _ = w.Write(search)
			return
		}
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	client, err := hh.NewClient(hh.ClientOptions{
		BaseURL:    srv.URL,
		UserAgent:  "LMATest/0.1 (+test@example.com)",
		MaxRetries: 0,
		Sleep:      func(context.Context, time.Duration) error { return nil },
	})
	require.NoError(t, err)

	mem := store.NewMemory()
	r := &pipeline.Runner{Source: client, Store: mem}
	res, err := r.Run(context.Background(), pipeline.Params{MaxPages: 1})
	require.NoError(t, err) // run finishes; status failed/partial
	require.NotEqual(t, store.StatusSuccess, res.Status)
	require.Empty(t, mem.Checkpoints)
	require.Empty(t, mem.Vacancies)
}

func TestResolvePageLimit(t *testing.T) {
	require.Equal(t, 20, pipeline.ResolvePageLimit(0, 100))
	require.Equal(t, 5, pipeline.ResolvePageLimit(5, 100))
	require.Equal(t, 20, pipeline.ResolvePageLimit(50, 100))
	require.Equal(t, 100, pipeline.ResolvePageLimit(0, 1))
}

func TestRunner_AllModeStopsAtAPIPageExhaustionAndResumes(t *testing.T) {
	detail, err := os.ReadFile(filepath.Join(moduleRoot(t), "testdata", "hh", "vacancy_detail.json"))
	require.NoError(t, err)
	src := &pagingSource{pages: 3, detail: detail}
	mem := store.NewMemory()
	r := &pipeline.Runner{Source: src, Store: mem}

	first, err := r.Run(context.Background(), pipeline.Params{
		Area: "1", Text: "golang", MaxPages: 1, PerPage: 100,
	})
	require.NoError(t, err)
	require.Equal(t, 1, first.Stats.Pages)
	require.Equal(t, []int{0}, src.seen)

	second, err := r.Run(context.Background(), pipeline.Params{
		Area: "1", Text: "golang", MaxPages: 0, PerPage: 100,
	})
	require.NoError(t, err)
	require.Equal(t, 2, second.Stats.Pages)
	require.Equal(t, []int{0, 1, 2}, src.seen)
}
