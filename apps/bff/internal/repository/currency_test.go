package repository

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSupportedDisplayCurrenciesIncludeTengeAndDram(t *testing.T) {
	t.Parallel()

	require.Equal(t, []string{"RUB", "USD", "EUR", "CNY", "KZT", "AMD"}, displayCurrencyCodes())
	require.Equal(t, "Казахстанский тенге", supportedDisplayCurrencies[4].Label)
	require.Equal(t, "₸", supportedDisplayCurrencies[4].Symbol)
	require.Equal(t, "Армянский драм", supportedDisplayCurrencies[5].Label)
	require.Equal(t, "֏", supportedDisplayCurrencies[5].Symbol)
}

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

func TestNominalRatesRoundTrip(t *testing.T) {
	t.Parallel()
	tests := []struct {
		currency string
		value    float64
		nominal  float64
		rub      float64
		display  float64
	}{
		{currency: "KZT", value: 18.5, nominal: 100, rub: 18_500, display: 100_000},
		{currency: "AMD", value: 20.75, nominal: 100, rub: 20_750, display: 100_000},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.currency, func(t *testing.T) {
			t.Parallel()
			rate := displayRate{factor: tt.value / tt.nominal}
			converted := convertMoney(tt.rub, rate)
			require.Equal(t, tt.display, converted)
			require.Equal(t, tt.rub, *toCanonicalRUB(&converted, rate))
		})
	}
}
