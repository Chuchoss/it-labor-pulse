package assistant

import (
	"regexp"
	"strings"
)

const SpecializationRulesVersion = "assistant-hard-gates-v4"

type Specialization string

const (
	SpecializationFrontend  Specialization = "frontend"
	SpecializationBackend   Specialization = "backend"
	SpecializationFullstack Specialization = "fullstack"
	SpecializationMobile    Specialization = "mobile"
	SpecializationDevOps    Specialization = "devops_platform"
	SpecializationDataML    Specialization = "data_ml"
	SpecializationOther     Specialization = "other"
	SpecializationUnknown   Specialization = "unknown"
)

var supportedSpecializations = map[Specialization]bool{
	SpecializationFrontend: true, SpecializationBackend: true, SpecializationFullstack: true,
	SpecializationMobile: true, SpecializationDevOps: true, SpecializationDataML: true,
	SpecializationOther: true,
}

type Classification struct {
	Specialization Specialization
	Leadership     bool
	Confidence     string
	Evidence       string
}

type aliasRules struct {
	frontend, backend, fullstack, mobile, devops, dataML, other, leadership []*regexp.Regexp
}

var specializationAliases = aliasRules{
	frontend: compileAliases(
		`front[\s-]?end`, `фронт[\s-]?энд`, `фронтенд`, `react(?:\.js)?`, `vue(?:\.js)?`,
		`angular`, `javascript`, `typescript`, `js`, `ts`,
	),
	backend:   compileAliases(`back[\s-]?end`, `бэк[\s-]?энд`, `бэкенд`, `backend`, `server[\s-]?side`),
	fullstack: compileAliases(`full[\s-]?stack`, `фулл[\s-]?ст[еэ]к`, `фулст[еэ]к`),
	mobile:    compileAliases(`mobile`, `мобильн\w*`, `android`, `ios`, `flutter`, `react[\s-]?native`),
	devops:    compileAliases(`devops`, `platform[\s-]?engineer\w*`, `sre`, `инженер\w*\s+платформ\w*`),
	dataML:    compileAliases(`data[\s-]?engineer\w*`, `machine[\s-]?learning`, `ml[\s-]?engineer\w*`, `data[\s-]?scientist\w*`, `дата[\s-]?инженер\w*`),
	other:     compileAliases(`embedded`, `firmware`, `game[\s-]?developer\w*`, `unity`, `1c`, `1с`),
	leadership: compileAliases(
		`team[\s-]?lead(?:er)?`, `tech(?:nical)?[\s-]?lead`, `lead[\s-]?(?:developer|engineer)`,
		`lead[\s-]?front[\s-]?end`, `тим[\s-]?лид`, `тех[\s-]?лид`, `руководител[\pL]*`, `head[\s-]?of`,
		`chief[\s-]+technology[\s-]+officer`, `cto`, `engineering[\s-]+director`,
		`director[\s-]+of[\s-]+(?:engineering|development)`,
		`техническ[\pL]*[\s-]+директор[\pL]*`, `директор[\pL]*[\s-]+по[\s-]+разработк[\pL]*`,
		`руководител[\pL]*[\s-]+(?:разработк[\pL]*|отдел[\pL]*)`,
	),
}

func compileAliases(values ...string) []*regexp.Regexp {
	result := make([]*regexp.Regexp, 0, len(values))
	for _, value := range values {
		result = append(result, regexp.MustCompile(`(?i)(?:^|[^\pL\pN])(?:`+value+`)(?:$|[^\pL\pN])`))
	}
	return result
}

func hasAlias(value string, rules []*regexp.Regexp) bool {
	for _, rule := range rules {
		if rule.MatchString(value) {
			return true
		}
	}
	return false
}

func ClassifyVacancy(v Vacancy) Classification {
	roleIDs := officialRoleIDs(v)
	leadership := contains(roleIDs, "104") || hasAlias(v.Title, specializationAliases.leadership)
	if specialization := classifyText(v.Title); specialization != SpecializationUnknown {
		return Classification{Specialization: specialization, Leadership: leadership, Confidence: "high", Evidence: "title"}
	}
	skills := strings.Join(v.Skills, " ")
	if specialization := classifyText(skills); specialization != SpecializationUnknown {
		return Classification{Specialization: specialization, Leadership: leadership, Confidence: "medium", Evidence: "skills"}
	}
	if specialization := classifyText(v.Description); specialization != SpecializationUnknown {
		return Classification{Specialization: specialization, Leadership: leadership, Confidence: "low", Evidence: "description"}
	}
	return Classification{Specialization: SpecializationUnknown, Leadership: leadership, Confidence: "low", Evidence: "none"}
}

func classifyText(value string) Specialization {
	if strings.TrimSpace(value) == "" {
		return SpecializationUnknown
	}
	if hasAlias(value, specializationAliases.fullstack) {
		return SpecializationFullstack
	}
	hits := []struct {
		value Specialization
		rules []*regexp.Regexp
	}{
		{SpecializationFrontend, specializationAliases.frontend},
		{SpecializationBackend, specializationAliases.backend},
		{SpecializationMobile, specializationAliases.mobile},
		{SpecializationDevOps, specializationAliases.devops},
		{SpecializationDataML, specializationAliases.dataML},
		{SpecializationOther, specializationAliases.other},
	}
	var found Specialization
	for _, hit := range hits {
		if hasAlias(value, hit.rules) {
			if found != "" && found != hit.value {
				return SpecializationUnknown
			}
			found = hit.value
		}
	}
	if found == "" {
		return SpecializationUnknown
	}
	return found
}

func validateSpecialization(value any) error {
	if value == nil {
		return nil
	}
	text, ok := value.(string)
	if !ok || !supportedSpecializations[Specialization(text)] {
		return ErrInvalidPreferences
	}
	return nil
}
