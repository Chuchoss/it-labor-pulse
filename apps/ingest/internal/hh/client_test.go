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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		switch {
		case r.URL.Path == "/vacancies" && r.Method == http.MethodGet:
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
