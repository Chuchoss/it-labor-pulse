import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import { delay, http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { renderPage } from '../test/render'
import { server } from '../test/server'
import { DashboardPage } from './DashboardPage'

function makeSkills(from: number, to: number) {
  return Array.from({ length: to - from + 1 }, (_, index) => {
    const rank = from + index
    return {
      skill_id: `skill-${rank}`,
      name: `Навык ${rank}`,
      count: 101 - rank,
      share: (101 - rank) / 100,
    }
  })
}

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

  it('shows ten skills and appends the next page without duplicates', async () => {
    server.use(
      http.get('*/api/v1/skills/top', ({ request }) => {
        const page = Number(new URL(request.url).searchParams.get('page'))
        return HttpResponse.json({
          data: page === 1 ? makeSkills(1, 10) : [makeSkills(10, 10)[0], ...makeSkills(11, 20)],
          page,
          page_size: 10,
          total: 20,
        })
      }),
    )

    renderPage(<DashboardPage />)

    expect(await screen.findByText('Навык 10')).toBeInTheDocument()
    expect(screen.queryByText('Навык 11')).not.toBeInTheDocument()
    expect(screen.getByText('Показано 10 из 20')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Показать ещё навыки' }))

    expect(await screen.findByText('Навык 20')).toBeInTheDocument()
    expect(screen.getAllByText('Навык 10')).toHaveLength(1)
    expect(screen.getByText('Показано 20 из 20')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Показать ещё навыки' })).not.toBeInTheDocument()
  })

  it('resets skills to the first page when the period changes', async () => {
    const requestedPages: string[] = []
    server.use(
      http.get('*/api/v1/skills/top', ({ request }) => {
        const url = new URL(request.url)
        const page = Number(url.searchParams.get('page'))
        const from = url.searchParams.get('from') || ''
        requestedPages.push(`${from}:${page}`)
        const prefix = from === '2026-08-01' ? 'Новый' : 'Старый'
        return HttpResponse.json({
          data: makeSkills(page === 1 ? 1 : 11, page === 1 ? 10 : 20).map((skill) => ({
            ...skill,
            name: `${prefix} ${skill.name}`,
          })),
          page,
          page_size: 10,
          total: 20,
        })
      }),
    )

    renderPage(<DashboardPage />)
    await screen.findByText('Старый Навык 10')
    fireEvent.click(screen.getByRole('button', { name: 'Показать ещё навыки' }))
    await screen.findByText('Старый Навык 20')

    fireEvent.change(screen.getByLabelText('С'), { target: { value: '2026-08-01' } })
    fireEvent.change(screen.getByLabelText('По'), { target: { value: '2026-08-20' } })
    fireEvent.click(screen.getByRole('button', { name: 'Применить' }))

    expect(await screen.findByText('Новый Навык 10')).toBeInTheDocument()
    expect(screen.queryByText('Старый Навык 20')).not.toBeInTheDocument()
    expect(screen.queryByText('Новый Навык 11')).not.toBeInTheDocument()
    expect(requestedPages).toContain('2026-08-01:1')
  })

  it('keeps the first page visible and retries a load-more failure', async () => {
    let secondPageAttempts = 0
    server.use(
      http.get('*/api/v1/skills/top', ({ request }) => {
        const page = Number(new URL(request.url).searchParams.get('page'))
        if (page === 2 && secondPageAttempts++ === 0) {
          return HttpResponse.json(
            { error: { code: 'DEPENDENCY_UNAVAILABLE', message: 'temporary error' } },
            { status: 503 },
          )
        }
        return HttpResponse.json({
          data: page === 1 ? makeSkills(1, 10) : makeSkills(11, 12),
          page,
          page_size: 10,
          total: 12,
        })
      }),
    )

    renderPage(<DashboardPage />)
    await screen.findByText('Навык 10')
    fireEvent.click(screen.getByRole('button', { name: 'Показать ещё навыки' }))

    expect(await screen.findByText('Не удалось загрузить ещё. Показано 10 из 12')).toBeInTheDocument()
    expect(screen.getByText('Навык 1')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Повторить загрузку навыков' }))

    expect(await screen.findByText('Навык 12')).toBeInTheDocument()
    expect(screen.getByText('Показано 12 из 12')).toBeInTheDocument()
  })

  it('switches each new ranking independently between count and salary', async () => {
    renderPage(<DashboardPage />)

    const languageCard = (await screen.findByText('Языки программирования')).closest(
      '.MuiCard-root',
    ) as HTMLElement
    expect(await within(languageCard).findByText('10 · 45%')).toBeInTheDocument()
    expect(screen.getByText('8 · 40%')).toBeInTheDocument()

    const languageGroup = within(languageCard).getByRole('group', {
      name: 'Метрика рейтинга: Языки программирования',
    })
    fireEvent.click(languageGroup.querySelector('[value="salary"]') as HTMLElement)

    expect(await within(languageCard).findByText('250 000 ₽ · n=6')).toBeInTheDocument()
    expect(screen.getByText('8 · 40%')).toBeInTheDocument()
    expect(within(languageCard).queryByText('10 · 45%')).not.toBeInTheDocument()
  })
})
