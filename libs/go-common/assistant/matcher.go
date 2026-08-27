// Package assistant contains the source-neutral personal vacancy assistant.
// It deliberately has no network or database dependencies.
package assistant

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Preferences struct {
	ApprovedRoles     []string
	Specialization    Specialization
	IncludeLeadership bool
	MinSalaryRUB      *float64
	Regions           []string
	RemoteOnly        bool
	RequiredSkills    []string
	ExcludedSkills    []string
	MaxAge            time.Duration
	Weights           Weights
}

type Weights struct {
	Role, Salary, Region, Skills float64
}

type Vacancy struct {
	ID, Title, RoleID, RegionID string
	RoleIDs                     []string
	Description                 string
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
	v.RoleIDs = officialRoleIDs(v)
	skills := make(map[string]bool, len(v.Skills))
	for _, s := range v.Skills {
		skills[normalizeSkill(s)] = true
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
	classification := ClassifyVacancy(v)
	if classification.Leadership && !p.IncludeLeadership {
		r.Decision = DecisionReject
		r.Conflicts = append(r.Conflicts, "leadership_excluded")
		r.Evidence = append(r.Evidence, "leadership:"+classification.Evidence)
		return r
	}
	if classification.Leadership && p.IncludeLeadership {
		r.Reasons = append(r.Reasons, "leadership_allowed")
		r.Evidence = append(r.Evidence, "leadership:"+classification.Evidence)
	}
	if p.Specialization != "" {
		switch {
		case classification.Specialization == SpecializationUnknown:
			r.Unknowns = append(r.Unknowns, "specialization")
		case classification.Specialization != p.Specialization:
			r.Decision = DecisionReject
			r.Conflicts = append(r.Conflicts, "specialization:"+string(classification.Specialization))
			r.Evidence = append(r.Evidence, "specialization:"+classification.Evidence)
			return r
		case classification.Confidence == "low":
			r.Unknowns = append(r.Unknowns, "specialization_description_only")
			r.Evidence = append(r.Evidence, "specialization:description")
		default:
			r.Reasons = append(r.Reasons, "specialization:"+string(p.Specialization))
			r.Evidence = append(r.Evidence, "specialization:"+classification.Evidence)
		}
	}
	if !applyCatalogRoleGate(&r, v, p, classification) {
		return r
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
		normalized := normalizeSkill(required)
		if hasExplicitSkill(v, normalized, skills) {
			r.Reasons = append(r.Reasons, "required_skill:"+required)
			r.Evidence = append(r.Evidence, "skill:"+required)
		} else if strings.TrimSpace(v.Title) == "" && len(v.Skills) == 0 && strings.TrimSpace(v.Description) == "" {
			r.Unknowns = append(r.Unknowns, "required_skill:"+required)
		} else {
			r.Decision = DecisionReject
			r.Conflicts = append(r.Conflicts, "required_skill_missing:"+required)
			return r
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
	rolePass := len(p.ApprovedRoles) == 0 || overlaps(p.ApprovedRoles, v.RoleIDs) ||
		contains(r.Reasons, "approved_role") || contains(r.Reasons, "specialization_satisfies_role")
	roleKnown := len(v.RoleIDs) > 0 || contains(r.Reasons, "specialization_satisfies_role") || len(p.ApprovedRoles) == 0
	components := []float64{
		boolScore(roleKnown, rolePass),
		boolScore(v.SalaryRUB != nil, p.MinSalaryRUB == nil || (v.SalaryRUB != nil && *v.SalaryRUB >= *p.MinSalaryRUB)),
		boolScore(v.RegionID != "", len(p.Regions) == 0 || contains(p.Regions, v.RegionID)),
		skillScore(p.RequiredSkills, skills),
	}
	r.Score = math.Round((components[0]*w.Role+components[1]*w.Salary+components[2]*w.Region+components[3]*w.Skills)*100) / 100
	if len(r.Unknowns) > 0 {
		r.Decision = DecisionReview
	}
	if r.Decision == DecisionMatch && len(r.Reasons) == 0 {
		r.Reasons = append(r.Reasons, "no_conflicts")
	}
	return r
}

func specializationProven(c Classification) bool {
	return c.Specialization != SpecializationUnknown && (c.Evidence == "title" || c.Evidence == "skills")
}

func applyCatalogRoleGate(r *Result, v Vacancy, p Preferences, classification Classification) bool {
	if len(p.ApprovedRoles) == 0 {
		return true
	}
	if len(v.RoleIDs) > 0 && overlaps(p.ApprovedRoles, v.RoleIDs) {
		r.Reasons = append(r.Reasons, "approved_role")
		r.Evidence = append(r.Evidence, "role")
		return true
	}
	specSet := p.Specialization != ""
	if specSet && classification.Specialization == p.Specialization && specializationProven(classification) {
		r.Reasons = append(r.Reasons, "specialization_satisfies_role")
		r.Evidence = append(r.Evidence, "specialization:"+classification.Evidence)
		return true
	}
	if specSet {
		r.Unknowns = append(r.Unknowns, "role")
		return true
	}
	if len(v.RoleIDs) == 0 {
		r.Unknowns = append(r.Unknowns, "role")
		return true
	}
	if contains(p.ApprovedRoles, "96") && classification.Specialization == SpecializationFrontend &&
		specializationProven(classification) {
		r.Reasons = append(r.Reasons, "specialization_satisfies_role")
		r.Evidence = append(r.Evidence, "specialization:"+classification.Evidence)
		return true
	}
	r.Decision = DecisionReject
	r.Conflicts = append(r.Conflicts, "role")
	return false
}

// catalogRoleHardRejectV3 is the superseded v3 catalog overlap gate.
func catalogRoleHardRejectV3(v Vacancy, p Preferences) bool {
	if len(p.ApprovedRoles) == 0 {
		return false
	}
	ids := officialRoleIDs(v)
	return len(ids) > 0 && !overlaps(p.ApprovedRoles, ids)
}

// ApplyHardGatePrecedence combines an untrusted AI classification with the
// authoritative deterministic result.
func ApplyHardGatePrecedence(deterministic Result, ai MatchOutput) MatchOutput {
	if deterministic.Decision == DecisionReject {
		ai.Decision = string(DecisionReject)
		ai.Conflicts = appendUnique(ai.Conflicts, deterministic.Conflicts...)
		return ai
	}
	if deterministic.Decision == DecisionMatch && len(deterministic.Unknowns) == 0 {
		ai.Decision = string(DecisionMatch)
		for criterion, proof := range ai.CriterionEvidence {
			if proof.Pass {
				ai.Evidence = appendUnique(ai.Evidence, "criterion:"+criterion+":"+proof.Source)
			}
		}
		return ai
	}
	if ai.Decision != string(DecisionMatch) {
		return ai
	}
	if ai.Rationale == "id_list_match" {
		ai = enrichIDListMatchEvidence(deterministic, ai)
	}
	for criterion, proof := range ai.CriterionEvidence {
		if proof.Pass {
			ai.Evidence = appendUnique(ai.Evidence, "criterion:"+criterion+":"+proof.Source)
		}
	}
	for _, unknown := range deterministic.Unknowns {
		key := unknown
		switch {
		case unknown == "specialization_description_only":
			key = "specialization"
		case strings.HasPrefix(unknown, "required_skill:"):
			key = "required_skill:" + normalizeSkill(strings.TrimPrefix(unknown, "required_skill:"))
		}
		proof, ok := ai.CriterionEvidence[key]
		if !ok || !proof.Pass || strings.TrimSpace(proof.Source) == "" {
			ai.Decision = string(DecisionReview)
			ai.Unknowns = appendUnique(ai.Unknowns, unknown)
		}
	}
	return ai
}

func enrichIDListMatchEvidence(deterministic Result, ai MatchOutput) MatchOutput {
	if ai.CriterionEvidence == nil {
		ai.CriterionEvidence = map[string]CriterionProof{}
	}
	for _, unknown := range deterministic.Unknowns {
		key := unknown
		switch {
		case unknown == "specialization_description_only":
			key = "specialization"
		case strings.HasPrefix(unknown, "required_skill:"):
			key = "required_skill:" + normalizeSkill(strings.TrimPrefix(unknown, "required_skill:"))
		}
		if _, ok := ai.CriterionEvidence[key]; !ok {
			ai.CriterionEvidence[key] = CriterionProof{Pass: true, Source: "description"}
		}
	}
	return ai
}

func appendUnique(values []string, additions ...string) []string {
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		seen[value] = true
	}
	for _, value := range additions {
		if value != "" && !seen[value] {
			values = append(values, value)
			seen[value] = true
		}
	}
	return values
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
		if skills[normalizeSkill(s)] {
			hit++
		}
	}
	return float64(hit) / float64(len(required))
}

func officialRoleIDs(v Vacancy) []string {
	ids := append([]string{}, v.RoleIDs...)
	primary := strings.TrimSpace(v.RoleID)
	if primary != "" && !contains(ids, primary) {
		if _, ok := approvedRolePolicy[primary]; ok {
			ids = append(ids, primary)
		}
	}
	return ids
}

func normalizeSkill(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "ё", "е")
	if impliesReactWeb(value) {
		return "react"
	}
	return value
}

var reactToken = regexp.MustCompile(`(?i)(?:^|[^\pL\pN])(?:react\.js|reactjs|react)(?:$|[^\pL\pN])`)

func stripReactNative(value string) string {
	lower := strings.ToLower(value)
	lower = strings.ReplaceAll(lower, "react-native", " ")
	lower = strings.ReplaceAll(lower, "react native", " ")
	return lower
}

func impliesReactWeb(value string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(value))
	switch trimmed {
	case "react", "react.js", "reactjs", "react-js":
		return true
	}
	slugged := strings.NewReplacer(".", " ", "_", " ", "/", " ", "+", " ").Replace(trimmed)
	return reactToken.MatchString(stripReactNative(slugged))
}

func hasExplicitSkill(v Vacancy, skill string, normalizedSkills map[string]bool) bool {
	if skill == "" {
		return false
	}
	if skill == "react" {
		if normalizedSkills["react"] {
			return true
		}
		for _, raw := range v.Skills {
			if impliesReactWeb(raw) {
				return true
			}
		}
		return reactToken.MatchString(stripReactNative(v.Title)) || reactToken.MatchString(stripReactNative(v.Description))
	}
	if normalizedSkills[skill] {
		return true
	}
	rule := regexp.MustCompile(`(?i)(?:^|[^\pL\pN])(?:` + regexp.QuoteMeta(skill) + `)(?:$|[^\pL\pN])`)
	return rule.MatchString(v.Title) || rule.MatchString(v.Description)
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
