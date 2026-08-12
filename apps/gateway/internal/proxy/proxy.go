package proxy

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

// NewAPIReverseProxy returns a reverse proxy for /api/* → upstream (BFF).
// Path and query are preserved; Host is set to the upstream host.
func NewAPIReverseProxy(upstream *url.URL) http.Handler {
	rp := httputil.NewSingleHostReverseProxy(upstream)
	originalDirector := rp.Director
	rp.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = upstream.Host
		req.URL.Host = upstream.Host
		req.URL.Scheme = upstream.Scheme
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		rp.ServeHTTP(w, r)
	})
}
