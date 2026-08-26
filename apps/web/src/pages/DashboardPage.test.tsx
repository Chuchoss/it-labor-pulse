import { fireEvent, screen, waitFor } from '@testing-library/react'
import { delay, http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { renderPage } from '../test/render'
import { server } from '../test/server'
import { DashboardPage } from './DashboardPage'

describe('DashboardPage', () => {
  it('shows loading skeletons and then renders API data', async () => {
    server.use(
      http.get('*/api/v1/dashboard/summary', async () => {
        await delay(80)
        return HttpResponse.json({
          period: { from: '2026-07-27', to: '2026-08-26' },
          vacancies_active: 22,
          vacancies_new: 7,
          median_salary: 245000,
          salary_currency: 'RUB',
          salary_sample_size: 12,
        })
      }),
    )

    const { container } = renderPage(<DashboardPage />)
    expect(container.querySelector('.MuiSkeleton-root')).toBeInTheDocument()
    expect(await screen.findByText('245 000 ₽')).toBeInTheDocument()
    expect(screen.getByText('22')).toBeInTheDocument()
  })

  it('renders an empty state for a sparse trend response', async () => {
    server.use(
      http.get('*/api/v1/trends/demand', () =>
        HttpResponse.json({ grain: 'week', points: [] }),
      ),
    )

    renderPage(<DashboardPage />)
    expect(await screen.findByText('Данных пока нет')).toBeInTheDocument()
  })

  it('shows canonical API error and request id', async () => {
    server.use(
      http.get('*/api/v1/dashboard/summary', () =>
        HttpResponse.json(
          {
            error: {
              code: 'DEPENDENCY_UNAVAILABLE',
              message: 'database unavailable',
              request_id: 'req-test-42',
            },
          },
          { status: 502 },
        ),
      ),
    )

    renderPage(<DashboardPage />)
    expect(await screen.findByText('database unavailable')).toBeInTheDocument()
    expect(screen.getByText('ID запроса: req-test-42')).toBeInTheDocument()
  })

  it('puts the selected period into API query parameters', async () => {
    let requestedUrl = ''
    server.use(
      http.get('*/api/v1/dashboard/summary', ({ request }) => {
        requestedUrl = request.url
        return HttpResponse.json({
          period: { from: '2026-08-01', to: '2026-08-20' },
          vacancies_active: 3,
          median_salary: 0,
          salary_currency: 'RUB',
          salary_sample_size: 0,
        })
      }),
    )

    renderPage(<DashboardPage />)
    fireEvent.change(screen.getByLabelText('С'), { target: { value: '2026-08-01' } })
    fireEvent.change(screen.getByLabelText('По'), { target: { value: '2026-08-20' } })
    fireEvent.click(screen.getByRole('button', { name: 'Применить' }))

    await waitFor(() => {
      expect(requestedUrl).toContain('from=2026-08-01')
      expect(requestedUrl).toContain('to=2026-08-20')
    })
  })
})
