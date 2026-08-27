// Package assistant contains the source-neutral personal vacancy assistant.
// It deliberately has no network or database dependencies.
package assistant

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

type Preferences struct {
	ApprovedRoles  []string
	MinSalaryRUB   *float64
	Regions        []string
	RemoteOnly     bool
	RequiredSkills []string
	ExcludedSkills []string
	MaxAge         time.Duration
	Weights        Weights
}

type Weights struct {
	Role, Salary, Region, Skills float64
}

type Vacancy struct {
	ID, Title, RoleID, RegionID string
	RoleIDs                     []string
	SalaryRUB                   *float64
	Skills                      []string
	IsRemote                    *bool
	PublishedAt                 *time.Time
	FirstObservedAt             *time.Time
}

type Decision string

const (
	DecisionMatch  Decision = "match"
	DecisionReject Decision = "reject"
	DecisionReview Decision = "review"
)

type Result struct {
	Decision  Decision `json:"decision"`
	Score     float64  `json:"score"`
	Reasons   []string `json:"reasons"`
	Unknowns  []string `json:"unknowns"`
	Conflicts []string `json:"conflicts"`
	Evidence  []string `json:"evidence_ids"`
}

func (p Preferences) normalizedWeights() Weights {
	w := p.Weights
	if w.Role <= 0 && w.Salary <= 0 && w.Region <= 0 && w.Skills <= 0 {
		return Weights{Role: 0.3, Salary: 0.25, Region: 0.2, Skills: 0.25}
	}
	total := w.Role + w.Salary + w.Region + w.Skills
	if total <= 0 {
		return Weights{Role: 0.3, Salary: 0.25, Region: 0.2, Skills: 0.25}
	}
	return Weights{w.Role / total, w.Salary / total, w.Region / total, w.Skills / total}
}

func Match(v Vacancy, p Preferences, now time.Time) Result {
	r := Result{Decision: DecisionMatch, Reasons: []string{}, Unknowns: []string{}, Conflicts: []string{}, Evidence: []string{}}
	skills := make(map[string]bool, len(v.Skills))
	for _, s := range v.Skills {
		skills[strings.ToLower(strings.TrimSpace(s))] = true
	}
	for _, excluded := range p.ExcludedSkills {
		if skills[strings.ToLower(strings.TrimSpace(excluded))] {
			r.Conflicts = append(r.Conflicts, "excluded_skill:"+excluded)
		}
	}
	if len(r.Conflicts) > 0 {
		r.Decision = DecisionReject
		return r
	}
	if len(p.ApprovedRoles) > 0 {
		if len(v.RoleIDs) == 0 {
			r.Unknowns = append(r.Unknowns, "role")
		} else if !overlaps(p.ApprovedRoles, v.RoleIDs) {
			r.Decision = DecisionReject
			r.Conflicts = append(r.Conflicts, "role")
			return r
		} else {
			r.Reasons = append(r.Reasons, "approved_role")
			r.Evidence = append(r.Evidence, "role")
		}
	}
	if p.MinSalaryRUB != nil {
		if v.SalaryRUB == nil {
			r.Unknowns = append(r.Unknowns, "salary")
		} else if *v.SalaryRUB < *p.MinSalaryRUB {
			r.Decision = DecisionReject
			r.Conflicts = append(r.Conflicts, "minimum_salary")
			return r
		} else {
			r.Reasons = append(r.Reasons, "salary_meets_minimum")
			r.Evidence = append(r.Evidence, "salary_rub")
		}
	}
	if len(p.Regions) > 0 {
		if v.RegionID == "" {
			r.Unknowns = append(r.Unknowns, "region")
		} else if !contains(p.Regions, v.RegionID) {
			r.Decision = DecisionReject
			r.Conflicts = append(r.Conflicts, "region")
			return r
		} else {
			r.Reasons = append(r.Reasons, "region")
			r.Evidence = append(r.Evidence, "region")
		}
	}
	if p.RemoteOnly {
		if v.IsRemote == nil {
			r.Unknowns = append(r.Unknowns, "remote")
		} else if !*v.IsRemote {
			r.Decision = DecisionReject
			r.Conflicts = append(r.Conflicts, "remote_only")
			return r
		} else {
			r.Reasons = append(r.Reasons, "remote")
			r.Evidence = append(r.Evidence, "remote")
		}
	}
	for _, required := range p.RequiredSkills {
		if skills[strings.ToLower(strings.TrimSpace(required))] {
			r.Reasons = append(r.Reasons, "required_skill:"+required)
			r.Evidence = append(r.Evidence, "skill:"+required)
		} else {
			r.Unknowns = append(r.Unknowns, "required_skill:"+required)
		}
	}
	if p.MaxAge > 0 {
		when := v.PublishedAt
		if when == nil {
			when = v.FirstObservedAt
		}
		if when == nil {
			r.Unknowns = append(r.Unknowns, "freshness")
		} else if now.Sub(*when) > p.MaxAge {
			r.Decision = DecisionReject
			r.Conflicts = append(r.Conflicts, "stale")
			return r
		}
	}
	w := p.normalizedWeights()
	components := []float64{boolScore(len(v.RoleIDs) > 0, len(p.ApprovedRoles) == 0 || overlaps(p.ApprovedRoles, v.RoleIDs)), boolScore(v.SalaryRUB != nil, p.MinSalaryRUB == nil || (v.SalaryRUB != nil && *v.SalaryRUB >= *p.MinSalaryRUB)), boolScore(v.RegionID != "", len(p.Regions) == 0 || contains(p.Regions, v.RegionID)), skillScore(p.RequiredSkills, skills)}
	r.Score = math.Round((components[0]*w.Role+components[1]*w.Salary+components[2]*w.Region+components[3]*w.Skills)*100) / 100
	if len(r.Unknowns) > 0 {
		r.Decision = DecisionReview
	}
	if r.Decision == DecisionMatch && len(r.Reasons) == 0 {
		r.Reasons = append(r.Reasons, "no_conflicts")
	}
	return r
}

func boolScore(known, passes bool) float64 {
	if !known {
		return 0
	}
	if passes {
		return 1
	}
	return 0
}
func skillScore(required []string, skills map[string]bool) float64 {
	if len(required) == 0 {
		return 1
	}
	hit := 0
	for _, s := range required {
		if skills[strings.ToLower(strings.TrimSpace(s))] {
			hit++
		}
	}
	return float64(hit) / float64(len(required))
}
func contains(values []string, needle string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(needle)) {
			return true
		}
	}
	return false
}

func overlaps(left, right []string) bool {
	for _, value := range left {
		if contains(right, value) {
			return true
		}
	}
	return false
}

func (r Result) ValidateEvidence(allowed map[string]bool) error {
	for _, id := range r.Evidence {
		if !allowed[id] {
			return fmt.Errorf("unknown evidence id %q", id)
		}
	}
	if r.Score < 0 || r.Score > 1 || math.IsNaN(r.Score) {
		return fmt.Errorf("score must be between 0 and 1")
	}
	return nil
}

func EvidenceID(kind, value string) string {
	return kind + ":" + strconv.Quote(strings.TrimSpace(value))
}
