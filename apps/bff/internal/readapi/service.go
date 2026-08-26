package readapi

import (
	"context"
	"time"
)

const defaultQueryTimeout = 5 * time.Second

type Service struct {
	repository Repository
	timeout    time.Duration
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository, timeout: defaultQueryTimeout}
}

func (s *Service) Dashboard(ctx context.Context, filter AnalyticsFilter) (DashboardSummary, error) {
	return withTimeout(ctx, s.timeout, func(ctx context.Context) (DashboardSummary, error) {
		return s.repository.Dashboard(ctx, filter)
	})
}

func (s *Service) ListRoles(ctx context.Context, filter AnalyticsFilter, page Page, sort string) (RolePage, error) {
	return withTimeout(ctx, s.timeout, func(ctx context.Context) (RolePage, error) {
		return s.repository.ListRoles(ctx, filter, page, sort)
	})
}

func (s *Service) GetRole(ctx context.Context, id string, filter AnalyticsFilter) (RoleStat, error) {
	return withTimeout(ctx, s.timeout, func(ctx context.Context) (RoleStat, error) {
		return s.repository.GetRole(ctx, id, filter)
	})
}

func (s *Service) ListRegions(ctx context.Context, filter AnalyticsFilter, page Page) (RegionPage, error) {
	return withTimeout(ctx, s.timeout, func(ctx context.Context) (RegionPage, error) {
		return s.repository.ListRegions(ctx, filter, page)
	})
}

func (s *Service) GetRegion(ctx context.Context, id string, filter AnalyticsFilter) (RegionStat, error) {
	return withTimeout(ctx, s.timeout, func(ctx context.Context) (RegionStat, error) {
		return s.repository.GetRegion(ctx, id, filter)
	})
}

func (s *Service) SalaryTrends(ctx context.Context, filter AnalyticsFilter, grain string) (SalaryTrends, error) {
	return withTimeout(ctx, s.timeout, func(ctx context.Context) (SalaryTrends, error) {
		return s.repository.SalaryTrends(ctx, filter, grain)
	})
}

func (s *Service) DemandTrends(ctx context.Context, filter AnalyticsFilter, grain string) (DemandTrends, error) {
	return withTimeout(ctx, s.timeout, func(ctx context.Context) (DemandTrends, error) {
		return s.repository.DemandTrends(ctx, filter, grain)
	})
}

func (s *Service) TrendsCoverage(ctx context.Context) (TrendsCoverage, error) {
	return withTimeout(ctx, s.timeout, func(ctx context.Context) (TrendsCoverage, error) {
		return s.repository.TrendsCoverage(ctx)
	})
}

func (s *Service) TopSkills(ctx context.Context, filter AnalyticsFilter, limit int) (TopSkills, error) {
	return withTimeout(ctx, s.timeout, func(ctx context.Context) (TopSkills, error) {
		return s.repository.TopSkills(ctx, filter, limit)
	})
}

func (s *Service) ListVacancies(ctx context.Context, filter VacancyFilter) (VacancyPage, error) {
	return withTimeout(ctx, s.timeout, func(ctx context.Context) (VacancyPage, error) {
		return s.repository.ListVacancies(ctx, filter)
	})
}

func withTimeout[T any](
	ctx context.Context,
	timeout time.Duration,
	query func(context.Context) (T, error),
) (T, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return query(ctx)
}
