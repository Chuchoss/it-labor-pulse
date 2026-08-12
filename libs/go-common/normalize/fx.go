package normalize

import (
	"strings"
	"time"
)

// MapFX is an injectable FX table for tests and static providers.
// Keys: ISO currency → YYYY-MM-DD (UTC) → rate to RUB.
type MapFX map[string]map[string]float64

// RateToRUB implements FXProvider.
func (m MapFX) RateToRUB(currency string, date time.Time) (float64, bool) {
	cur := strings.ToUpper(strings.TrimSpace(currency))
	if cur == "" || cur == "RUB" {
		return 1, true
	}
	byDate, ok := m[cur]
	if !ok || byDate == nil {
		return 0, false
	}
	rate, ok := byDate[date.UTC().Format("2006-01-02")]
	return rate, ok
}

// NormalizeCurrency maps source currency codes to ISO 4217 (HH RUR → RUB).
func NormalizeCurrency(raw string) string {
	c := strings.ToUpper(strings.TrimSpace(raw))
	switch c {
	case "RUR":
		return "RUB"
	default:
		return c
	}
}

// resolveFX returns mid_rub factor for currency on rateDate (UTC date of collected_at).
// Walks back up to maxFallbackDays; optional static USD fallback for local/dev.
func resolveFX(opts Options, currency string, rateDate time.Time) (rate float64, miss bool) {
	cur := NormalizeCurrency(currency)
	if cur == "" {
		return 0, true
	}
	if cur == "RUB" {
		return 1, false
	}

	fx := opts.FX
	if fx == nil {
		fx = MapFX{}
	}
	maxDays := opts.FXMaxFallbackDays
	if maxDays < 0 {
		maxDays = 0
	}
	day := rateDate.UTC().Truncate(24 * time.Hour)
	for i := 0; i <= maxDays; i++ {
		d := day.AddDate(0, 0, -i)
		if r, ok := fx.RateToRUB(cur, d); ok && r > 0 {
			return r, false
		}
	}
	if opts.AllowStaticFXFallback && cur == "USD" && opts.StaticUSDToRUB > 0 {
		return opts.StaticUSDToRUB, false
	}
	return 0, true
}
