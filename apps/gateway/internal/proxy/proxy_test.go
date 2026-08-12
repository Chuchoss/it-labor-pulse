package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReverseProxyForwardsAPI(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/health", r.URL.Path)
		require.Equal(t, "req-1", r.Header.Get("X-Request-Id"))
		w.Header().Set("X-Upstream", "bff")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	t.Cleanup(upstream.Close)

	u, err := url.Parse(upstream.URL)
	require.NoError(t, err)

	h := NewAPIReverseProxy(u)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	req.Header.Set("X-Request-Id", "req-1")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "bff", rec.Header().Get("X-Upstream"))
	body, _ := io.ReadAll(rec.Body)
	require.JSONEq(t, `{"status":"ok"}`, string(body))
}

func TestReverseProxyRejectsNonAPI(t *testing.T) {
	t.Parallel()

	u, err := url.Parse("http://127.0.0.1:9")
	require.NoError(t, err)
	h := NewAPIReverseProxy(u)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)
}
