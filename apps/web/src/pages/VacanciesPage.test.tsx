import { fireEvent, screen, waitFor } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { renderPage } from '../test/render'
import { server } from '../test/server'
import { VacanciesPage } from './VacanciesPage'

describe('VacanciesPage', () => {
  it('renders vacancies and sends search and pagination parameters', async () => {
    const requestedUrls: string[] = []
    server.use(
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

    fireEvent.change(screen.getByLabelText('Поиск по названию'), {
      target: { value: 'Data engineer' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Найти' }))
    expect((await screen.findAllByText('Data engineer')).length).toBeGreaterThan(0)

    fireEvent.click(screen.getByTitle('Go to next page'))
    await waitFor(() => {
      expect(requestedUrls.some((url) => url.includes('q=Data+engineer'))).toBe(true)
      expect(requestedUrls.some((url) => url.includes('page=2'))).toBe(true)
      expect(requestedUrls.every((url) => url.includes('only_active=true'))).toBe(true)
    })
  })
})
