package httpserver

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Chuchoss/it-labor-pulse/apps/bff/internal/readapi"
)

type stubReadService struct {
	listVacancies func(context.Context, readapi.VacancyFilter) (readapi.VacancyPage, error)
	getRole       func(context.Context, string, readapi.AnalyticsFilter) (readapi.RoleStat, error)
	topSkills     func(context.Context, readapi.AnalyticsFilter, readapi.Page) (readapi.TopSkills, error)
}

func (s stubReadService) Dashboard(context.Context, readapi.AnalyticsFilter) (readapi.DashboardSummary, error) {
	return readapi.DashboardSummary{}, nil
}

func (s stubReadService) ListRoles(
	context.Context,
	readapi.AnalyticsFilter,
	readapi.Page,
	string,
) (readapi.RolePage, error) {
	return readapi.RolePage{Data: []readapi.RoleStat{}}, nil
}

func (s stubReadService) GetRole(
	ctx context.Context,
	id string,
	filter readapi.AnalyticsFilter,
) (readapi.RoleStat, error) {
	if s.getRole != nil {
		return s.getRole(ctx, id, filter)
	}
	return readapi.RoleStat{}, nil
}

func (s stubReadService) ListRegions(
	context.Context,
	readapi.AnalyticsFilter,
	readapi.Page,
) (readapi.RegionPage, error) {
	return readapi.RegionPage{Data: []readapi.RegionStat{}}, nil
}

func (s stubReadService) GetRegion(
	context.Context,
	string,
	readapi.AnalyticsFilter,
) (readapi.RegionStat, error) {
	return readapi.RegionStat{}, nil
}

func (s stubReadService) SalaryTrends(
	context.Context,
	readapi.AnalyticsFilter,
	string,
) (readapi.SalaryTrends, error) {
	return readapi.SalaryTrends{Points: []readapi.SalaryPoint{}}, nil
}

func (s stubReadService) DemandTrends(
	context.Context,
	readapi.AnalyticsFilter,
	string,
) (readapi.DemandTrends, error) {
	return readapi.DemandTrends{Points: []readapi.DemandPoint{}}, nil
}

func (s stubReadService) TrendsCoverage(context.Context) (readapi.TrendsCoverage, error) {
	return readapi.TrendsCoverage{
		Status: "collecting", Source: "hh",
		AvailableYears: []int{}, Regions: []readapi.CoverageRegion{},
	}, nil
}

func (s stubReadService) TopSkills(
	ctx context.Context,
	filter readapi.AnalyticsFilter,
	page readapi.Page,
) (readapi.TopSkills, error) {
	if s.topSkills != nil {
		return s.topSkills(ctx, filter, page)
	}
	return readapi.TopSkills{Data: []readapi.SkillStat{}, Page: page.Number, PageSize: page.Size}, nil
}

func (s stubReadService) ListVacancies(
	ctx context.Context,
	filter readapi.VacancyFilter,
) (readapi.VacancyPage, error) {
	if s.listVacancies != nil {
		return s.listVacancies(ctx, filter)
	}
	return readapi.VacancyPage{Data: []readapi.Vacancy{}}, nil
}

func TestTopSkillsPagination(t *testing.T) {
	t.Parallel()

	var receivedPages []readapi.Page
	service := stubReadService{
		topSkills: func(
			_ context.Context,
			_ readapi.AnalyticsFilter,
			page readapi.Page,
		) (readapi.TopSkills, error) {
			receivedPages = append(receivedPages, page)
			return readapi.TopSkills{
				Data: []readapi.SkillStat{{
					SkillID: "00000000-0000-4000-8000-000000000002",
					Name:    "Go",
					Count:   9,
					Share:   0.45,
				}},
				Page: page.Number, PageSize: page.Size, Total: 21,
			}, nil
		},
	}
	server := New(Options{Addr: ":0", ReadService: service})

	for _, path := range []string{
		"/api/v1/skills/top?from=2026-08-01&to=2026-08-26&page=2&page_size=10",
		"/api/v1/skills/top?from=2026-08-01&to=2026-08-26&limit=7",
	} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		recorder := httptest.NewRecorder()
		server.Handler.ServeHTTP(recorder, request)
		require.Equal(t, http.StatusOK, recorder.Code)
	}

	require.Equal(t, []readapi.Page{{Number: 2, Size: 10}, {Number: 1, Size: 7}}, receivedPages)
}

func TestReadHandlerValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
	}{
		{name: "dashboard_requires_from", path: "/api/v1/dashboard/summary?to=2026-08-26"},
		{name: "invalid_date", path: "/api/v1/roles?from=bad&to=2026-08-26"},
		{name: "reversed_period", path: "/api/v1/regions?from=2026-08-27&to=2026-08-26"},
		{name: "page_below_minimum", path: "/api/v1/vacancies?page=0"},
		{name: "page_size_above_maximum", path: "/api/v1/vacancies?page_size=101"},
		{name: "invalid_active", path: "/api/v1/vacancies?only_active=1"},
		{name: "invalid_source", path: "/api/v1/vacancies?source=unknown"},
		{name: "invalid_role_uuid", path: "/api/v1/vacancies?role_id=role_go"},
		{name: "invalid_skill_uuid", path: "/api/v1/vacancies?skill_id=skill_go"},
		{name: "invalid_salary", path: "/api/v1/vacancies?salary_min=-1"},
		{name: "reversed_salary", path: "/api/v1/vacancies?salary_min=200000&salary_max=100000"},
		{name: "invalid_sort", path: "/api/v1/roles?from=2026-08-01&to=2026-08-26&sort=title"},
		{name: "invalid_grain", path: "/api/v1/trends/salaries?from=2026-08-01&to=2026-08-26&grain=year"},
		{name: "invalid_demand_grain", path: "/api/v1/trends/demand?from=2026-08-01&to=2026-08-26&grain=month"},
		{name: "invalid_role_group", path: "/api/v1/trends/demand?from=2026-08-01&to=2026-08-26&role_group=ai"},
		{name: "invalid_limit", path: "/api/v1/skills/top?from=2026-08-01&to=2026-08-26&limit=0"},
		{name: "invalid_skills_page_size", path: "/api/v1/skills/top?from=2026-08-01&to=2026-08-26&page_size=101"},
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := New(Options{Addr: ":0", Log: log, ReadService: stubReadService{}})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			request := httptest.NewRequest(http.MethodGet, tt.path, nil)
			request.Header.Set("X-Request-Id", "validation-request")
			recorder := httptest.NewRecorder()

			server.Handler.ServeHTTP(recorder, request)

			require.Equal(t, http.StatusBadRequest, recorder.Code)
			require.Equal(t, "application/json; charset=utf-8", recorder.Header().Get("Content-Type"))
			require.JSONEq(t, `{
				"error": {
					"code": "VALIDATION_ERROR",
					"message": "Invalid request parameters",
					"details": {"field": "`+expectedField(tt.path)+`", "reason": "`+expectedReason(tt.path)+`"},
					"request_id": "validation-request"
				}
			}`, recorder.Body.String())
		})
	}
}

func TestVacanciesDefaultsAndResponseEnvelope(t *testing.T) {
	t.Parallel()

	service := stubReadService{
		listVacancies: func(_ context.Context, filter readapi.VacancyFilter) (readapi.VacancyPage, error) {
			require.Equal(t, readapi.Page{Number: 1, Size: 20}, filter.Page)
			require.True(t, filter.OnlyActive)
			return readapi.VacancyPage{
				Data:     []readapi.Vacancy{},
				Page:     1,
				PageSize: 20,
				Total:    0,
			}, nil
		},
	}
	server := New(Options{Addr: ":0", ReadService: service})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/vacancies", nil)
	request.Header.Set("X-Request-Id", "vacancies-request")
	recorder := httptest.NewRecorder()

	server.Handler.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "vacancies-request", recorder.Header().Get("X-Request-Id"))
	require.NotEmpty(t, recorder.Header().Get("Traceparent"))
	require.JSONEq(t, `{"data":[],"page":1,"page_size":20,"total":0}`, recorder.Body.String())
}

func TestVacancyFilterCombination(t *testing.T) {
	t.Parallel()
	const (
		roleOne   = "10000000-0000-4000-8000-000000000001"
		roleTwo   = "10000000-0000-4000-8000-000000000002"
		regionOne = "20000000-0000-4000-8000-000000000001"
		skillOne  = "30000000-0000-4000-8000-000000000001"
	)
	service := stubReadService{
		listVacancies: func(_ context.Context, filter readapi.VacancyFilter) (readapi.VacancyPage, error) {
			require.Equal(t, []string{roleOne, roleTwo}, filter.RoleIDs)
			require.Equal(t, []string{regionOne}, filter.RegionIDs)
			require.Equal(t, []string{skillOne}, filter.SkillIDs)
			require.Equal(t, 100000.0, *filter.SalaryMin)
			require.Equal(t, 300000.0, *filter.SalaryMax)
			return readapi.VacancyPage{Data: []readapi.Vacancy{}, Page: 1, PageSize: 20}, nil
		},
	}
	server := New(Options{Addr: ":0", ReadService: service})
	path := "/api/v1/vacancies?role_id=" + roleOne + "," + roleTwo +
		"&region_id=" + regionOne + "&skill_id=" + skillOne +
		"&salary_min=100000&salary_max=300000"
	recorder := httptest.NewRecorder()
	server.Handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	require.Equal(t, http.StatusOK, recorder.Code)
}

func TestReadHandlerErrorMapping(t *testing.T) {
	t.Parallel()

	const roleID = "10000000-0000-4000-8000-000000000001"
	tests := []struct {
		name       string
		service    ReadService
		path       string
		wantStatus int
		wantCode   string
	}{
		{
			name: "not_found",
			service: stubReadService{getRole: func(
				context.Context,
				string,
				readapi.AnalyticsFilter,
			) (readapi.RoleStat, error) {
				return readapi.RoleStat{}, readapi.ErrNotFound
			}},
			path:       "/api/v1/roles/" + roleID + "?from=2026-08-01&to=2026-08-26",
			wantStatus: http.StatusNotFound,
			wantCode:   "NOT_FOUND",
		},
		{
			name: "database_error",
			service: stubReadService{listVacancies: func(
				context.Context,
				readapi.VacancyFilter,
			) (readapi.VacancyPage, error) {
				return readapi.VacancyPage{}, errors.New("database timeout")
			}},
			path:       "/api/v1/vacancies",
			wantStatus: http.StatusBadGateway,
			wantCode:   "DEPENDENCY_UNAVAILABLE",
		},
		{
			name:       "database_not_configured",
			path:       "/api/v1/vacancies",
			wantStatus: http.StatusBadGateway,
			wantCode:   "DEPENDENCY_UNAVAILABLE",
		},
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			server := New(Options{Addr: ":0", Log: log, ReadService: tt.service})
			request := httptest.NewRequest(http.MethodGet, tt.path, nil)
			request.Header.Set("X-Request-Id", "error-request")
			recorder := httptest.NewRecorder()

			server.Handler.ServeHTTP(recorder, request)

			require.Equal(t, tt.wantStatus, recorder.Code)
			require.Contains(t, recorder.Body.String(), `"code":"`+tt.wantCode+`"`)
			require.Contains(t, recorder.Body.String(), `"request_id":"error-request"`)
			require.NotContains(t, recorder.Body.String(), "database timeout")
		})
	}
}

func TestReadRoutesMatchOpenAPIPhaseOneSurface(t *testing.T) {
	t.Parallel()

	period := "?from=2026-08-01&to=2026-08-26"
	routes := []string{
		"/api/v1/dashboard/summary" + period,
		"/api/v1/roles" + period,
		"/api/v1/roles/10000000-0000-4000-8000-000000000001" + period,
		"/api/v1/regions" + period,
		"/api/v1/regions/20000000-0000-4000-8000-000000000001" + period,
		"/api/v1/trends/salaries" + period,
		"/api/v1/trends/demand" + period,
		"/api/v1/trends/coverage",
		"/api/v1/skills/top" + period,
		"/api/v1/vacancies",
	}
	server := New(Options{Addr: ":0", ReadService: stubReadService{}})
	for _, route := range routes {
		request := httptest.NewRequest(http.MethodGet, route, nil)
		recorder := httptest.NewRecorder()
		server.Handler.ServeHTTP(recorder, request)
		require.NotEqual(t, http.StatusNotFound, recorder.Code, route)
		require.Equal(t, http.StatusOK, recorder.Code, route)
	}
}

func expectedField(path string) string {
	switch {
	case stringsContains(path, "from=bad"), stringsContains(path, "from=2026-08-27"), stringsContains(path, "summary"):
		return "from"
	case stringsContains(path, "page_size"):
		return "page_size"
	case stringsContains(path, "page=0"):
		return "page"
	case stringsContains(path, "only_active"):
		return "only_active"
	case stringsContains(path, "source"):
		return "source"
	case stringsContains(path, "role_id"):
		return "role_id"
	case stringsContains(path, "skill_id"):
		return "skill_id"
	case stringsContains(path, "salary_min"):
		return "salary_min"
	case stringsContains(path, "sort"):
		return "sort"
	case stringsContains(path, "grain"):
		return "grain"
	case stringsContains(path, "role_group"):
		return "role_group"
	default:
		return "limit"
	}
}

func expectedReason(path string) string {
	switch {
	case stringsContains(path, "summary"):
		return "is required"
	case stringsContains(path, "from=bad"):
		return "must use YYYY-MM-DD"
	case stringsContains(path, "from=2026-08-27"):
		return "must not be after to"
	case stringsContains(path, "page="), stringsContains(path, "page_size"), stringsContains(path, "limit"):
		return "is outside the allowed range"
	case stringsContains(path, "only_active"):
		return "must be true or false"
	case stringsContains(path, "source"):
		return "is not supported"
	case stringsContains(path, "role_id"):
		return "must be a UUID"
	case stringsContains(path, "skill_id"):
		return "must be a UUID"
	case stringsContains(path, "salary_min=200000"):
		return "must not exceed salary_max"
	case stringsContains(path, "salary_min"):
		return "is outside the allowed range"
	case stringsContains(path, "sort"):
		return "must be count or median_salary"
	case stringsContains(path, "role_group"):
		return "must be software_development, analytics or quality_assurance"
	case stringsContains(path, "trends/demand"):
		return "must be day or week"
	default:
		return "must be day, week or month"
	}
}

func stringsContains(value, part string) bool {
	return strings.Contains(value, part)
}
