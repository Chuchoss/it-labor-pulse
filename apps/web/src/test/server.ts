import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'

export const handlers = [
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
      method_version: 'vacancy_demand_v1',
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
      method_version: 'vacancy_demand_v1',
      available_years: [2026],
      first_observation: '2026-08-26',
      last_observation: '2026-08-26',
      publication_from: '2026-08-26',
      publication_to: '2026-08-26',
      complete_daily_count: 1,
      complete_weekly_count: 0,
      latest_complete_cycle: '2026-08-26T10:00:00Z',
      regions: [],
    }),
  ),
  http.get('*/api/v1/skills/top', () =>
    HttpResponse.json({
      data: [{ skill_id: 'go', name: 'Go', count: 10, share: 0.45 }],
    }),
  ),
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
