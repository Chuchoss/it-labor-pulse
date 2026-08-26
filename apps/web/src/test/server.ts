import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'

export const handlers = [
  http.get('*/api/v1/currencies', () =>
    HttpResponse.json({
      base_currency: 'RUB',
      rates: [
        {
          code: 'RUB',
          label: 'Российский рубль',
          symbol: '₽',
          rate_date: null,
          stale_days: null,
          available: true,
        },
        {
          code: 'USD',
          label: 'Доллар США',
          symbol: '$',
          rate_date: '2026-08-26',
          provider: 'cbr',
          stale_days: 0,
          available: true,
        },
        {
          code: 'EUR',
          label: 'Евро',
          symbol: '€',
          rate_date: '2026-08-26',
          provider: 'cbr',
          stale_days: 0,
          available: true,
        },
        {
          code: 'CNY',
          label: 'Китайский юань',
          symbol: '¥',
          rate_date: '2026-08-26',
          provider: 'cbr',
          stale_days: 0,
          available: true,
        },
      ],
    }),
  ),
  http.get('*/api/v1/dashboard/summary', () =>
    HttpResponse.json({
      period: { from: '2026-07-27', to: '2026-08-26' },
      vacancies_active: 22,
      vacancies_new: 7,
      median_salary: 245000,
      salary_currency: 'RUB',
      salary_sample_size: 12,
      top_roles: [],
      top_regions: [],
      generated_at: '2026-08-26T07:00:00Z',
      cache: 'MISS',
    }),
  ),
  http.get('*/api/v1/trends/salaries', () =>
    HttpResponse.json({
      grain: 'week',
      currency: 'RUB',
      points: [
        {
          period_start: '2026-08-18',
          median: 245000,
          p25: 200000,
          p75: 300000,
          sample_size: 12,
        },
      ],
    }),
  ),
  http.get('*/api/v1/trends/demand', () =>
    HttpResponse.json({
      grain: 'week',
      status: 'ready',
      source: 'hh',
      method_version: 'vacancy_demand_v2',
      points: [
        {
          period_start: '2026-08-18',
          active_count: 22,
          published_count: 7,
          new_count: 7,
          complete: true,
          source_day_count: 7,
        },
      ],
    }),
  ),
  http.get('*/api/v1/trends/coverage', () =>
    HttpResponse.json({
      status: 'ready',
      source: 'hh',
      method_version: 'vacancy_demand_v2',
      available_years: [2026],
      first_observation: '2026-08-26',
      last_observation: '2026-08-26',
      publication_from: '2026-08-26',
      publication_to: '2026-08-26',
      complete_daily_count: 1,
      complete_weekly_count: 0,
      expected_daily_count: 1,
      missed_daily_count: 0,
      incomplete_daily_count: 0,
      latest_incomplete_date: null,
      next_scheduled_cycle: '2026-08-27T01:00:00Z',
      latest_complete_cycle: '2026-08-26T10:00:00Z',
      regions: [],
    }),
  ),
  http.get('*/api/v1/skills/top', () =>
    HttpResponse.json({
      data: [{ skill_id: 'go', name: 'Go', count: 10, share: 0.45 }],
      page: 1,
      page_size: 10,
      total: 1,
    }),
  ),
  http.get('*/api/v1/rankings/programming-languages', ({ request }) => {
    const metric = new URL(request.url).searchParams.get('metric') || 'count'
    return HttpResponse.json({
      data: [
        {
          id: 'language-go',
          name: 'Go',
          rank: 1,
          vacancy_count: 10,
          share: 0.45,
          median_salary_rub: metric === 'salary' ? 250000 : null,
          salary_sample_size: 6,
        },
      ],
      metric,
      denominator: 22,
      min_salary_sample_size: 5,
      page: 1,
      page_size: 10,
      total: 1,
    })
  }),
  http.get('*/api/v1/rankings/managerial-roles', ({ request }) => {
    const metric = new URL(request.url).searchParams.get('metric') || 'count'
    return HttpResponse.json({
      data: [
        {
          id: 'role-project-manager',
          name: 'Руководитель проектов',
          rank: 1,
          vacancy_count: 8,
          share: 0.4,
          median_salary_rub: metric === 'salary' ? 220000 : null,
          salary_sample_size: 5,
        },
      ],
      metric,
      denominator: 20,
      min_salary_sample_size: 5,
      page: 1,
      page_size: 10,
      total: 1,
    })
  }),
  http.get('*/api/v1/regions', () =>
    HttpResponse.json({
      data: [],
      page: 1,
      page_size: 100,
      total: 0,
    }),
  ),
  http.get('*/api/v1/roles', () =>
    HttpResponse.json({
      data: [],
      page: 1,
      page_size: 100,
      total: 0,
    }),
  ),
  http.get('*/api/v1/vacancies', ({ request }) => {
    const url = new URL(request.url)
    return HttpResponse.json({
      data: [
        {
          id: '3fa85f64-5717-4562-b3fc-2c963f66afa6',
          source: 'hh',
          external_id: '123',
          title: url.searchParams.get('q') || 'Senior Go Developer',
          role_id: null,
          region_id: null,
          salary_from: 200000,
          salary_to: 300000,
          salary_currency: 'RUB',
          salary_gross: false,
          published_at: '2026-08-25T10:00:00Z',
          is_active: true,
          skills: ['Go', 'PostgreSQL'],
        },
      ],
      page: Number(url.searchParams.get('page') || 1),
      page_size: Number(url.searchParams.get('page_size') || 20),
      total: 22,
    })
  }),
]

export const server = setupServer(...handlers)
