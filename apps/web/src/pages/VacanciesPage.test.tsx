import { act, fireEvent, screen, waitFor } from '@testing-library/react'
import { delay, http, HttpResponse } from 'msw'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { renderPage } from '../test/render'
import { server } from '../test/server'
import { getRegionLabel } from '../utils/regions'
import { VacanciesPage } from './VacanciesPage'
import {
  dedupeVacancies,
  getNextVacancyPageParam,
  mergeFreshVacancies,
  parseVacancyPollInterval,
} from './vacancyPagination'

let intersectionCallback: IntersectionObserverCallback | undefined

class IntersectionObserverMock {
  constructor(callback: IntersectionObserverCallback) {
    intersectionCallback = callback
  }
  observe() {}
  unobserve() {}
  disconnect() {}
  takeRecords() {
    return []
  }
  readonly root = null
  readonly rootMargin = ''
  readonly thresholds = []
}

function intersectSentinel() {
  act(() => {
    intersectionCallback?.(
      [{ isIntersecting: true } as IntersectionObserverEntry],
      {} as IntersectionObserver,
    )
  })
}

afterEach(() => {
  intersectionCallback = undefined
  vi.unstubAllGlobals()
})

describe('VacanciesPage', () => {
  it('loads the next page from the sentinel, dedupes and stops at total', async () => {
    vi.stubGlobal('IntersectionObserver', IntersectionObserverMock)
    const requestedUrls: string[] = []
    const requestedRegionPages: string[] = []
    const requestedRegionUrls: string[] = []
    server.use(
      http.get('*/api/v1/regions', ({ request }) => {
        const url = new URL(request.url)
        const currentPage = url.searchParams.get('page') || '1'
        requestedRegionPages.push(currentPage)
        requestedRegionUrls.push(request.url)
        return HttpResponse.json({
          data:
            currentPage === '1'
              ? [{ region_id: 'another-region', title: 'Другой регион' }]
              : [{ region_id: 'region-2', title: 'Санкт-Петербург' }],
          page: Number(currentPage),
          page_size: 100,
          total: 2,
        })
      }),
      http.get('*/api/v1/vacancies', ({ request }) => {
        const url = new URL(request.url)
        requestedUrls.push(request.url)
        const page = Number(url.searchParams.get('page') || 1)
        return HttpResponse.json({
          data:
            page === 1
              ? [
                  {
                    id: 'vacancy-1',
                    source: 'hh',
                    source_name: 'HeadHunter',
                    source_url: 'https://hh.ru/vacancy/123',
                    external_id: '123',
                    title: url.searchParams.get('q') || 'Senior Go Developer',
                    published_at: '2026-08-25T10:00:00Z',
                    is_active: true,
                    skills: ['Go'],
                    region_id: 'region-2',
                  },
                ]
              : [
                  {
                    id: 'vacancy-1',
                    source: 'hh',
                    external_id: '123',
                    title: 'Duplicate',
                    published_at: '2026-08-25T10:00:00Z',
                    is_active: true,
                  },
                  {
                    id: 'vacancy-2',
                    source: 'hh',
                    external_id: '124',
                    title: 'Go Platform Engineer',
                    published_at: '2026-08-25T10:00:00Z',
                    is_active: true,
                  },
                ],
          page,
          page_size: 2,
          total: 3,
        })
      }),
    )

    renderPage(<VacanciesPage />, '/vacancies')
    expect((await screen.findAllByText('Senior Go Developer')).length).toBeGreaterThan(0)
    expect((await screen.findAllByText('Санкт-Петербург')).length).toBeGreaterThan(0)
    expect(screen.queryByText('region-2')).not.toBeInTheDocument()
    expect(requestedRegionPages).toEqual(['1', '2'])
    expect(requestedRegionUrls.every((url) => url.includes('from=2000-01-01'))).toBe(true)
    const loadedStatus = screen.getByText('Загружено 1 из 3')
    expect(loadedStatus).toBeInTheDocument()
    expect(getComputedStyle(loadedStatus).marginBottom).toBe('8px')
    const sourceLinks = screen.getAllByRole('link', {
      name: 'Открыть вакансию «Senior Go Developer» на hh.ru в новой вкладке',
    })
    expect(sourceLinks).toHaveLength(2)
    sourceLinks.forEach((link) => {
      expect(link).toHaveAttribute('href', 'https://hh.ru/vacancy/123')
      expect(link).toHaveAttribute('target', '_blank')
      expect(link).toHaveAttribute('rel', 'noopener noreferrer nofollow')
      expect(link.tagName).toBe('A')
      expect(link.querySelector('a, button, input, select, textarea')).toBeNull()
    })
    expect(requestedUrls.every((url) => url.includes('currency=RUB'))).toBe(true)

    intersectSentinel()
    expect((await screen.findAllByText('Go Platform Engineer')).length).toBeGreaterThan(0)
    expect(screen.queryByText('Duplicate')).not.toBeInTheDocument()
    expect(screen.getByText('Загружено 2 из 3')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Загрузить ещё' })).toBeInTheDocument()
    expect(requestedUrls.some((url) => url.includes('page=2'))).toBe(true)
    expect(
      requestedUrls
        .filter((url) => !url.includes('page_size=100'))
        .every((url) => url.includes('page_size=20')),
    ).toBe(true)
    expect(requestedUrls.every((url) => url.includes('only_active=true'))).toBe(true)
  })

  it('renders missing source URLs as non-clickable rows and cards', async () => {
    server.use(
      http.get('*/api/v1/vacancies', () =>
        HttpResponse.json({
          data: [
            {
              id: 'without-source-url',
              source: 'hh',
              source_name: 'HeadHunter',
              source_url: null,
              title: 'Vacancy without source URL',
              is_active: true,
            },
          ],
          page: 1,
          page_size: 20,
          total: 1,
        }),
      ),
    )

    renderPage(<VacanciesPage pollIntervalMs={0} />)

    const titles = await screen.findAllByText('Vacancy without source URL')
    expect(titles).toHaveLength(2)
    expect(screen.queryByRole('link', { name: /Vacancy without source URL/ })).not.toBeInTheDocument()
    expect(titles[0].closest('tr')?.querySelector('a')).toBeNull()
    expect(titles[1].closest('.MuiCard-root')?.querySelector('a')).toBeNull()
  })

  it('resets accumulated pages when a filter changes', async () => {
    vi.stubGlobal('IntersectionObserver', IntersectionObserverMock)
    const requestedQueries: string[] = []
    server.use(
      http.get('*/api/v1/vacancies', ({ request }) => {
        const url = new URL(request.url)
        const query = url.searchParams.get('q') || ''
        if (url.searchParams.get('page_size') !== '100') requestedQueries.push(query)
        return HttpResponse.json({
          data: [
            {
              id: query ? 'filtered' : 'initial',
              title: query || 'Initial vacancy',
              published_at: '2026-08-25T10:00:00Z',
              is_active: true,
            },
          ],
          page: 1,
          page_size: 20,
          total: 1,
        })
      }),
    )
    renderPage(<VacanciesPage />, '/vacancies?page=9')
    expect((await screen.findAllByText('Initial vacancy')).length).toBeGreaterThan(0)
    fireEvent.change(screen.getByLabelText('Поиск по названию'), {
      target: { value: 'Data engineer' },
    })
    const submitButton = screen.getByRole('button', { name: 'Найти' })
    expect(submitButton).toHaveStyle({ flexShrink: '0', whiteSpace: 'nowrap' })
    expect(submitButton.querySelector('[data-testid="SearchRoundedIcon"]')).toBeInTheDocument()
    fireEvent.click(submitButton)
    expect((await screen.findAllByText('Data engineer')).length).toBeGreaterThan(0)
    expect(screen.queryByText('Initial vacancy')).not.toBeInTheDocument()
    expect(requestedQueries).toEqual(['', 'Data engineer'])
  })

  it('serializes combined URL filters and clears them', async () => {
    const requestedUrls: string[] = []
    server.use(
      http.get('*/api/v1/vacancies', ({ request }) => {
        requestedUrls.push(request.url)
        return HttpResponse.json({ data: [], page: 1, page_size: 20, total: 0 })
      }),
    )
    const role = '10000000-0000-4000-8000-000000000001'
    const region = '20000000-0000-4000-8000-000000000001'
    const skill = '30000000-0000-4000-8000-000000000001'
    renderPage(
      <VacanciesPage />,
      `/vacancies?role_id=${role}&region_id=${region}&skill_id=${skill}&salary_min=100000&salary_max=300000`,
    )
    expect(await screen.findByText('Вакансии не найдены')).toBeInTheDocument()
    expect(screen.getByText('Активных фильтров: 5')).toBeInTheDocument()
    expect(requestedUrls[0]).toContain(`role_id=${role}`)
    expect(requestedUrls[0]).toContain(`region_id=${region}`)
    expect(requestedUrls[0]).toContain(`skill_id=${skill}`)
    expect(requestedUrls[0]).toContain('salary_min=100000')
    fireEvent.click(screen.getByRole('button', { name: 'Сбросить все' }))
    await waitFor(() => expect(requestedUrls.length).toBeGreaterThan(1))
    expect(requestedUrls.at(-1)).not.toContain('salary_min')
  })

  it('retries a failed next page from the fallback button', async () => {
    vi.stubGlobal('IntersectionObserver', IntersectionObserverMock)
    let pageTwoAttempts = 0
    server.use(
      http.get('*/api/v1/vacancies', ({ request }) => {
        const page = Number(new URL(request.url).searchParams.get('page') || 1)
        if (page === 2 && ++pageTwoAttempts === 1) {
          return new HttpResponse(null, { status: 503 })
        }
        return HttpResponse.json({
          data: [{ id: `vacancy-${page}`, title: `Vacancy ${page}`, is_active: true }],
          page,
          page_size: 1,
          total: 2,
        })
      }),
    )
    renderPage(<VacanciesPage />)
    expect((await screen.findAllByText('Vacancy 1')).length).toBeGreaterThan(0)
    intersectSentinel()
    expect(await screen.findByText('Не удалось загрузить следующую страницу.')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Повторить' }))
    expect((await screen.findAllByText('Vacancy 2')).length).toBeGreaterThan(0)
    expect(pageTwoAttempts).toBe(2)
  })

  it('offers a load-more fallback without IntersectionObserver', async () => {
    vi.stubGlobal('IntersectionObserver', undefined)
    server.use(
      http.get('*/api/v1/vacancies', ({ request }) => {
        const page = Number(new URL(request.url).searchParams.get('page') || 1)
        return HttpResponse.json({
          data: [{ id: `fallback-${page}`, title: `Fallback ${page}`, is_active: true }],
          page,
          page_size: 1,
          total: 2,
        })
      }),
    )
    renderPage(<VacanciesPage />)
    expect((await screen.findAllByText('Fallback 1')).length).toBeGreaterThan(0)
    fireEvent.click(screen.getByRole('button', { name: 'Загрузить ещё' }))
    expect((await screen.findAllByText('Fallback 2')).length).toBeGreaterThan(0)
  })

  it('keeps vacancies visible with a neutral label while the dictionary loads', async () => {
    server.use(
      http.get('*/api/v1/regions', async () => {
        await delay(200)
        return HttpResponse.json({
          data: [{ region_id: 'region-1', title: 'Москва' }],
          page: 1,
          page_size: 100,
          total: 1,
        })
      }),
      http.get('*/api/v1/vacancies', () =>
        HttpResponse.json({
          data: [
            {
              id: 'vacancy-1',
              title: 'Backend Developer',
              region_id: 'region-1',
              published_at: '2026-08-25T10:00:00Z',
              is_active: true,
            },
          ],
          page: 1,
          page_size: 20,
          total: 1,
        }),
      ),
    )

    renderPage(<VacanciesPage />, '/vacancies')

    expect((await screen.findAllByText('Backend Developer')).length).toBeGreaterThan(0)
    expect(screen.getAllByText('Регион не указан').length).toBeGreaterThan(0)
    expect(screen.queryByText('region-1')).not.toBeInTheDocument()
    expect((await screen.findAllByText('Москва')).length).toBeGreaterThan(0)
  })

  it('keeps vacancies visible when the region dictionary fails', async () => {
    server.use(
      http.get('*/api/v1/regions', () => new HttpResponse(null, { status: 503 })),
      http.get('*/api/v1/vacancies', () =>
        HttpResponse.json({
          data: [
            {
              id: 'vacancy-2',
              title: 'Data Engineer',
              region_id: 'unmapped-region',
              published_at: '2026-08-25T10:00:00Z',
              is_active: true,
            },
          ],
          page: 1,
          page_size: 20,
          total: 1,
        }),
      ),
    )

    renderPage(<VacanciesPage />, '/vacancies')

    expect((await screen.findAllByText('Data Engineer')).length).toBeGreaterThan(0)
    expect(screen.getAllByText('Регион не указан').length).toBeGreaterThan(0)
    expect(screen.queryByText('unmapped-region')).not.toBeInTheDocument()
  })

  it('uses a baseline, merges one new filtered vacancy, ignores duplicates and removes highlighting', async () => {
    let freshnessCalls = 0
    let releaseNewVacancy = false
    const freshnessUrls: string[] = []
    server.use(
      http.get('*/api/v1/vacancies', ({ request }) => {
        const url = new URL(request.url)
        const isFreshness = url.searchParams.get('page_size') === '100'
        const initial = {
          id: 'baseline-id',
          title: 'Synthetic baseline vacancy',
          published_at: '2026-08-25T10:00:00Z',
          is_active: true,
        }
        if (isFreshness) {
          freshnessCalls += 1
          freshnessUrls.push(request.url)
        }
        const withNew = isFreshness && releaseNewVacancy && freshnessCalls >= 2
        return HttpResponse.json({
          data: withNew
            ? [
                {
                  id: 'new-id',
                  title: 'Synthetic new vacancy',
                  published_at: '2026-08-26T10:00:00Z',
                  is_active: true,
                },
                initial,
              ]
            : [initial],
          page: 1,
          page_size: isFreshness ? 100 : 20,
          total: withNew ? 2 : 1,
        })
      }),
    )

    renderPage(
      <VacanciesPage pollIntervalMs={30} highlightDurationMs={300} />,
      '/vacancies?q=Synthetic&only_active=false',
    )
    expect((await screen.findAllByText('Synthetic baseline vacancy')).length).toBe(2)
    await waitFor(() => expect(freshnessCalls).toBeGreaterThanOrEqual(1))
    expect(document.querySelectorAll('[data-new-vacancy="true"]')).toHaveLength(0)

    releaseNewVacancy = true
    expect(await screen.findByText('Добавлено 1 новых вакансий')).toBeInTheDocument()
    expect(screen.getAllByText('Synthetic new vacancy')).toHaveLength(2)
    expect(document.querySelectorAll('[data-new-vacancy="true"]')).toHaveLength(2)
    expect(screen.getByText('Загружено 2 из 2')).toBeInTheDocument()
    expect(freshnessUrls.every((url) => url.includes('q=Synthetic'))).toBe(true)
    expect(freshnessUrls.every((url) => url.includes('only_active=false'))).toBe(true)

    await waitFor(
      () => expect(document.querySelectorAll('[data-new-vacancy="true"]')).toHaveLength(0),
      { timeout: 1000 },
    )
    expect(screen.getAllByText('Synthetic new vacancy')).toHaveLength(2)
  })

  it('respects reduced motion for a newly discovered vacancy', async () => {
    vi.stubGlobal('matchMedia', (query: string) => ({
      matches: query === '(prefers-reduced-motion: reduce)',
      media: query,
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    }))
    let freshnessCalls = 0
    server.use(
      http.get('*/api/v1/vacancies', ({ request }) => {
        const isFreshness = new URL(request.url).searchParams.get('page_size') === '100'
        if (isFreshness) freshnessCalls += 1
        const data =
          isFreshness && freshnessCalls >= 2
            ? [
                { id: 'reduced-new', title: 'Reduced motion vacancy', is_active: true },
                { id: 'reduced-base', title: 'Reduced baseline', is_active: true },
              ]
            : [{ id: 'reduced-base', title: 'Reduced baseline', is_active: true }]
        return HttpResponse.json({
          data,
          page: 1,
          page_size: isFreshness ? 100 : 20,
          total: data.length,
        })
      }),
    )

    renderPage(<VacanciesPage pollIntervalMs={30} highlightDurationMs={200} />)
    expect((await screen.findAllByText('Reduced baseline')).length).toBe(2)
    expect(await screen.findByText('Добавлено 1 новых вакансий')).toBeInTheDocument()
    const highlighted = document.querySelectorAll('[data-new-vacancy="true"]')
    expect(highlighted).toHaveLength(2)
    highlighted.forEach((element) =>
      expect(element).toHaveAttribute('data-reduced-motion', 'true'),
    )
  })

  it('can disable polling completely', async () => {
    let freshnessCalls = 0
    server.use(
      http.get('*/api/v1/vacancies', ({ request }) => {
        if (new URL(request.url).searchParams.get('page_size') === '100') freshnessCalls += 1
        return HttpResponse.json({
          data: [{ id: 'disabled', title: 'Polling disabled', is_active: true }],
          page: 1,
          page_size: 20,
          total: 1,
        })
      }),
    )

    renderPage(<VacanciesPage pollIntervalMs={0} />)
    expect((await screen.findAllByText('Polling disabled')).length).toBe(2)
    await new Promise((resolve) => window.setTimeout(resolve, 80))
    expect(freshnessCalls).toBe(0)
  })

  it('keeps the current list when repeated freshness requests fail', async () => {
    let freshnessCalls = 0
    server.use(
      http.get('*/api/v1/vacancies', ({ request }) => {
        const isFreshness = new URL(request.url).searchParams.get('page_size') === '100'
        if (isFreshness && ++freshnessCalls > 1) {
          return new HttpResponse(null, { status: 503 })
        }
        return HttpResponse.json({
          data: [{ id: 'preserved', title: 'Preserved vacancy', is_active: true }],
          page: 1,
          page_size: isFreshness ? 100 : 20,
          total: 1,
        })
      }),
    )

    renderPage(<VacanciesPage pollIntervalMs={30} />)
    expect((await screen.findAllByText('Preserved vacancy')).length).toBe(2)
    expect(
      await screen.findByText('Автообновление временно недоступно; список сохранён.', {}, {
        timeout: 2500,
      }),
    ).toBeInTheDocument()
    expect(screen.getAllByText('Preserved vacancy')).toHaveLength(2)
    expect(screen.queryByText(/Не удалось связаться с API/)).not.toBeInTheDocument()
  })
})

describe('vacancy page helpers', () => {
  const vacancy = (id: string) => ({ id, title: id, is_active: true })

  it('deduplicates vacancy IDs', () => {
    expect(
      dedupeVacancies([
        { data: [vacancy('1')], page: 1, page_size: 1, total: 2 },
        { data: [vacancy('1'), vacancy('2')], page: 2, page_size: 1, total: 2 },
      ]).map((item) => item.id),
    ).toEqual(['1', '2'])
  })

  it('stops on empty and final pages while skipping shifted duplicates', () => {
    const first = { data: [vacancy('1')], page: 1, page_size: 1, total: 3 }
    expect(getNextVacancyPageParam(first, [first])).toBe(2)
    const duplicate = { data: [vacancy('1')], page: 2, page_size: 1, total: 3 }
    expect(getNextVacancyPageParam(duplicate, [first, duplicate])).toBe(3)
    const empty = { data: [], page: 2, page_size: 1, total: 3 }
    expect(getNextVacancyPageParam(empty, [first, empty])).toBeUndefined()
    const final = { data: [vacancy('2')], page: 2, page_size: 1, total: 2 }
    expect(getNextVacancyPageParam(final, [first, final])).toBeUndefined()
  })

  it('merges fresh rows in stable newest order and updates total', () => {
    const old = {
      data: [
        { ...vacancy('old'), published_at: '2026-08-25T00:00:00Z' },
        { ...vacancy('older'), published_at: '2026-08-24T00:00:00Z' },
      ],
      page: 1,
      page_size: 2,
      total: 2,
    }
    const freshness = {
      data: [
        { ...vacancy('new'), published_at: '2026-08-26T00:00:00Z' },
        { ...vacancy('old'), title: 'updated without new highlight', published_at: '2026-08-25T00:00:00Z' },
      ],
      page: 1,
      page_size: 100,
      total: 3,
    }

    const merged = mergeFreshVacancies([old], freshness, new Set(['new']))
    expect(merged[0].data.map((item) => item.id)).toEqual(['new', 'old', 'older'])
    expect(merged[0].data[1].title).toBe('updated without new highlight')
    expect(merged[0].total).toBe(3)
    expect(getNextVacancyPageParam(merged[0], merged)).toBeUndefined()
  })

  it('parses polling defaults, minimum and disabled mode', () => {
    expect(parseVacancyPollInterval(undefined)).toBe(30_000)
    expect(parseVacancyPollInterval('1000')).toBe(10_000)
    expect(parseVacancyPollInterval('0')).toBe(0)
    expect(parseVacancyPollInterval('invalid')).toBe(30_000)
  })
})

describe('getRegionLabel', () => {
  it('maps an ID to its region name', () => {
    expect(getRegionLabel('region-1', new Map([['region-1', 'Москва']]))).toBe('Москва')
  })

  it('uses a neutral fallback for missing and unknown IDs', () => {
    const regionNames = new Map([['region-1', 'Москва']])

    expect(getRegionLabel(null, regionNames)).toBe('Регион не указан')
    expect(getRegionLabel('unknown-region', regionNames)).toBe('Регион не указан')
  })
})
