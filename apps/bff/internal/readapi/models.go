package readapi

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("not found")

type Period struct {
	From time.Time `json:"-"`
	To   time.Time `json:"-"`
}

type Page struct {
	Number int
	Size   int
}

type AnalyticsFilter struct {
	Period    Period
	RoleID    string
	RoleGroup string
	RegionID  string
	Source    string
}

type DashboardSummary struct {
	Period          PeriodResponse `json:"period"`
	VacanciesActive int64          `json:"vacancies_active"`
	VacanciesNew    int64          `json:"vacancies_new"`
	MedianSalary    float64        `json:"median_salary"`
	SalaryCurrency  string         `json:"salary_currency"`
	SalarySample    int64          `json:"salary_sample_size"`
	TopRoles        []RoleCount    `json:"top_roles"`
	TopRegions      []RegionCount  `json:"top_regions"`
	GeneratedAt     time.Time      `json:"generated_at"`
	Cache           string         `json:"cache"`
}

type PeriodResponse struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type RoleCount struct {
	RoleID string `json:"role_id"`
	Title  string `json:"title"`
	Count  int64  `json:"count"`
}

type RegionCount struct {
	RegionID string `json:"region_id"`
	Title    string `json:"title"`
	Count    int64  `json:"count"`
}

type DimensionStat struct {
	ID             string  `json:"-"`
	Title          string  `json:"title"`
	VacanciesCount int64   `json:"vacancies_count"`
	MedianSalary   float64 `json:"median_salary"`
	P25Salary      float64 `json:"p25_salary"`
	P75Salary      float64 `json:"p75_salary"`
	Currency       string  `json:"currency"`
}

type RoleStat struct {
	RoleID string `json:"role_id"`
	DimensionStat
}

type RegionStat struct {
	RegionID string `json:"region_id"`
	DimensionStat
}

type RolePage struct {
	Data     []RoleStat `json:"data"`
	Page     int        `json:"page"`
	PageSize int        `json:"page_size"`
	Total    int64      `json:"total"`
}

type RegionPage struct {
	Data     []RegionStat `json:"data"`
	Page     int          `json:"page"`
	PageSize int          `json:"page_size"`
	Total    int64        `json:"total"`
}

type SalaryPoint struct {
	PeriodStart string  `json:"period_start"`
	Median      float64 `json:"median"`
	P25         float64 `json:"p25"`
	P75         float64 `json:"p75"`
	SampleSize  int64   `json:"sample_size"`
}

type SalaryTrends struct {
	Grain    string        `json:"grain"`
	Currency string        `json:"currency"`
	Points   []SalaryPoint `json:"points"`
}

type DemandPoint struct {
	PeriodStart    string `json:"period_start"`
	ActiveCount    int64  `json:"active_count"`
	PublishedCount int64  `json:"published_count"`
	NewCount       int64  `json:"new_count"`
	Complete       bool   `json:"complete"`
	SourceDayCount int    `json:"source_day_count"`
}

type DemandTrends struct {
	Grain         string        `json:"grain"`
	Status        string        `json:"status"`
	Source        string        `json:"source"`
	MethodVersion string        `json:"method_version,omitempty"`
	Points        []DemandPoint `json:"points"`
}

type CoverageRegion struct {
	RegionID string `json:"region_id"`
	Name     string `json:"name"`
}

type TrendsCoverage struct {
	Status              string           `json:"status"`
	Source              string           `json:"source"`
	MethodVersion       string           `json:"method_version,omitempty"`
	AvailableYears      []int            `json:"available_years"`
	FirstObservation    *string          `json:"first_observation"`
	LastObservation     *string          `json:"last_observation"`
	PublicationFrom     *string          `json:"publication_from"`
	PublicationTo       *string          `json:"publication_to"`
	CompleteDailyCount  int64            `json:"complete_daily_count"`
	CompleteWeeklyCount int64            `json:"complete_weekly_count"`
	LatestCompleteCycle *time.Time       `json:"latest_complete_cycle"`
	Regions             []CoverageRegion `json:"regions"`
}

type SkillStat struct {
	SkillID string  `json:"skill_id"`
	Name    string  `json:"name"`
	Count   int64   `json:"count"`
	Share   float64 `json:"share"`
}

type TopSkills struct {
	Data     []SkillStat `json:"data"`
	Page     int         `json:"page"`
	PageSize int         `json:"page_size"`
	Total    int64       `json:"total"`
}

type VacancyFilter struct {
	Query      string
	RoleIDs    []string
	RegionIDs  []string
	SkillIDs   []string
	Source     string
	OnlyActive bool
	SalaryMin  *float64
	SalaryMax  *float64
	Page       Page
}

type Vacancy struct {
	ID             string     `json:"id"`
	Source         string     `json:"source"`
	ExternalID     string     `json:"external_id"`
	Title          string     `json:"title"`
	RoleID         *string    `json:"role_id"`
	RegionID       *string    `json:"region_id"`
	SalaryFrom     *float64   `json:"salary_from"`
	SalaryTo       *float64   `json:"salary_to"`
	SalaryCurrency *string    `json:"salary_currency"`
	SalaryGross    *bool      `json:"salary_gross"`
	PublishedAt    *time.Time `json:"published_at"`
	IsActive       bool       `json:"is_active"`
	Skills         []string   `json:"skills"`
}

type VacancyPage struct {
	Data     []Vacancy `json:"data"`
	Page     int       `json:"page"`
	PageSize int       `json:"page_size"`
	Total    int64     `json:"total"`
}

type Repository interface {
	Dashboard(context.Context, AnalyticsFilter) (DashboardSummary, error)
	ListRoles(context.Context, AnalyticsFilter, Page, string) (RolePage, error)
	GetRole(context.Context, string, AnalyticsFilter) (RoleStat, error)
	ListRegions(context.Context, AnalyticsFilter, Page) (RegionPage, error)
	GetRegion(context.Context, string, AnalyticsFilter) (RegionStat, error)
	SalaryTrends(context.Context, AnalyticsFilter, string) (SalaryTrends, error)
	DemandTrends(context.Context, AnalyticsFilter, string) (DemandTrends, error)
	TrendsCoverage(context.Context) (TrendsCoverage, error)
	TopSkills(context.Context, AnalyticsFilter, Page) (TopSkills, error)
	ListVacancies(context.Context, VacancyFilter) (VacancyPage, error)
}
