package worker

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMondayUTC(t *testing.T) {
	t.Parallel()
	got := MondayUTC(time.Date(2026, 8, 26, 18, 30, 0, 0, time.FixedZone("UTC+3", 3*60*60)))
	require.Equal(t, time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC), got)
}

func TestWeeklyRejectsNonMondayBeforeDatabaseAccess(t *testing.T) {
	t.Parallel()
	w := &Worker{}
	_, err := w.RunWeekly(
		context.Background(),
		time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC),
		"hh",
	)
	require.EqualError(t, err, "analytics: week start must be Monday")
}
