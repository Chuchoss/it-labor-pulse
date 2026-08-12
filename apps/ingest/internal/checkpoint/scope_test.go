package checkpoint_test

import (
	"testing"

	"github.com/Chuchoss/it-labor-pulse/apps/ingest/internal/checkpoint"
	"github.com/stretchr/testify/require"
)

func TestScopeHash_Stable(t *testing.T) {
	a := checkpoint.ScopeHash("hh", "incremental", "1", "Golang")
	b := checkpoint.ScopeHash("HH", "incremental", "1", "golang")
	require.Equal(t, a, b)
	require.Len(t, a, 64)
	require.NotEqual(t, a, checkpoint.ScopeHash("hh", "incremental", "2", "golang"))
}

func TestDecide_FailureDoesNotAdvance(t *testing.T) {
	d := checkpoint.Decide(checkpoint.PageOutcome{
		AllOK:       false,
		CurrentPage: 0,
		TotalPages:  3,
		ItemCount:   20,
	})
	require.False(t, d.Advance)
	require.False(t, d.Terminal)
}

func TestDecide_AllOKAdvances(t *testing.T) {
	d := checkpoint.Decide(checkpoint.PageOutcome{
		AllOK:       true,
		CurrentPage: 0,
		TotalPages:  3,
		ItemCount:   20,
	})
	require.True(t, d.Advance)
	require.Equal(t, "1", d.NextCursor)
	require.False(t, d.Terminal)
}

func TestDecide_LastPageTerminal(t *testing.T) {
	d := checkpoint.Decide(checkpoint.PageOutcome{
		AllOK:       true,
		CurrentPage: 2,
		TotalPages:  3,
		ItemCount:   5,
	})
	require.True(t, d.Advance)
	require.True(t, d.Terminal)
	require.Equal(t, "last_page", d.TerminalReason)
	require.Equal(t, "3", d.NextCursor)
}

func TestDecide_EmptyPageTerminal(t *testing.T) {
	d := checkpoint.Decide(checkpoint.PageOutcome{
		AllOK:       true,
		CurrentPage: 1,
		TotalPages:  5,
		ItemCount:   0,
	})
	require.True(t, d.Advance)
	require.True(t, d.Terminal)
	require.Equal(t, "empty_page", d.TerminalReason)
}

func TestParseCursorPage(t *testing.T) {
	n, err := checkpoint.ParseCursorPage("")
	require.NoError(t, err)
	require.Equal(t, 0, n)
	n, err = checkpoint.ParseCursorPage("4")
	require.NoError(t, err)
	require.Equal(t, 4, n)
}
