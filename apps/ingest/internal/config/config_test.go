package config_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Chuchoss/it-labor-pulse/apps/ingest/internal/config"
)

func TestLoad_AllPagesMode(t *testing.T) {
	t.Setenv("INGEST_MAX_PAGES", "all")
	t.Setenv("INGEST_PER_PAGE", "100")
	t.Setenv("INGEST_RUN_TIMEOUT_SEC", "1800")

	cfg := config.Load()

	require.Zero(t, cfg.MaxPages)
	require.Equal(t, 100, cfg.PerPage)
	require.Equal(t, 30*time.Minute, cfg.RunTimeout)
}

func TestValidateLive_RejectsInvalidPaging(t *testing.T) {
	cfg := config.Config{
		DatabaseURL: "postgres://example.invalid/db",
		HHUserAgent: "LMATest/0.1 (+test@example.com)",
		RunTimeout:  time.Minute,
		MaxPages:    -1,
		PerPage:     101,
	}

	require.ErrorContains(t, cfg.ValidateLive(), "INGEST_MAX_PAGES")
	cfg.MaxPages = 0
	require.ErrorContains(t, cfg.ValidateLive(), "INGEST_PER_PAGE")
}
