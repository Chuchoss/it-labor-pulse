package hh

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/Chuchoss/it-labor-pulse/libs/go-common/normalize"
)

// SourceCode is the HH sources.code value.
const SourceCode = "hh"

// SearchPage is a parsed HH vacancies search response.
type SearchPage struct {
	Found   int          `json:"found"`
	Page    int          `json:"page"`
	Pages   int          `json:"pages"`
	PerPage int          `json:"per_page"`
	Items   []SearchItem `json:"items"`
}

// SearchItem contains only fields documented on the HH vacancy search item.
// It deliberately excludes employer/title persistence and descriptions.
type SearchItem struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	AlternateURL string `json:"alternate_url"`
	PublishedAt  string `json:"published_at"`
	Area         *struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"area"`
	Salary *struct {
		From     *float64 `json:"from"`
		To       *float64 `json:"to"`
		Currency string   `json:"currency"`
		Gross    *bool    `json:"gross"`
	} `json:"salary"`
	ProfessionalRoles []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"professional_roles"`
}

type vacancyPayload struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	AlternateURL string `json:"alternate_url"`
	Area         *struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"area"`
	Salary *struct {
		From     *float64 `json:"from"`
		To       *float64 `json:"to"`
		Currency string   `json:"currency"`
		Gross    *bool    `json:"gross"`
	} `json:"salary"`
	Employer *struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"employer"`
	KeySkills []struct {
		Name string `json:"name"`
	} `json:"key_skills"`
	ProfessionalRoles []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"professional_roles"`
	Schedule *struct {
		ID string `json:"id"`
	} `json:"schedule"`
	WorkFormat []struct {
		ID string `json:"id"`
	} `json:"work_format"`
	Description string `json:"description"`
	PublishedAt string `json:"published_at"`
	Archived    bool   `json:"archived"`
}

// ParseSearchPage decodes HH GET /vacancies JSON.
func ParseSearchPage(raw []byte) (SearchPage, error) {
	var page SearchPage
	if err := json.Unmarshal(raw, &page); err != nil {
		return SearchPage{}, fmt.Errorf("hh parse search page: %w", err)
	}
	return page, nil
}

// DraftFromSearch maps the search response fields shared with vacancy detail.
// Search does not expose key_skills or description; callers must not infer them.
func DraftFromSearch(item SearchItem, observedAt time.Time) (normalize.Draft, error) {
	if item.ID == "" {
		return normalize.Draft{}, fmt.Errorf("hh parse search item: empty id")
	}
	if item.Name == "" {
		return normalize.Draft{}, fmt.Errorf("hh parse search item: empty name")
	}
	d := normalize.Draft{
		SchemaVersion: normalize.SchemaVersionV1,
		Source:        SourceCode,
		ExternalID:    item.ID,
		Title:         item.Name,
		CollectedAt:   observedAt.UTC(),
	}
	if item.AlternateURL != "" {
		sourceURL, err := ValidateSourceURL(item.AlternateURL)
		if err != nil {
			return normalize.Draft{}, fmt.Errorf("hh parse search item alternate_url: %w", err)
		}
		d.SourceURL = sourceURL
	}
	if item.PublishedAt == "" {
		return normalize.Draft{}, fmt.Errorf("hh parse search item: empty published_at")
	}
	publishedAt, err := parseHHTime(item.PublishedAt)
	if err != nil {
		return normalize.Draft{}, fmt.Errorf("hh parse search item published_at: %w", err)
	}
	d.PublishedAt = publishedAt
	if item.Area != nil {
		d.RegionExternalID = item.Area.ID
		d.RegionName = item.Area.Name
	}
	if item.Salary != nil {
		d.SalaryFrom = item.Salary.From
		d.SalaryTo = item.Salary.To
		d.SalaryCurrencyRaw = item.Salary.Currency
		d.SalaryGross = item.Salary.Gross
	}
	for _, role := range item.ProfessionalRoles {
		if role.ID != "" {
			d.ProfessionalRoleIDs = append(d.ProfessionalRoleIDs, role.ID)
		}
	}
	return d, nil
}

// DraftFromDetail maps HH vacancy detail JSON → SourceNeutralDraftV1.
// Does not apply shared salary/FX/outlier policy (normalizer ownership).
func DraftFromDetail(raw []byte, collectedAt time.Time) (normalize.Draft, error) {
	var v vacancyPayload
	if err := json.Unmarshal(raw, &v); err != nil {
		return normalize.Draft{}, fmt.Errorf("hh parse detail: %w", err)
	}
	if v.ID == "" {
		return normalize.Draft{}, fmt.Errorf("hh parse detail: empty id")
	}
	if v.Name == "" {
		return normalize.Draft{}, fmt.Errorf("hh parse detail: empty name")
	}

	d := normalize.Draft{
		SchemaVersion:   normalize.SchemaVersionV1,
		Source:          SourceCode,
		ExternalID:      v.ID,
		Title:           v.Name,
		CollectedAt:     collectedAt.UTC(),
		DescriptionText: StripHTML(v.Description),
		RawPayload:      append(json.RawMessage(nil), raw...),
	}
	if v.AlternateURL != "" {
		sourceURL, err := ValidateSourceURL(v.AlternateURL)
		if err != nil {
			return normalize.Draft{}, fmt.Errorf("hh parse detail alternate_url: %w", err)
		}
		d.SourceURL = sourceURL
	}
	if v.Area != nil {
		d.RegionExternalID = v.Area.ID
		d.RegionName = v.Area.Name
	}
	if v.Employer != nil {
		d.EmployerExternalID = v.Employer.ID
		d.EmployerName = v.Employer.Name
	}
	if v.Salary != nil {
		d.SalaryFrom = v.Salary.From
		d.SalaryTo = v.Salary.To
		d.SalaryCurrencyRaw = v.Salary.Currency
		d.SalaryGross = v.Salary.Gross
	}
	for _, s := range v.KeySkills {
		if s.Name != "" {
			d.SkillsRaw = append(d.SkillsRaw, s.Name)
		}
	}
	for _, r := range v.ProfessionalRoles {
		if r.ID != "" {
			d.ProfessionalRoleIDs = append(d.ProfessionalRoleIDs, r.ID)
		}
	}
	if v.Schedule != nil {
		d.ScheduleID = v.Schedule.ID
	}
	for _, wf := range v.WorkFormat {
		if wf.ID != "" {
			d.WorkFormatIDs = append(d.WorkFormatIDs, wf.ID)
		}
	}
	if v.PublishedAt != "" {
		ts, err := parseHHTime(v.PublishedAt)
		if err != nil {
			return normalize.Draft{}, fmt.Errorf("hh parse detail published_at: %w", err)
		}
		d.PublishedAt = ts
	}
	active := !v.Archived
	d.IsActiveHint = &active
	return d, nil
}

// ValidateSourceURL enforces HH adapter URL policy before canonical storage.
func ValidateSourceURL(raw string) (string, error) {
	if strings.ContainsAny(raw, "\x00\r\n\t") {
		return "", fmt.Errorf("contains control characters")
	}
	parsed, err := url.Parse(raw)
	if err != nil || !parsed.IsAbs() || parsed.User != nil {
		return "", fmt.Errorf("must be an absolute URL without userinfo")
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return "", fmt.Errorf("unsupported URL scheme")
	}
	host := strings.ToLower(parsed.Hostname())
	if host != "hh.ru" && !strings.HasSuffix(host, ".hh.ru") {
		return "", fmt.Errorf("unsupported HH host")
	}
	return parsed.String(), nil
}

func parseHHTime(s string) (time.Time, error) {
	// HH uses +0300 without colon.
	if t, err := time.Parse("2006-01-02T15:04:05-0700", s); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), nil
	}
	return time.Time{}, fmt.Errorf("unsupported time %q", s)
}
