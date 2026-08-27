// Package normalize maps SourceNeutralDraftV1 into canonical vacancy fields
// per docs/architecture/15-normalization-rules.md. Pure functions; no I/O.
package normalize

import (
	"encoding/json"
	"time"
)

// SchemaVersionV1 is the current source-neutral draft schema.
const SchemaVersionV1 = "SourceNeutralDraftV1"

// Draft is the adapter → normalizer contract (SourceNeutralDraftV1).
// It holds source facts only; shared salary/role/FX rules live in this package.
type Draft struct {
	SchemaVersion        string          `json:"schema_version"`
	Source               string          `json:"source"`
	ExternalID           string          `json:"external_id"`
	SourceURL            string          `json:"source_url,omitempty"`
	Title                string          `json:"title"`
	EmployerExternalID   string          `json:"employer_external_id"`
	EmployerName         string          `json:"employer_name,omitempty"`
	RegionExternalID     string          `json:"region_external_id"`
	RegionName           string          `json:"region_name,omitempty"`
	SalaryFrom           *float64        `json:"salary_from"`
	SalaryTo             *float64        `json:"salary_to"`
	SalaryCurrencyRaw    string          `json:"salary_currency_raw"`
	SalaryGross          *bool           `json:"salary_gross"`
	PublishedAt          time.Time       `json:"published_at"`
	CollectedAt          time.Time       `json:"collected_at"`
	SkillsRaw            []string        `json:"skills_raw"`
	ProfessionalRoleIDs  []string        `json:"professional_role_ids,omitempty"`
	ScheduleID           string          `json:"schedule_id,omitempty"`
	WorkFormatIDs        []string        `json:"work_format_ids,omitempty"`
	DescriptionText      string          `json:"description_text,omitempty"`
	DescriptionTruncated bool            `json:"description_truncated,omitempty"`
	IsActiveHint         *bool           `json:"is_active_hint,omitempty"`
	RawPayload           json.RawMessage `json:"raw_payload,omitempty"`
}

// SkillRef is a normalized skill reference after alias lookup / slug upsert stub.
type SkillRef struct {
	SkillID string `json:"skill_id"`
	RawName string `json:"raw_name"`
	IsNew   bool   `json:"is_new,omitempty"`
}

// CanonicalVacancy is the shared analytical shape before PG UPSERT.
type CanonicalVacancy struct {
	Source               string     `json:"source"`
	ExternalID           string     `json:"external_id"`
	SourceURL            string     `json:"source_url,omitempty"`
	Title                string     `json:"title"`
	EmployerExternalID   string     `json:"employer_external_id"`
	EmployerName         string     `json:"employer_name,omitempty"`
	RegionExternalID     string     `json:"region_external_id"`
	RegionID             *string    `json:"region_id,omitempty"`
	RoleID               *string    `json:"role_id,omitempty"`
	SalaryFrom           *float64   `json:"salary_from"`
	SalaryTo             *float64   `json:"salary_to"`
	SalaryCurrency       string     `json:"salary_currency,omitempty"`
	SalaryGross          *bool      `json:"salary_gross"`
	SalaryMid            *float64   `json:"salary_mid"`
	SalaryMidRub         *float64   `json:"salary_mid_rub"`
	SalaryRateDate       *time.Time `json:"salary_rate_date,omitempty"`
	SalaryRateProvider   string     `json:"salary_rate_provider,omitempty"`
	ExcludeFromSalaryAgg bool       `json:"exclude_from_salary_agg"`
	Skills               []SkillRef `json:"skills"`
	IsRemote             *bool      `json:"is_remote"`
	IsActive             bool       `json:"is_active"`
	PublishedAt          time.Time  `json:"published_at"`
	CollectedAt          time.Time  `json:"collected_at"`
	ContentHash          string     `json:"content_hash"`
	DescriptionText      string     `json:"description_text,omitempty"`
	DescriptionTruncated bool       `json:"description_truncated,omitempty"`
}

// Metrics are pure flags for counters (wired to metrics later).
type Metrics struct {
	SalaryInvalid bool `json:"salary_invalid"`
	GrossUnknown  bool `json:"gross_unknown"`
	FXMiss        bool `json:"fx_miss"`
	RoleUnmapped  bool `json:"role_unmapped"`
	SalarySwapped bool `json:"salary_swapped"`
}

// NormalizeResult is the output of Normalize.
type NormalizeResult struct {
	Vacancy CanonicalVacancy `json:"vacancy"`
	Metrics Metrics          `json:"metrics"`
}
