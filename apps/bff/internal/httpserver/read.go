package httpserver

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Chuchoss/it-labor-pulse/apps/bff/internal/readapi"
	"github.com/Chuchoss/it-labor-pulse/libs/go-common/httpx"
)

type ReadService interface {
	Dashboard(context.Context, readapi.AnalyticsFilter) (readapi.DashboardSummary, error)
	ListRoles(context.Context, readapi.AnalyticsFilter, readapi.Page, string) (readapi.RolePage, error)
	GetRole(context.Context, string, readapi.AnalyticsFilter) (readapi.RoleStat, error)
	ListRegions(context.Context, readapi.AnalyticsFilter, readapi.Page) (readapi.RegionPage, error)
	GetRegion(context.Context, string, readapi.AnalyticsFilter) (readapi.RegionStat, error)
	SalaryTrends(context.Context, readapi.AnalyticsFilter, string) (readapi.SalaryTrends, error)
	DemandTrends(context.Context, readapi.AnalyticsFilter, string) (readapi.DemandTrends, error)
	TrendsCoverage(context.Context) (readapi.TrendsCoverage, error)
	TopSkills(context.Context, readapi.AnalyticsFilter, readapi.Page) (readapi.TopSkills, error)
	ListVacancies(context.Context, readapi.VacancyFilter) (readapi.VacancyPage, error)
}

type ReadHandler struct {
	service ReadService
	log     *slog.Logger
}

func NewReadHandler(service ReadService, log *slog.Logger) *ReadHandler {
	if log == nil {
		log = slog.Default()
	}
	return &ReadHandler{service: service, log: log}
}

func (h *ReadHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/dashboard/summary", h.dashboard)
	mux.HandleFunc("GET /api/v1/roles", h.listRoles)
	mux.HandleFunc("GET /api/v1/roles/{role_id}", h.getRole)
	mux.HandleFunc("GET /api/v1/regions", h.listRegions)
	mux.HandleFunc("GET /api/v1/regions/{region_id}", h.getRegion)
	mux.HandleFunc("GET /api/v1/trends/salaries", h.salaryTrends)
	mux.HandleFunc("GET /api/v1/trends/demand", h.demandTrends)
	mux.HandleFunc("GET /api/v1/trends/coverage", h.trendsCoverage)
	mux.HandleFunc("GET /api/v1/skills/top", h.topSkills)
	mux.HandleFunc("GET /api/v1/vacancies", h.listVacancies)
}

func (h *ReadHandler) dashboard(w http.ResponseWriter, r *http.Request) {
	filter, err := parseAnalyticsFilter(r.URL.Query(), true, false)
	if err != nil {
		h.validationError(w, r, err)
		return
	}
	if h.requireService(w, r) {
		return
	}
	result, err := h.service.Dashboard(r.Context(), filter)
	h.respond(w, r, result, err)
}

func (h *ReadHandler) listRoles(w http.ResponseWriter, r *http.Request) {
	filter, err := parseAnalyticsFilter(r.URL.Query(), false, true)
	if err != nil {
		h.validationError(w, r, err)
		return
	}
	page, err := parsePage(r.URL.Query())
	if err != nil {
		h.validationError(w, r, err)
		return
	}
	sortBy := queryDefault(r.URL.Query(), "sort", "count")
	if sortBy != "count" && sortBy != "median_salary" {
		h.validationError(w, r, fieldError{"sort", "must be count or median_salary"})
		return
	}
	if h.requireService(w, r) {
		return
	}
	result, err := h.service.ListRoles(r.Context(), filter, page, sortBy)
	h.respond(w, r, result, err)
}

func (h *ReadHandler) getRole(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("role_id")
	if err := validateUUID("role_id", id); err != nil {
		h.validationError(w, r, err)
		return
	}
	filter, err := parseAnalyticsFilter(r.URL.Query(), false, false)
	if err != nil {
		h.validationError(w, r, err)
		return
	}
	if h.requireService(w, r) {
		return
	}
	result, err := h.service.GetRole(r.Context(), id, filter)
	h.respond(w, r, result, err)
}

func (h *ReadHandler) listRegions(w http.ResponseWriter, r *http.Request) {
	filter, err := parseAnalyticsFilter(r.URL.Query(), true, false)
	if err != nil {
		h.validationError(w, r, err)
		return
	}
	page, err := parsePage(r.URL.Query())
	if err != nil {
		h.validationError(w, r, err)
		return
	}
	if h.requireService(w, r) {
		return
	}
	result, err := h.service.ListRegions(r.Context(), filter, page)
	h.respond(w, r, result, err)
}

func (h *ReadHandler) getRegion(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("region_id")
	if err := validateUUID("region_id", id); err != nil {
		h.validationError(w, r, err)
		return
	}
	filter, err := parseAnalyticsFilter(r.URL.Query(), false, false)
	if err != nil {
		h.validationError(w, r, err)
		return
	}
	if h.requireService(w, r) {
		return
	}
	result, err := h.service.GetRegion(r.Context(), id, filter)
	h.respond(w, r, result, err)
}

func (h *ReadHandler) salaryTrends(w http.ResponseWriter, r *http.Request) {
	h.trends(w, r, true)
}

func (h *ReadHandler) demandTrends(w http.ResponseWriter, r *http.Request) {
	filter, err := parseDemandFilter(r.URL.Query())
	if err != nil {
		h.validationError(w, r, err)
		return
	}
	grain := queryDefault(r.URL.Query(), "grain", "week")
	if grain != "day" && grain != "week" {
		h.validationError(w, r, fieldError{"grain", "must be day or week"})
		return
	}
	if h.requireService(w, r) {
		return
	}
	result, serviceErr := h.service.DemandTrends(r.Context(), filter, grain)
	h.respond(w, r, result, serviceErr)
}

func (h *ReadHandler) trends(w http.ResponseWriter, r *http.Request, salary bool) {
	filter, err := parseAnalyticsFilter(r.URL.Query(), true, false)
	if err != nil {
		h.validationError(w, r, err)
		return
	}
	grain := queryDefault(r.URL.Query(), "grain", "week")
	if grain != "day" && grain != "week" && grain != "month" {
		h.validationError(w, r, fieldError{"grain", "must be day, week or month"})
		return
	}
	if h.requireService(w, r) {
		return
	}
	if salary {
		result, serviceErr := h.service.SalaryTrends(r.Context(), filter, grain)
		h.respond(w, r, result, serviceErr)
		return
	}
	result, serviceErr := h.service.DemandTrends(r.Context(), filter, grain)
	h.respond(w, r, result, serviceErr)
}

func (h *ReadHandler) trendsCoverage(w http.ResponseWriter, r *http.Request) {
	if h.requireService(w, r) {
		return
	}
	result, err := h.service.TrendsCoverage(r.Context())
	h.respond(w, r, result, err)
}

func (h *ReadHandler) topSkills(w http.ResponseWriter, r *http.Request) {
	filter, err := parseAnalyticsFilter(r.URL.Query(), true, false)
	if err != nil {
		h.validationError(w, r, err)
		return
	}
	page, err := parseTopSkillsPage(r.URL.Query())
	if err != nil {
		h.validationError(w, r, err)
		return
	}
	if h.requireService(w, r) {
		return
	}
	result, err := h.service.TopSkills(r.Context(), filter, page)
	h.respond(w, r, result, err)
}

func (h *ReadHandler) listVacancies(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	page, err := parsePage(values)
	if err != nil {
		h.validationError(w, r, err)
		return
	}
	roleIDs, err := parseUUIDList(values, "role_id", 20)
	if err != nil {
		h.validationError(w, r, err)
		return
	}
	regionIDs, err := parseUUIDList(values, "region_id", 20)
	if err != nil {
		h.validationError(w, r, err)
		return
	}
	skillIDs, err := parseUUIDList(values, "skill_id", 20)
	if err != nil {
		h.validationError(w, r, err)
		return
	}
	source := strings.TrimSpace(values.Get("source"))
	if err := validateSource(source); err != nil {
		h.validationError(w, r, err)
		return
	}
	onlyActive, err := parseBool(values, "only_active", true)
	if err != nil {
		h.validationError(w, r, err)
		return
	}
	salaryMin, err := parseOptionalFloat(values, "salary_min", 0, 2_000_000)
	if err != nil {
		h.validationError(w, r, err)
		return
	}
	salaryMax, err := parseOptionalFloat(values, "salary_max", 0, 2_000_000)
	if err != nil {
		h.validationError(w, r, err)
		return
	}
	if salaryMin != nil && salaryMax != nil && *salaryMin > *salaryMax {
		h.validationError(w, r, fieldError{"salary_min", "must not exceed salary_max"})
		return
	}
	query := strings.TrimSpace(values.Get("q"))
	if len([]rune(query)) > 200 {
		h.validationError(w, r, fieldError{"q", "must be at most 200 characters"})
		return
	}
	if h.requireService(w, r) {
		return
	}
	result, err := h.service.ListVacancies(r.Context(), readapi.VacancyFilter{
		Query:      query,
		RoleIDs:    roleIDs,
		RegionIDs:  regionIDs,
		SkillIDs:   skillIDs,
		Source:     source,
		OnlyActive: onlyActive,
		SalaryMin:  salaryMin,
		SalaryMax:  salaryMax,
		Page:       page,
	})
	h.respond(w, r, result, err)
}

func (h *ReadHandler) requireService(w http.ResponseWriter, r *http.Request) bool {
	if h.service != nil {
		return false
	}
	h.dependencyError(w, r, errors.New("database is not configured"))
	return true
}

func (h *ReadHandler) respond(w http.ResponseWriter, r *http.Request, result any, err error) {
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, result)
	case errors.Is(err, readapi.ErrNotFound):
		writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "Resource not found", nil, httpx.RequestID(r.Context()))
	default:
		h.dependencyError(w, r, err)
	}
}

func (h *ReadHandler) validationError(w http.ResponseWriter, r *http.Request, err error) {
	details := map[string]any{}
	var fieldErr fieldError
	if errors.As(err, &fieldErr) {
		details["field"] = fieldErr.field
		details["reason"] = fieldErr.reason
	}
	writeAPIError(
		w,
		http.StatusBadRequest,
		"VALIDATION_ERROR",
		"Invalid request parameters",
		details,
		httpx.RequestID(r.Context()),
	)
}

func (h *ReadHandler) dependencyError(w http.ResponseWriter, r *http.Request, err error) {
	h.log.Error(
		"dependency_error",
		"dep", "pg",
		"request_id", httpx.RequestID(r.Context()),
		"trace_id", httpx.TraceID(r.Context()),
		"error", err.Error(),
	)
	writeAPIError(
		w,
		http.StatusBadGateway,
		"DEPENDENCY_UNAVAILABLE",
		"Data service is temporarily unavailable",
		nil,
		httpx.RequestID(r.Context()),
	)
}

type fieldError struct {
	field  string
	reason string
}

func (e fieldError) Error() string {
	return e.field + ": " + e.reason
}

func parseAnalyticsFilter(values url.Values, allowRole, allowSource bool) (readapi.AnalyticsFilter, error) {
	from, err := parseDate(values, "from")
	if err != nil {
		return readapi.AnalyticsFilter{}, err
	}
	to, err := parseDate(values, "to")
	if err != nil {
		return readapi.AnalyticsFilter{}, err
	}
	if from.After(to) {
		return readapi.AnalyticsFilter{}, fieldError{"from", "must not be after to"}
	}

	filter := readapi.AnalyticsFilter{Period: readapi.Period{From: from, To: to}}
	if allowRole {
		filter.RoleID = strings.TrimSpace(values.Get("role_id"))
		if filter.RoleID != "" {
			if err := validateUUID("role_id", filter.RoleID); err != nil {
				return readapi.AnalyticsFilter{}, err
			}
		}
	}
	filter.RegionID = strings.TrimSpace(values.Get("region_id"))
	if filter.RegionID != "" {
		if err := validateUUID("region_id", filter.RegionID); err != nil {
			return readapi.AnalyticsFilter{}, err
		}
	}
	if allowSource {
		filter.Source = strings.TrimSpace(values.Get("source"))
		if err := validateSource(filter.Source); err != nil {
			return readapi.AnalyticsFilter{}, err
		}
	}
	return filter, nil
}

func parseDemandFilter(values url.Values) (readapi.AnalyticsFilter, error) {
	from, err := parseDate(values, "from")
	if err != nil {
		return readapi.AnalyticsFilter{}, err
	}
	to, err := parseDate(values, "to")
	if err != nil {
		return readapi.AnalyticsFilter{}, err
	}
	if from.After(to) {
		return readapi.AnalyticsFilter{}, fieldError{"from", "must not be after to"}
	}
	filter := readapi.AnalyticsFilter{
		Period:    readapi.Period{From: from, To: to},
		RoleGroup: strings.TrimSpace(values.Get("role_group")),
		RegionID:  strings.TrimSpace(values.Get("region_id")),
		Source:    strings.TrimSpace(values.Get("source")),
	}
	switch filter.RoleGroup {
	case "", "software_development", "analytics", "quality_assurance":
	default:
		return readapi.AnalyticsFilter{}, fieldError{
			"role_group",
			"must be software_development, analytics or quality_assurance",
		}
	}
	if filter.RegionID != "" {
		if err := validateUUID("region_id", filter.RegionID); err != nil {
			return readapi.AnalyticsFilter{}, err
		}
	}
	if err := validateSource(filter.Source); err != nil {
		return readapi.AnalyticsFilter{}, err
	}
	return filter, nil
}

func parseDate(values url.Values, field string) (time.Time, error) {
	raw := strings.TrimSpace(values.Get(field))
	if raw == "" {
		return time.Time{}, fieldError{field, "is required"}
	}
	value, err := time.Parse(time.DateOnly, raw)
	if err != nil {
		return time.Time{}, fieldError{field, "must use YYYY-MM-DD"}
	}
	return value, nil
}

func parsePage(values url.Values) (readapi.Page, error) {
	number, err := parseInt(values, "page", 1, 1, 0)
	if err != nil {
		return readapi.Page{}, err
	}
	size, err := parseInt(values, "page_size", 20, 1, 100)
	if err != nil {
		return readapi.Page{}, err
	}
	if number > math.MaxInt/size {
		return readapi.Page{}, fieldError{"page", "is too large"}
	}
	return readapi.Page{Number: number, Size: size}, nil
}

func parseTopSkillsPage(values url.Values) (readapi.Page, error) {
	if strings.TrimSpace(values.Get("limit")) == "" {
		return parsePage(values)
	}
	if strings.TrimSpace(values.Get("page")) != "" || strings.TrimSpace(values.Get("page_size")) != "" {
		return readapi.Page{}, fieldError{"limit", "must not be combined with page or page_size"}
	}
	limit, err := parseInt(values, "limit", 20, 1, 100)
	if err != nil {
		return readapi.Page{}, err
	}
	return readapi.Page{Number: 1, Size: limit}, nil
}

func parseInt(values url.Values, field string, fallback, minimum, maximum int) (int, error) {
	raw := strings.TrimSpace(values.Get(field))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < minimum || (maximum > 0 && value > maximum) {
		reason := "is outside the allowed range"
		return 0, fieldError{field, reason}
	}
	return value, nil
}

func parseBool(values url.Values, field string, fallback bool) (bool, error) {
	raw := strings.TrimSpace(values.Get(field))
	if raw == "" {
		return fallback, nil
	}
	switch raw {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fieldError{field, "must be true or false"}
	}
}

func parseOptionalFloat(values url.Values, field string, minimum, maximum float64) (*float64, error) {
	raw := strings.TrimSpace(values.Get(field))
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < minimum || value > maximum {
		return nil, fieldError{field, "is outside the allowed range"}
	}
	return &value, nil
}

func parseUUIDList(values url.Values, field string, maximum int) ([]string, error) {
	result := make([]string, 0)
	seen := map[string]struct{}{}
	for _, raw := range values[field] {
		for _, part := range strings.Split(raw, ",") {
			value := strings.TrimSpace(part)
			if value == "" {
				continue
			}
			if err := validateUUID(field, value); err != nil {
				return nil, err
			}
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			result = append(result, value)
			if len(result) > maximum {
				return nil, fieldError{field, "has too many values"}
			}
		}
	}
	return result, nil
}

func queryDefault(values url.Values, field, fallback string) string {
	value := strings.TrimSpace(values.Get(field))
	if value == "" {
		return fallback
	}
	return value
}

func validateUUID(field, value string) error {
	var uuid pgtype.UUID
	if err := uuid.Scan(value); err != nil || !uuid.Valid {
		return fieldError{field, "must be a UUID"}
	}
	return nil
}

func validateSource(source string) error {
	switch source {
	case "", "hh", "superjob", "remotive", "adzuna":
		return nil
	default:
		return fieldError{"source", "is not supported"}
	}
}

type errorResponse struct {
	Error apiError `json:"error"`
}

type apiError struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Details   map[string]any `json:"details,omitempty"`
	RequestID string         `json:"request_id"`
}

func writeAPIError(
	w http.ResponseWriter,
	status int,
	code string,
	message string,
	details map[string]any,
	requestID string,
) {
	writeJSON(w, status, errorResponse{Error: apiError{
		Code:      code,
		Message:   message,
		Details:   details,
		RequestID: requestID,
	}})
}
