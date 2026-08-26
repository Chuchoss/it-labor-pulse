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
    let version = 1
    let savedHardCriteria: Record<string, unknown> = { role: 'backend' }
    let requestBody: Record<string, unknown> | undefined
    server.use(
      http.get('*/api/v1/assistant/preferences', () => HttpResponse.json({
        version, note: savedNote, hard_criteria: savedHardCriteria, soft_criteria: {}, weights: {},
        active_from: '2026-08-26T12:00:00Z',
      })),
      http.patch('*/api/v1/assistant/preferences', async ({ request }) => {
        const body = await request.json() as { note: string; hard_criteria: Record<string, unknown> }
        requestBody = body
        savedNote = body.note
        savedHardCriteria = body.hard_criteria
        version = 2
        return HttpResponse.json({
          version, note: savedNote, hard_criteria: savedHardCriteria, soft_criteria: {}, weights: {},
          active_from: '2026-08-26T12:01:00Z',
        })
      }),
      http.get('*/api/v1/assistant/preferences/list', () => HttpResponse.json([])),
      http.get('*/api/v1/assistant/status', () => HttpResponse.json({
        ai_configured: false, state: 'disabled', processed: 0, eligible: 0, matched: 0,
        ai_calls: 0, skipped: 0, pending_candidates: false,
      })),
      http.get('*/api/v1/assistant/automation', () => HttpResponse.json({
        ai_enabled: false, telegram_enabled: false, max_ai_calls_per_hour: 20,
      })),
      http.get('*/api/v1/assistant/matches', () => HttpResponse.json([])),
      http.get('*/api/v1/assistant/telegram', () => HttpResponse.json({ configured: false, linked: false, opted_in: false })),
    )

    const user = userEvent.setup()
    renderPage(<AssistantPage />)
    const input = await screen.findByDisplayValue('saved synthetic profile')
    expect(await screen.findByText('Разработчик')).toBeInTheDocument()
    expect(screen.getByText(/Устаревший критерий роли сопоставлен/)).toBeInTheDocument()
    await user.clear(input)
    await user.type(input, 'updated synthetic profile')
    await user.click(screen.getByRole('button', { name: 'Сохранить критерии' }))

    expect(await screen.findByText(/Критерии сохранены/)).toBeInTheDocument()
    expect(screen.getAllByText(/Версия 2/)).toHaveLength(2)
    if (!requestBody) throw new Error('save request was not captured')
    expect((requestBody.hard_criteria as Record<string, unknown>).role).toBeUndefined()
    expect((requestBody.hard_criteria as Record<string, unknown>).approved_roles).toEqual(['96'])
    expect(savedHardCriteria).toEqual({ approved_roles: ['96'] })
  }, 10_000)

  it('shows an error and does not claim success on non-2xx save', async () => {
    server.use(
      http.get('*/api/v1/assistant/preferences', () => HttpResponse.json({
        note: 'synthetic profile', hard_criteria: {}, soft_criteria: {}, weights: {},
      })),
      http.patch('*/api/v1/assistant/preferences', () => HttpResponse.json(
        { error: { code: 'VALIDATION_ERROR', message: 'Invalid preferences', request_id: 'test' } },
        { status: 400 },
      )),
      http.get('*/api/v1/assistant/preferences/list', () => HttpResponse.json([])),
      http.get('*/api/v1/assistant/status', () => HttpResponse.json({
        ai_configured: false, state: 'disabled', processed: 0, eligible: 0, matched: 0,
        ai_calls: 0, skipped: 0, pending_candidates: false,
      })),
      http.get('*/api/v1/assistant/automation', () => HttpResponse.json({
        ai_enabled: false, telegram_enabled: false, max_ai_calls_per_hour: 20,
      })),
      http.get('*/api/v1/assistant/matches', () => HttpResponse.json([])),
      http.get('*/api/v1/assistant/telegram', () => HttpResponse.json({ configured: false, linked: false, opted_in: false })),
    )

    const user = userEvent.setup()
    renderPage(<AssistantPage />)
    await screen.findByDisplayValue('synthetic profile')
    await user.click(screen.getByRole('button', { name: 'Сохранить критерии' }))

    expect(await screen.findByText(/Не удалось сохранить критерии/)).toBeInTheDocument()
    expect(screen.getByText(/VALIDATION_ERROR, request_id: test/)).toBeInTheDocument()
    expect(screen.queryByText(/Критерии сохранены/)).not.toBeInTheDocument()
  })
})
