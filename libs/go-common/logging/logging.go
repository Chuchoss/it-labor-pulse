// Package logging provides structured JSON slog helpers for LMA services.
package logging

import (
	"log/slog"
	"os"
	"strings"
)

// Options configures the process-wide JSON logger.
type Options struct {
	Service string
	Env     string
	Level   string
}

// New returns a JSON handler logger with service/env attrs (docs/architecture/18).
func New(opts Options) *slog.Logger {
	level := parseLevel(opts.Level)
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	return slog.New(handler).With(
		"service", opts.Service,
		"env", opts.Env,
	)
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
