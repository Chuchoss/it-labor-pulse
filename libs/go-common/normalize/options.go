package normalize

import "time"

// Default policy constants from docs/architecture/15-normalization-rules.md.
const (
	DefaultTaxRate           = 0.13
	DefaultOutlierMinRub     = 10_000.0
	DefaultOutlierMaxRub     = 2_000_000.0
	DefaultFXMaxFallbackDays = 7
)

// FXProvider resolves currency→RUB rates for a UTC rate date.
// Implementations must be deterministic for a given (currency, date).
type FXProvider interface {
	// RateToRUB returns the multiply factor mid_currency * rate = mid_rub.
	// ok=false means no rate for that exact date (caller may walk fallback days).
	RateToRUB(currency string, date time.Time) (rate float64, ok bool)
}

// RoleMatcher maps HH professional role ids / normalized titles to role_id.
type RoleMatcher interface {
	MatchRole(source string, professionalRoleIDs []string, title string) (roleID string, ok bool)
}

// SkillMatcher maps raw skill names to skill ids (MVP: upsert-by-slug stub).
type SkillMatcher interface {
	MatchSkill(source, rawName string) (skillID string, isNew bool)
}

// RegionMatcher maps source area external ids to region_id.
type RegionMatcher interface {
	MatchRegion(source, regionExternalID string) (regionID string, ok bool)
}

// Options configures normalize policies. Zero value is not usable; call DefaultOptions.
type Options struct {
	TaxRate           float64
	OutlierMinRub     float64
	OutlierMaxRub     float64
	FXMaxFallbackDays int
	// AllowStaticFXFallback enables FX_*_FALLBACK only for local/dev (docs § Currency→RUB).
	AllowStaticFXFallback bool
	StaticUSDToRUB        float64

	FX      FXProvider
	Roles   RoleMatcher
	Skills  SkillMatcher
	Regions RegionMatcher
}

// DefaultOptions returns MVP defaults with empty matchers and no FX rates.
func DefaultOptions() Options {
	return Options{
		TaxRate:           DefaultTaxRate,
		OutlierMinRub:     DefaultOutlierMinRub,
		OutlierMaxRub:     DefaultOutlierMaxRub,
		FXMaxFallbackDays: DefaultFXMaxFallbackDays,
		Roles:             MapRoleMatcher{},
		Skills:            SlugSkillMatcher{},
		Regions:           MapRegionMatcher{},
		FX:                MapFX{},
	}
}
