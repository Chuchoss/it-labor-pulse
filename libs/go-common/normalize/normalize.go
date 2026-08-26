package normalize

import "fmt"

// Normalize maps a source-neutral draft to a canonical vacancy (no I/O, no DB).
func Normalize(d Draft, opts Options) (NormalizeResult, error) {
	if d.Source == "" {
		return NormalizeResult{}, fmt.Errorf("normalize: empty source")
	}
	if d.ExternalID == "" {
		return NormalizeResult{}, fmt.Errorf("normalize: empty external_id")
	}
	if d.Title == "" {
		return NormalizeResult{}, fmt.Errorf("normalize: empty title")
	}

	opts = withDefaults(opts)
	sal := normalizeSalary(d, opts)

	roles := opts.Roles
	roleID, roleOK := roles.MatchRole(d.Source, d.ProfessionalRoleIDs, d.Title)

	var regionID *string
	if rid, ok := opts.Regions.MatchRegion(d.Source, d.RegionExternalID); ok {
		regionID = &rid
	}

	skills := matchSkills(opts.Skills, d.Source, d.SkillsRaw)
	isRemote := DetectRemote(d.ScheduleID, d.WorkFormatIDs, d.RegionName, d.Title, d.DescriptionText)

	isActive := true
	if d.IsActiveHint != nil {
		isActive = *d.IsActiveHint
	}

	v := CanonicalVacancy{
		Source:               d.Source,
		ExternalID:           d.ExternalID,
		SourceURL:            d.SourceURL,
		Title:                d.Title,
		EmployerExternalID:   d.EmployerExternalID,
		EmployerName:         d.EmployerName,
		RegionExternalID:     d.RegionExternalID,
		RegionID:             regionID,
		SalaryFrom:           sal.From,
		SalaryTo:             sal.To,
		SalaryCurrency:       sal.Currency,
		SalaryGross:          sal.Gross,
		SalaryMid:            sal.Mid,
		SalaryMidRub:         sal.MidRub,
		SalaryRateDate:       sal.RateDate,
		SalaryRateProvider:   sal.RateProvider,
		ExcludeFromSalaryAgg: sal.ExcludeFromSalaryAgg,
		Skills:               skills,
		IsRemote:             isRemote,
		IsActive:             isActive,
		PublishedAt:          d.PublishedAt,
		CollectedAt:          d.CollectedAt,
		DescriptionText:      d.DescriptionText,
	}
	if roleOK {
		v.RoleID = &roleID
	}

	v.ContentHash = ContentHash(v)

	m := Metrics{
		SalaryInvalid: sal.Invalid,
		GrossUnknown:  sal.GrossUnknown,
		FXMiss:        sal.FXMiss,
		RoleUnmapped:  !roleOK,
		SalarySwapped: sal.Swapped,
	}
	return NormalizeResult{Vacancy: v, Metrics: m}, nil
}

func withDefaults(opts Options) Options {
	def := DefaultOptions()
	if opts.TaxRate == 0 {
		opts.TaxRate = def.TaxRate
	}
	if opts.OutlierMinRub == 0 {
		opts.OutlierMinRub = def.OutlierMinRub
	}
	if opts.OutlierMaxRub == 0 {
		opts.OutlierMaxRub = def.OutlierMaxRub
	}
	if opts.FXMaxFallbackDays == 0 {
		opts.FXMaxFallbackDays = def.FXMaxFallbackDays
	}
	if opts.FX == nil {
		opts.FX = def.FX
	}
	if opts.Roles == nil {
		opts.Roles = def.Roles
	}
	if opts.Skills == nil {
		opts.Skills = def.Skills
	}
	if opts.Regions == nil {
		opts.Regions = def.Regions
	}
	return opts
}
