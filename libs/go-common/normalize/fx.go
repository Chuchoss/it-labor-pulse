package normalize

import (
	"strings"
	"time"
)

// MapFX is an injectable FX table for tests and static providers.
// Keys: ISO currency → YYYY-MM-DD (UTC) → rate to RUB.
type MapFX map[string]map[string]float64

type RateRecord struct {
	RubPerUnit float64
	RateDate   time.Time
	Provider   string
}

// RateTable is an in-memory snapshot loaded from durable FX storage.
type RateTable map[string]map[string]RateRecord

func (t RateTable) RateToRUB(currency string, date time.Time) (float64, bool) {
	rate, _, _, ok := t.RateToRUBDetailed(currency, date)
	return rate, ok
}

func (t RateTable) RateToRUBDetailed(
	currency string,
	date time.Time,
) (float64, time.Time, string, bool) {
	byDate := t[NormalizeCurrency(currency)]
	if byDate == nil {
		return 0, time.Time{}, "", false
	}
	record, ok := byDate[date.UTC().Format(time.DateOnly)]
	return record.RubPerUnit, record.RateDate, record.Provider, ok
}

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
func resolveFX(opts Options, currency string, rateDate time.Time) (
	rate float64,
	usedDate time.Time,
	provider string,
	miss bool,
) {
	cur := NormalizeCurrency(currency)
	if cur == "" {
		return 0, time.Time{}, "", true
	}
	day := time.Date(rateDate.UTC().Year(), rateDate.UTC().Month(), rateDate.UTC().Day(), 0, 0, 0, 0, time.UTC)
	if cur == "RUB" {
		return 1, day, "canonical", false
	}

	fx := opts.FX
	if fx == nil {
		fx = MapFX{}
	}
	maxDays := opts.FXMaxFallbackDays
	if maxDays < 0 {
		maxDays = 0
	}
	for i := 0; i <= maxDays; i++ {
		d := day.AddDate(0, 0, -i)
		if detailed, ok := fx.(DetailedFXProvider); ok {
			if r, actualDate, source, found := detailed.RateToRUBDetailed(cur, d); found && r > 0 {
				return r, actualDate, source, false
			}
		}
		if r, ok := fx.RateToRUB(cur, d); ok && r > 0 {
			return r, d, "configured", false
		}
	}
	if opts.AllowStaticFXFallback && cur == "USD" && opts.StaticUSDToRUB > 0 {
		return opts.StaticUSDToRUB, day, "static-dev", false
	}
	return 0, time.Time{}, "", true
}
