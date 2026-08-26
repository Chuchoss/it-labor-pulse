package normalize_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Chuchoss/it-labor-pulse/libs/go-common/normalize"
)

func TestMid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		from, to    *float64
		wantMid     *float64
		wantSwap    bool
		wantInvalid bool
	}{
		{name: "from_and_to", from: f64(100), to: f64(200), wantMid: f64(150)},
		{name: "only_from", from: f64(100), wantMid: f64(100)},
		{name: "only_to", to: f64(200), wantMid: f64(200)},
		{name: "both_null", wantMid: nil},
		{name: "from_gt_to_swap", from: f64(200), to: f64(100), wantMid: f64(150), wantSwap: true},
		{name: "zero_from_invalid", from: f64(0), to: f64(100), wantMid: f64(100), wantInvalid: true},
		{name: "negative_to_invalid", from: f64(100), to: f64(-1), wantMid: f64(100), wantInvalid: true},
		{name: "both_invalid", from: f64(0), to: f64(-5), wantMid: nil, wantInvalid: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, swap, invalid := normalize.Mid(tt.from, tt.to)
			require.Equal(t, tt.wantSwap, swap)
			require.Equal(t, tt.wantInvalid, invalid)
			if tt.wantMid == nil {
				require.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			require.InDelta(t, *tt.wantMid, *got, 1e-9)
		})
	}
}

func TestNormalize_grossNetAndFX(t *testing.T) {
	t.Parallel()
	collected := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name         string
		draft        normalize.Draft
		opts         normalize.Options
		wantMid      *float64
		wantMidRub   *float64
		wantCurrency string
		wantExclude  bool
		wantFXMiss   bool
		wantGrossUnk bool
	}{
		{
			name: "gross_true_applies_tax",
			draft: normalize.Draft{
				Source: "hh", ExternalID: "1", Title: "Dev",
				SalaryFrom: f64(100_000), SalaryTo: f64(100_000),
				SalaryCurrencyRaw: "RUB", SalaryGross: b(true),
				CollectedAt: collected,
			},
			wantMid: f64(87_000), wantMidRub: f64(87_000), wantCurrency: "RUB",
		},
		{
			name: "gross_false_no_tax",
			draft: normalize.Draft{
				Source: "hh", ExternalID: "2", Title: "Dev",
				SalaryFrom: f64(100_000), SalaryCurrencyRaw: "RUB", SalaryGross: b(false),
				CollectedAt: collected,
			},
			wantMid: f64(100_000), wantMidRub: f64(100_000), wantCurrency: "RUB",
		},
		{
			name: "gross_null_as_net_metric",
			draft: normalize.Draft{
				Source: "hh", ExternalID: "3", Title: "Dev",
				SalaryFrom: f64(100_000), SalaryCurrencyRaw: "RUB",
				CollectedAt: collected,
			},
			wantMid: f64(100_000), wantMidRub: f64(100_000), wantCurrency: "RUB", wantGrossUnk: true,
		},
		{
			name: "usd_with_rate",
			draft: normalize.Draft{
				Source: "hh", ExternalID: "4", Title: "Dev",
				SalaryFrom: f64(1000), SalaryTo: f64(1000),
				SalaryCurrencyRaw: "USD", SalaryGross: b(false),
				CollectedAt: collected,
			},
			opts: normalize.Options{
				FX: normalize.MapFX{"USD": {"2026-08-10": 90}},
			},
			wantMid: f64(1000), wantMidRub: f64(90_000), wantCurrency: "USD",
		},
		{
			name: "fx_fallback_previous_day",
			draft: normalize.Draft{
				Source: "hh", ExternalID: "5", Title: "Dev",
				SalaryFrom: f64(1000), SalaryCurrencyRaw: "USD", SalaryGross: b(false),
				CollectedAt: collected,
			},
			opts: normalize.Options{
				FX:                normalize.MapFX{"USD": {"2026-08-09": 80}},
				FXMaxFallbackDays: 7,
			},
			wantMid: f64(1000), wantMidRub: f64(80_000), wantCurrency: "USD",
		},
		{
			name: "fx_miss_no_static",
			draft: normalize.Draft{
				Source: "hh", ExternalID: "6", Title: "Dev",
				SalaryFrom: f64(1000), SalaryCurrencyRaw: "USD", SalaryGross: b(false),
				CollectedAt: collected,
			},
			wantMid: f64(1000), wantMidRub: nil, wantCurrency: "USD", wantExclude: true, wantFXMiss: true,
		},
		{
			name: "fx_static_fallback_dev",
			draft: normalize.Draft{
				Source: "hh", ExternalID: "7", Title: "Dev",
				SalaryFrom: f64(1000), SalaryCurrencyRaw: "USD", SalaryGross: b(false),
				CollectedAt: collected,
			},
			opts: normalize.Options{
				AllowStaticFXFallback: true,
				StaticUSDToRUB:        90,
			},
			wantMid: f64(1000), wantMidRub: f64(90_000), wantCurrency: "USD",
		},
		{
			name: "outlier_low",
			draft: normalize.Draft{
				Source: "hh", ExternalID: "8", Title: "Dev",
				SalaryFrom: f64(5_000), SalaryCurrencyRaw: "RUB", SalaryGross: b(false),
				CollectedAt: collected,
			},
			wantMid: f64(5_000), wantMidRub: f64(5_000), wantCurrency: "RUB", wantExclude: true,
		},
		{
			name: "outlier_high",
			draft: normalize.Draft{
				Source: "hh", ExternalID: "9", Title: "Dev",
				SalaryFrom: f64(3_000_000), SalaryCurrencyRaw: "RUB", SalaryGross: b(false),
				CollectedAt: collected,
			},
			wantMid: f64(3_000_000), wantMidRub: f64(3_000_000), wantCurrency: "RUB", wantExclude: true,
		},
		{
			name: "absent_salary",
			draft: normalize.Draft{
				Source: "hh", ExternalID: "10", Title: "Dev",
				CollectedAt: collected,
			},
			wantMid: nil, wantMidRub: nil, wantExclude: true,
		},
		{
			name: "rur_to_rub",
			draft: normalize.Draft{
				Source: "hh", ExternalID: "11", Title: "Dev",
				SalaryFrom: f64(100_000), SalaryTo: f64(200_000),
				SalaryCurrencyRaw: "RUR", SalaryGross: b(false),
				CollectedAt: collected,
			},
			wantMid: f64(150_000), wantMidRub: f64(150_000), wantCurrency: "RUB",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			opts := normalize.DefaultOptions()
			if tt.opts.FX != nil {
				opts.FX = tt.opts.FX
			}
			if tt.opts.FXMaxFallbackDays != 0 {
				opts.FXMaxFallbackDays = tt.opts.FXMaxFallbackDays
			}
			opts.AllowStaticFXFallback = tt.opts.AllowStaticFXFallback
			opts.StaticUSDToRUB = tt.opts.StaticUSDToRUB

			res, err := normalize.Normalize(tt.draft, opts)
			require.NoError(t, err)
			v := res.Vacancy
			if tt.wantMid == nil {
				require.Nil(t, v.SalaryMid)
			} else {
				require.NotNil(t, v.SalaryMid)
				require.InDelta(t, *tt.wantMid, *v.SalaryMid, 1e-6)
			}
			if tt.wantMidRub == nil {
				require.Nil(t, v.SalaryMidRub)
			} else {
				require.NotNil(t, v.SalaryMidRub)
				require.InDelta(t, *tt.wantMidRub, *v.SalaryMidRub, 1e-6)
			}
			require.Equal(t, tt.wantCurrency, v.SalaryCurrency)
			require.Equal(t, tt.wantExclude, v.ExcludeFromSalaryAgg)
			require.Equal(t, tt.wantFXMiss, res.Metrics.FXMiss)
			require.Equal(t, tt.wantGrossUnk, res.Metrics.GrossUnknown)
		})
	}
}

func TestNormalizeCurrency(t *testing.T) {
	t.Parallel()
	require.Equal(t, "RUB", normalize.NormalizeCurrency("RUR"))
	require.Equal(t, "RUB", normalize.NormalizeCurrency("rur"))
	require.Equal(t, "USD", normalize.NormalizeCurrency("USD"))
}
