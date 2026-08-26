import { screen } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { renderPage } from '../test/render'
import { server } from '../test/server'
import { MarketPage } from './MarketPage'

describe('MarketPage', () => {
  it('derives the selectable year from coverage and renders snapshot semantics', async () => {
    renderPage(<MarketPage />)
    expect(await screen.findByText('Покрытие данных')).toBeInTheDocument()
    expect(screen.getByText('2026')).toBeInTheDocument()
    expect(screen.queryByText('2024')).not.toBeInTheDocument()
    expect(screen.getByText('vacancy_demand_v1')).toBeInTheDocument()
    expect(await screen.findByText('Активные')).toBeInTheDocument()
    expect(screen.getByText('Опубликованные')).toBeInTheDocument()
  })

  it('shows an honest collecting state without requesting a fabricated series', async () => {
    let demandCalls = 0
    server.use(
      http.get('*/api/v1/trends/coverage', () =>
        HttpResponse.json({
          status: 'collecting',
          source: 'hh',
          available_years: [],
          first_observation: null,
          last_observation: null,
          publication_from: null,
          publication_to: null,
          complete_daily_count: 0,
          complete_weekly_count: 0,
          latest_complete_cycle: null,
          regions: [],
        }),
      ),
      http.get('*/api/v1/trends/demand', () => {
        demandCalls += 1
        return HttpResponse.json({ status: 'no_complete_snapshots', points: [] })
      }),
    )
    renderPage(<MarketPage />)
    expect(await screen.findByText(/Полный all-IT цикл ещё не завершён/)).toBeInTheDocument()
    expect(screen.getByText(/История за прошлые годы не реконструируется/)).toBeInTheDocument()
    expect(demandCalls).toBe(0)
  })
})
