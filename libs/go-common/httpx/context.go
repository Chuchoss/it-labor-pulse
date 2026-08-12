package httpx

import "context"

type ctxKey int

const (
	requestIDKey ctxKey = iota + 1
	traceIDKey
)

// RequestID returns the request_id from context, or empty string.
func RequestID(ctx context.Context) string {
	v, _ := ctx.Value(requestIDKey).(string)
	return v
}

// TraceID returns the W3C trace_id (32 hex) from context, or empty string.
func TraceID(ctx context.Context) string {
	v, _ := ctx.Value(traceIDKey).(string)
	return v
}

func withRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

func withTraceID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, traceIDKey, id)
}
