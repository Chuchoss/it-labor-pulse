package httpx

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const (
	headerRequestID   = "X-Request-Id"
	headerTraceparent = "Traceparent"
)

// Middleware attaches request_id / trace_id, echoes correlation headers,
// and logs http_request as structured JSON (docs/architecture/18, 23).
func Middleware(log *slog.Logger) func(http.Handler) http.Handler {
	if log == nil {
		log = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := strings.TrimSpace(r.Header.Get(headerRequestID))
			if requestID == "" {
				requestID = newUUID()
			}

			traceID, spanID, flags := ensureTrace(r.Header.Get(headerTraceparent))
			traceparent := formatTraceparent(traceID, spanID, flags)

			ctx := withRequestID(r.Context(), requestID)
			ctx = withTraceID(ctx, traceID)
			r = r.WithContext(ctx)

			// Echo to client and forward to upstream (e.g. gateway → bff).
			w.Header().Set(headerRequestID, requestID)
			w.Header().Set(headerTraceparent, traceparent)
			r.Header.Set(headerRequestID, requestID)
			r.Header.Set(headerTraceparent, traceparent)

			reqLog := log.With(
				"request_id", requestID,
				"trace_id", traceID,
			)

			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			start := time.Now()
			next.ServeHTTP(sw, r)
			reqLog.Info("http_request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", sw.status,
				"latency_ms", time.Since(start).Milliseconds(),
			)
		})
	}
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func ensureTrace(header string) (traceID, spanID, flags string) {
	traceID, _, flags, ok := parseTraceparent(header)
	if ok {
		return traceID, newSpanID(), flags
	}
	return newTraceID(), newSpanID(), "01"
}

func parseTraceparent(h string) (traceID, parentID, flags string, ok bool) {
	h = strings.TrimSpace(h)
	if h == "" {
		return "", "", "", false
	}
	parts := strings.Split(h, "-")
	if len(parts) != 4 {
		return "", "", "", false
	}
	version, tid, pid, fl := parts[0], parts[1], parts[2], parts[3]
	if version != "00" || len(tid) != 32 || !isHex(tid) || isZeroHex(tid) {
		return "", "", "", false
	}
	if len(pid) != 16 || !isHex(pid) || isZeroHex(pid) {
		return "", "", "", false
	}
	if len(fl) != 2 || !isHex(fl) {
		return "", "", "", false
	}
	return strings.ToLower(tid), strings.ToLower(pid), strings.ToLower(fl), true
}

func formatTraceparent(traceID, spanID, flags string) string {
	return "00-" + traceID + "-" + spanID + "-" + flags
}

func newTraceID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func newSpanID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

func isHex(s string) bool {
	_, err := hex.DecodeString(s)
	return err == nil
}

func isZeroHex(s string) bool {
	for _, c := range s {
		if c != '0' {
			return false
		}
	}
	return true
}
