package repository

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMoneyConversionAndHistoricalFallback(t *testing.T) {
	t.Parallel()
	date := "2026-08-21"
	rate := displayRate{factor: 80, date: &date, provider: "cbr"}
	require.Equal(t, 1_000.0, convertMoney(80_000, rate))
	value := 1_000.0
	require.Equal(t, 80_000.0, *toCanonicalRUB(&value, rate))

	rates := map[string]displayRate{"2026-08-21": rate}
	weekend := time.Date(2026, 8, 23, 0, 0, 0, 0, time.UTC)
	resolved := historicalRateFromTable("USD", weekend, rates)
	require.Equal(t, 80.0, resolved.factor)
	require.Equal(t, "2026-08-21", *resolved.date)

	stale := time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC)
	require.Zero(t, historicalRateFromTable("USD", stale, rates).factor)
}

func TestNominalRateRoundTrip(t *testing.T) {
	t.Parallel()
	// CBR Value=18.5 for Nominal=100 means 0.185 RUB per KZT.
	rate := displayRate{factor: 18.5 / 100}
	rub := 18_500.0
	display := convertMoney(rub, rate)
	require.Equal(t, 100_000.0, display)
	require.Equal(t, rub, *toCanonicalRUB(&display, rate))
}
