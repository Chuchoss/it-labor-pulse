import { http, HttpResponse } from 'msw'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import { AssistantPage } from './AssistantPage'
import { server } from '../test/server'
import { renderPage } from '../test/render'

describe('AssistantPage', () => {
  it('loads saved criteria and displays the new version after saving', async () => {
    let savedNote = 'saved synthetic profile'
    server.use(
      http.get('*/api/v1/assistant/preferences', () => HttpResponse.json({
        version: 1, note: savedNote, hard_criteria: { role: 'backend' }, soft_criteria: {}, weights: {},
        active_from: '2026-08-26T12:00:00Z',
      })),
      http.patch('*/api/v1/assistant/preferences', async ({ request }) => {
        const body = await request.json() as { note: string }
        savedNote = body.note
        return HttpResponse.json({
          version: 2, note: savedNote, hard_criteria: { role: 'backend' }, soft_criteria: {}, weights: {},
          active_from: '2026-08-26T12:01:00Z',
        })
      }),
      http.get('*/api/v1/assistant/matches', () => HttpResponse.json([])),
      http.get('*/api/v1/assistant/telegram', () => HttpResponse.json({ configured: false, linked: false, opted_in: false })),
    )

    const user = userEvent.setup()
    renderPage(<AssistantPage />)
    const input = await screen.findByDisplayValue('saved synthetic profile')
    await user.clear(input)
    await user.type(input, 'updated synthetic profile')
    await user.click(screen.getByRole('button', { name: 'Сохранить критерии' }))

    expect(await screen.findByText(/Критерии сохранены/)).toBeInTheDocument()
    expect(screen.getByText(/Версия 2/)).toBeInTheDocument()
  })

  it('shows an error and does not claim success on non-2xx save', async () => {
    server.use(
      http.get('*/api/v1/assistant/preferences', () => HttpResponse.json({
        note: 'synthetic profile', hard_criteria: {}, soft_criteria: {}, weights: {},
      })),
      http.patch('*/api/v1/assistant/preferences', () => HttpResponse.json(
        { error: { code: 'VALIDATION_ERROR', message: 'Invalid preferences', request_id: 'test' } },
        { status: 400 },
      )),
      http.get('*/api/v1/assistant/matches', () => HttpResponse.json([])),
      http.get('*/api/v1/assistant/telegram', () => HttpResponse.json({ configured: false, linked: false, opted_in: false })),
    )

    const user = userEvent.setup()
    renderPage(<AssistantPage />)
    await screen.findByDisplayValue('synthetic profile')
    await user.click(screen.getByRole('button', { name: 'Сохранить критерии' }))

    expect(await screen.findByText(/Не удалось сохранить критерии/)).toBeInTheDocument()
    expect(screen.queryByText(/Критерии сохранены/)).not.toBeInTheDocument()
  })
})
