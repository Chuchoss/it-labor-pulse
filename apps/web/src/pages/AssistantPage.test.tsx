import { http, HttpResponse } from 'msw'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import { AssistantPage } from './AssistantPage'
import { server } from '../test/server'
import { renderPage } from '../test/render'

function useSupportingAssistantHandlers() {
  server.use(
    http.get('*/api/v1/assistant/preferences/list', () => HttpResponse.json([])),
    http.get('*/api/v1/assistant/status', () => HttpResponse.json({
      ai_configured: false, state: 'disabled', processed: 0, eligible: 0, matched: 0,
      ai_calls: 0, skipped: 0, pending_candidates: false,
    })),
    http.get('*/api/v1/assistant/automation', () => HttpResponse.json({
      ai_enabled: false, telegram_enabled: false, max_ai_calls_per_hour: 20,
    })),
    http.get('*/api/v1/assistant/matches', () => HttpResponse.json([])),
    http.get('*/api/v1/assistant/telegram', () => HttpResponse.json({
      configured: false, linked: false, opted_in: false,
    })),
  )
}

describe('AssistantPage', () => {
  it('loads, edits and saves structured criteria without technical content', async () => {
    let savedNote = ''
    let version = 1
    let savedHardCriteria: Record<string, unknown> = {
      role: 'backend',
      regions: ['Москва'],
      required_skills: ['React'],
      remote_only: false,
      min_salary_rub: 100000,
    }
    let requestBody: Record<string, unknown> | undefined
    useSupportingAssistantHandlers()
    server.use(
      http.get('*/api/v1/assistant/preferences', () => HttpResponse.json({
        id: 'preference-1', version, note: savedNote, hard_criteria: savedHardCriteria,
        soft_criteria: {}, weights: {},
        active_from: '2026-08-26T12:00:00Z',
      })),
      http.patch('*/api/v1/assistant/preferences', async ({ request }) => {
        const body = await request.json() as { note: string; hard_criteria: Record<string, unknown> }
        requestBody = body
        savedNote = body.note
        savedHardCriteria = body.hard_criteria
        version = 2
        return HttpResponse.json({
          id: 'preference-2', version, note: savedNote, hard_criteria: savedHardCriteria,
          soft_criteria: {}, weights: {},
          active_from: '2026-08-26T12:01:00Z',
        })
      }),
    )

    const user = userEvent.setup()
    renderPage(<AssistantPage />)
    expect(await screen.findByText('Критерии вакансии')).toBeInTheDocument()
    expect(await screen.findByText('Разработчик')).toBeInTheDocument()
    expect(screen.getByText('Москва')).toBeInTheDocument()
    expect(screen.getByText('React')).toBeInTheDocument()
    expect(screen.queryByText(/Устаревший критерий роли/)).not.toBeInTheDocument()
    expect(screen.queryByText(/Жёсткие критерии|JSON только|Мягкие критерии|matcher использует/)).not.toBeInTheDocument()
    expect(screen.queryByText(/role:|approved_roles|required_skills|min_salary_rub/)).not.toBeInTheDocument()

    const regionInput = screen.getByRole('combobox', { name: 'Регионы' })
    await user.type(regionInput, 'Санкт-Петербург{Enter}')
    const skillsInput = screen.getByRole('combobox', { name: 'Обязательные навыки' })
    await user.type(skillsInput, 'TypeScript{Enter}')
    const salaryInput = screen.getByRole('spinbutton', { name: 'Минимальная зарплата, ₽' })
    await user.clear(salaryInput)
    await user.type(salaryInput, '200000')
    await user.click(screen.getByRole('radio', { name: 'Удалённо' }))
    await user.click(screen.getByText('Дополнительные критерии'))
    await user.type(screen.getByRole('combobox', { name: 'Исключить навыки' }), 'PHP{Enter}')
    await user.click(screen.getByRole('button', { name: 'Сохранить критерии' }))

    expect(await screen.findByRole('status')).toHaveTextContent('Сохранено')
    if (!requestBody) throw new Error('save request was not captured')
    expect(requestBody.hard_criteria).toEqual({
      approved_roles: ['96'],
      regions: ['Москва', 'Санкт-Петербург'],
      required_skills: ['React', 'TypeScript'],
      excluded_skills: ['PHP'],
      remote_only: true,
      min_salary_rub: 200000,
    })
    expect((requestBody.hard_criteria as Record<string, unknown>).role).toBeUndefined()
    await user.click(screen.getByText('История настроек'))
    expect(await screen.findByText(/Версия 2/)).toBeInTheDocument()
  }, 10_000)

  it('shows an error and does not claim success on non-2xx save', async () => {
    useSupportingAssistantHandlers()
    server.use(
      http.get('*/api/v1/assistant/preferences', () => HttpResponse.json({
        note: '', hard_criteria: {}, soft_criteria: {}, weights: {},
      })),
      http.patch('*/api/v1/assistant/preferences', () => HttpResponse.json(
        { error: { code: 'VALIDATION_ERROR', message: 'Invalid preferences', request_id: 'test' } },
        { status: 400 },
      )),
    )

    const user = userEvent.setup()
    renderPage(<AssistantPage />)
    await screen.findByText('Критерии вакансии')
    await user.click(screen.getByRole('button', { name: 'Сохранить критерии' }))

    expect(await screen.findByText(/Не удалось сохранить критерии/)).toBeInTheDocument()
    expect(screen.getByText(/VALIDATION_ERROR, request_id: test/)).toBeInTheDocument()
    expect(screen.queryByRole('status')).not.toBeInTheDocument()
  })

  it('requires manual role selection for an unknown legacy role and validates salary', async () => {
    let requestBody: Record<string, unknown> | undefined
    useSupportingAssistantHandlers()
    server.use(
      http.get('*/api/v1/assistant/preferences', () => HttpResponse.json({
        note: '', hard_criteria: { role: 'wizard' }, soft_criteria: {}, weights: {},
      })),
      http.patch('*/api/v1/assistant/preferences', async ({ request }) => {
        requestBody = await request.json() as Record<string, unknown>
        return HttpResponse.json({
          version: 2, note: '', hard_criteria: { approved_roles: ['96'], remote_only: false },
          soft_criteria: {}, weights: {},
        })
      }),
    )

    const user = userEvent.setup()
    renderPage(<AssistantPage />)
    expect(await screen.findByText(/Не удалось распознать сохранённую роль/)).toBeInTheDocument()
    const saveButton = screen.getByRole('button', { name: 'Сохранить критерии' })
    expect(saveButton).toBeDisabled()

    await user.type(screen.getByRole('combobox', { name: 'Роли' }), 'Разработчик')
    await user.click(await screen.findByRole('option', { name: 'Разработчик' }))
    const salaryInput = screen.getByRole('spinbutton', { name: 'Минимальная зарплата, ₽' })
    await user.type(salaryInput, '-1')
    expect(screen.getByText('Введите число не меньше 0')).toBeInTheDocument()
    expect(saveButton).toBeDisabled()
    await user.clear(salaryInput)
    await user.click(saveButton)

    expect(await screen.findByRole('status')).toHaveTextContent('Сохранено')
    if (!requestBody) throw new Error('save request was not captured')
    const hardCriteria = requestBody.hard_criteria as Record<string, unknown>
    expect(hardCriteria.approved_roles).toEqual(['96'])
    expect(hardCriteria.role).toBeUndefined()
    expect(hardCriteria.min_salary_rub).toBeUndefined()
  })
})
