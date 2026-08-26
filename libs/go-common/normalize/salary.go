package normalize

import "time"

// SalaryNorm is the result of salary policy application (before / with FX).
type SalaryNorm struct {
	From                 *float64
	To                   *float64
	Currency             string
	Gross                *bool
	Mid                  *float64 // source currency, after gross→net
	MidRub               *float64
	RateDate             *time.Time
	RateProvider         string
	ExcludeFromSalaryAgg bool
	Invalid              bool
	GrossUnknown         bool
	Swapped              bool
	FXMiss               bool
}

// Mid computes salary mid in source units before gross/net and FX.
// Invalid (≤0) bounds are treated as null. from>to → swap.
func Mid(from, to *float64) (mid *float64, swapped bool, invalid bool) {
	f, fOK := positiveSalary(from)
	t, tOK := positiveSalary(to)
	if from != nil && !fOK {
		invalid = true
	}
	if to != nil && !tOK {
		invalid = true
	}
	if !fOK && !tOK {
		return nil, false, invalid
	}
	if fOK && tOK && f > t {
		f, t = t, f
		swapped = true
	}
	switch {
	case fOK && tOK:
		v := (f + t) / 2
		return &v, swapped, invalid
	case fOK:
		v := f
		return &v, swapped, invalid
	default:
		v := t
		return &v, swapped, invalid
	}
}

func positiveSalary(p *float64) (float64, bool) {
	if p == nil {
		return 0, false
	}
	if *p <= 0 {
		return 0, false
	}
	return *p, true
}

// applyGrossNet converts mid_gross → mid_net when gross=true.
// null gross → treat as net + GrossUnknown metric.
func applyGrossNet(mid *float64, gross *bool, taxRate float64) (out *float64, grossUnknown bool) {
	if mid == nil {
		return nil, false
	}
	if gross == nil {
		v := *mid
		return &v, true
	}
	if !*gross {
		v := *mid
		return &v, false
	}
	if taxRate < 0 {
		taxRate = 0
	}
	if taxRate > 1 {
		taxRate = 1
	}
	v := *mid * (1 - taxRate)
	return &v, false
}

// normalizeSalary applies mid, gross/net, currency, FX, and outlier policy.
func normalizeSalary(d Draft, opts Options) SalaryNorm {
	var out SalaryNorm

	midRaw, swapped, invalid := Mid(d.SalaryFrom, d.SalaryTo)
	out.Swapped = swapped
	out.Invalid = invalid
	out.Gross = d.SalaryGross

	from, fromOK := positiveSalary(d.SalaryFrom)
	to, toOK := positiveSalary(d.SalaryTo)
	if fromOK && toOK && swapped {
		from, to = to, from
	}
	if fromOK {
		v := from
		out.From = &v
	}
	if toOK {
		v := to
		out.To = &v
	}

	if midRaw == nil {
		out.ExcludeFromSalaryAgg = true
		return out
	}

	midNet, grossUnknown := applyGrossNet(midRaw, d.SalaryGross, opts.TaxRate)
	out.Mid = midNet
	out.GrossUnknown = grossUnknown
	out.Currency = NormalizeCurrency(d.SalaryCurrencyRaw)

	if out.Currency == "" {
		out.ExcludeFromSalaryAgg = true
		out.FXMiss = true
		return out
	}

	rateDate := d.PublishedAt
	if rateDate.IsZero() {
		rateDate = d.CollectedAt
	}
	rate, usedDate, provider, miss := resolveFX(opts, out.Currency, rateDate)
	if miss {
		out.FXMiss = true
		out.ExcludeFromSalaryAgg = true
		return out
	}
	midRub := *midNet * rate
	out.MidRub = &midRub
	out.RateDate = &usedDate
	out.RateProvider = provider

	minR := opts.OutlierMinRub
	if minR <= 0 {
		minR = DefaultOutlierMinRub
	}
	maxR := opts.OutlierMaxRub
	if maxR <= 0 {
		maxR = DefaultOutlierMaxRub
	}
	if midRub < minR || midRub > maxR {
		out.ExcludeFromSalaryAgg = true
	}
	return out
}
