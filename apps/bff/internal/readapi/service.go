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
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	return s.repository.Dashboard(ctx, filter)
}

func (s *Service) ListRoles(ctx context.Context, filter AnalyticsFilter, page Page, sort string) (RolePage, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	return s.repository.ListRoles(ctx, filter, page, sort)
}

func (s *Service) GetRole(ctx context.Context, id string, filter AnalyticsFilter) (RoleStat, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	return s.repository.GetRole(ctx, id, filter)
}

func (s *Service) ListRegions(ctx context.Context, filter AnalyticsFilter, page Page) (RegionPage, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	return s.repository.ListRegions(ctx, filter, page)
}

func (s *Service) GetRegion(ctx context.Context, id string, filter AnalyticsFilter) (RegionStat, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	return s.repository.GetRegion(ctx, id, filter)
}

func (s *Service) SalaryTrends(ctx context.Context, filter AnalyticsFilter, grain string) (SalaryTrends, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	return s.repository.SalaryTrends(ctx, filter, grain)
}

func (s *Service) DemandTrends(ctx context.Context, filter AnalyticsFilter, grain string) (DemandTrends, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	return s.repository.DemandTrends(ctx, filter, grain)
}

func (s *Service) TopSkills(ctx context.Context, filter AnalyticsFilter, limit int) (TopSkills, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	return s.repository.TopSkills(ctx, filter, limit)
}

func (s *Service) ListVacancies(ctx context.Context, filter VacancyFilter) (VacancyPage, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	return s.repository.ListVacancies(ctx, filter)
}
