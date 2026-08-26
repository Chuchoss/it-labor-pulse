import { fireEvent, screen, waitFor } from '@testing-library/react'
import { delay, http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { renderPage } from '../test/render'
import { server } from '../test/server'
import { getRegionLabel } from '../utils/regions'
import { VacanciesPage } from './VacanciesPage'

describe('VacanciesPage', () => {
  it('renders region names from every dictionary page and sends vacancy parameters', async () => {
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
        return HttpResponse.json({
          data: [
            {
              id: '3fa85f64-5717-4562-b3fc-2c963f66afa6',
              source: 'hh',
              external_id: '123',
              title: url.searchParams.get('q') || 'Senior Go Developer',
              salary_from: 200000,
              salary_to: 300000,
              salary_currency: 'RUB',
              salary_gross: false,
              published_at: '2026-08-25T10:00:00Z',
              is_active: true,
              skills: ['Go'],
              region_id: 'region-2',
            },
          ],
          page: Number(url.searchParams.get('page') || 1),
          page_size: 20,
          total: 22,
        })
      }),
    )

    renderPage(<VacanciesPage />, '/vacancies')
    expect((await screen.findAllByText('Senior Go Developer')).length).toBeGreaterThan(0)
    expect((await screen.findAllByText('Санкт-Петербург')).length).toBeGreaterThan(0)
    expect(screen.queryByText('region-2')).not.toBeInTheDocument()
    expect(requestedRegionPages).toEqual(['1', '2'])
    expect(
      requestedRegionUrls.every(
        (url) => url.includes('from=2026-08-25') && url.includes('to=2026-08-25'),
      ),
    ).toBe(true)

    fireEvent.change(screen.getByLabelText('Поиск по названию'), {
      target: { value: 'Data engineer' },
    })
    const submitButton = screen.getByRole('button', { name: 'Найти' })
    expect(submitButton).toHaveStyle({ flexShrink: '0', whiteSpace: 'nowrap' })
    expect(submitButton.querySelector('[data-testid="SearchRoundedIcon"]')).toBeInTheDocument()
    fireEvent.click(submitButton)
    expect((await screen.findAllByText('Data engineer')).length).toBeGreaterThan(0)

    fireEvent.click(screen.getByTitle('Go to next page'))
    await waitFor(() => {
      expect(requestedUrls.some((url) => url.includes('q=Data+engineer'))).toBe(true)
      expect(requestedUrls.some((url) => url.includes('page=2'))).toBe(true)
      expect(requestedUrls.every((url) => url.includes('only_active=true'))).toBe(true)
    })
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
