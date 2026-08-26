import { act, fireEvent, screen } from '@testing-library/react'
import { delay, http, HttpResponse } from 'msw'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { renderPage } from '../test/render'
import { server } from '../test/server'
import { getRegionLabel } from '../utils/regions'
import { VacanciesPage } from './VacanciesPage'
import { dedupeVacancies, getNextVacancyPageParam } from './vacancyPagination'

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
    expect(requestedRegionUrls.every((url) => url.includes('from=2026-08-25'))).toBe(true)
    expect(screen.getByText('Загружено 1 из 3')).toBeInTheDocument()

    intersectSentinel()
    expect((await screen.findAllByText('Go Platform Engineer')).length).toBeGreaterThan(0)
    expect(screen.queryByText('Duplicate')).not.toBeInTheDocument()
    expect(screen.getByText('Загружено 2 из 3')).toBeInTheDocument()
    expect(screen.getByText('Все доступные вакансии загружены')).toBeInTheDocument()
    expect(requestedUrls.some((url) => url.includes('page=2'))).toBe(true)
    expect(requestedUrls.every((url) => url.includes('page_size=20'))).toBe(true)
    expect(requestedUrls.every((url) => url.includes('only_active=true'))).toBe(true)
  })

  it('resets accumulated pages when a filter changes', async () => {
    vi.stubGlobal('IntersectionObserver', IntersectionObserverMock)
    const requestedQueries: string[] = []
    server.use(
      http.get('*/api/v1/vacancies', ({ request }) => {
        const url = new URL(request.url)
        const query = url.searchParams.get('q') || ''
        requestedQueries.push(query)
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

  it('stops on empty, duplicate-only, and final pages', () => {
    const first = { data: [vacancy('1')], page: 1, page_size: 1, total: 3 }
    expect(getNextVacancyPageParam(first, [first])).toBe(2)
    const duplicate = { data: [vacancy('1')], page: 2, page_size: 1, total: 3 }
    expect(getNextVacancyPageParam(duplicate, [first, duplicate])).toBeUndefined()
    const empty = { data: [], page: 2, page_size: 1, total: 3 }
    expect(getNextVacancyPageParam(empty, [first, empty])).toBeUndefined()
    const final = { data: [vacancy('2')], page: 2, page_size: 1, total: 2 }
    expect(getNextVacancyPageParam(final, [first, final])).toBeUndefined()
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
