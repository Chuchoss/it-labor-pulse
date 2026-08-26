package hh_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Chuchoss/it-labor-pulse/apps/ingest/internal/hh"
)

func TestNewClient_EmptyUserAgentFails(t *testing.T) {
	_, err := hh.NewClient(hh.ClientOptions{BaseURL: "http://example.com"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "HH_USER_AGENT")
}

func TestClient_SearchAndDetail_httptest(t *testing.T) {
	search := readFixture(t, "vacancy_search_page.json")
	detail := readFixture(t, "vacancy_detail.json")

	var gotUA string
	var gotPerPage string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		switch {
		case r.URL.Path == "/vacancies" && r.Method == http.MethodGet:
			gotPerPage = r.URL.Query().Get("per_page")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(search)
		case r.URL.Path == "/vacancies/900001":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(detail)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c, err := hh.NewClient(hh.ClientOptions{
		BaseURL:   srv.URL,
		UserAgent: "LMATest/0.1 (+test@example.com)",
		PageDelay: 0,
	})
	require.NoError(t, err)

	page, err := c.SearchVacancies(context.Background(), hh.SearchQuery{Text: "golang", Area: "1", Page: 0})
	require.NoError(t, err)
	require.Len(t, page.Items, 2)
	require.Equal(t, "LMATest/0.1 (+test@example.com)", gotUA)
	require.Equal(t, "100", gotPerPage)

	raw, err := c.GetVacancyRaw(context.Background(), "900001")
	require.NoError(t, err)
	d, err := hh.DraftFromDetail(raw, time.Now().UTC())
	require.NoError(t, err)
	require.Equal(t, "900001", d.ExternalID)
}

func TestClient_RetriesOn429(t *testing.T) {
	detail := readFixture(t, "vacancy_detail.json")
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := hits.Add(1)
		if n == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(detail)
	}))
	defer srv.Close()

	var slept int
	c, err := hh.NewClient(hh.ClientOptions{
		BaseURL:   srv.URL,
		UserAgent: "LMATest/0.1 (+test@example.com)",
		Sleep: func(ctx context.Context, d time.Duration) error {
			slept++
			return nil
		},
		MaxRetries: 3,
	})
	require.NoError(t, err)

	raw, err := c.GetVacancyRaw(context.Background(), "900001")
	require.NoError(t, err)
	require.NotEmpty(t, raw)
	require.GreaterOrEqual(t, slept, 1)
	require.Equal(t, int32(2), hits.Load())
}

func TestClient_UsesRetryAfterAndCapsPageSize(t *testing.T) {
	var hits atomic.Int32
	var perPage string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		perPage = r.URL.Query().Get("per_page")
		if hits.Add(1) == 1 {
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"found":0,"page":0,"pages":0,"per_page":100,"items":[]}`))
	}))
	defer srv.Close()

	var sleeps []time.Duration
	c, err := hh.NewClient(hh.ClientOptions{
		BaseURL:   srv.URL,
		UserAgent: "LMATest/0.1 (+test@example.com)",
		Sleep: func(_ context.Context, d time.Duration) error {
			sleeps = append(sleeps, d)
			return nil
		},
		MaxRetries: 2,
	})
	require.NoError(t, err)

	_, err = c.SearchVacancies(context.Background(), hh.SearchQuery{PerPage: 500})
	require.NoError(t, err)
	require.Equal(t, "100", perPage)
	require.Contains(t, sleeps, 2*time.Second)
}
