package main

import (
	"testing"
	"time"
)

func TestNextDailyUTC(t *testing.T) {
	before := time.Date(2026, 8, 26, 0, 30, 0, 0, time.UTC)
	if got := nextDaily(before, 1); !got.Equal(
		time.Date(2026, 8, 26, 1, 0, 0, 0, time.UTC),
	) {
		t.Fatalf("next before schedule = %s", got)
	}
	after := time.Date(2026, 8, 26, 1, 0, 0, 0, time.UTC)
	if got := nextDaily(after, 1); !got.Equal(
		time.Date(2026, 8, 27, 1, 0, 0, 0, time.UTC),
	) {
		t.Fatalf("next at schedule = %s", got)
	}
}
