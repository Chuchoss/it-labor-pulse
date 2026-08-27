import { http, HttpResponse } from 'msw'
import { fireEvent, screen, waitFor } from '@testing-library/react'
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
      ai_calls: 0, ai_matches: 0, ai_failures: 0, ai_skipped: 0,
      skipped: 0, pending_candidates: false,
    })),
    http.get('*/api/v1/assistant/automation', () => HttpResponse.json({
      ai_enabled: false, telegram_enabled: false,
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
    let requestURL = ''
    let requestMethod = ''
    useSupportingAssistantHandlers()
    server.use(
      http.get('*/api/v1/assistant/preferences', () => HttpResponse.json({
        id: 'preference-1', version, note: savedNote, hard_criteria: savedHardCriteria,
        soft_criteria: {}, weights: {},
        active_from: '2026-08-26T12:00:00Z',
      })),
      http.patch('*/api/v1/assistant/preferences', async ({ request }) => {
        requestURL = request.url
        requestMethod = request.method
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
    expect(new URL(requestURL).pathname).toBe('/api/v1/assistant/preferences')
    expect(requestMethod).toBe('PATCH')
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

  it('keeps edits after a network failure and allows one-request retry', async () => {
    let attempts = 0
    let savedBody: Record<string, unknown> | undefined
    let releaseFailure = () => {}
    useSupportingAssistantHandlers()
    server.use(
      http.get('*/api/v1/assistant/preferences', () => HttpResponse.json({
        id: 'preference-1', version: 1, note: '', hard_criteria: { regions: ['Москва'] },
        soft_criteria: {}, weights: {},
      })),
      http.patch('*/api/v1/assistant/preferences', async ({ request }) => {
        attempts += 1
        if (attempts === 1) {
          await new Promise<void>((resolve) => { releaseFailure = resolve })
          return HttpResponse.error()
        }
        savedBody = await request.json() as Record<string, unknown>
        return HttpResponse.json({
          id: 'preference-2', version: 2, ...(savedBody as object),
        })
      }),
    )

    const user = userEvent.setup()
    renderPage(<AssistantPage />)
    await screen.findByText('Москва')
    await user.type(screen.getByRole('combobox', { name: 'Регионы' }), 'Казань{Enter}')
    const saveButton = screen.getByRole('button', { name: 'Сохранить критерии' })

    await user.click(saveButton)
    await waitFor(() => expect(attempts).toBe(1))
    expect(saveButton).toBeDisabled()
    fireEvent.click(saveButton)
    expect(attempts).toBe(1)
    releaseFailure()
    expect(await screen.findByText(
      'Не удалось сохранить критерии: API недоступен. Проверьте, запущен ли BFF.',
    )).toBeInTheDocument()
    expect(screen.getByText('Казань')).toBeInTheDocument()
    expect(screen.queryByText(/Failed to fetch/)).not.toBeInTheDocument()
    expect(screen.queryByRole('status')).not.toBeInTheDocument()
    expect(attempts).toBe(1)

    await user.click(saveButton)
    expect(await screen.findByRole('status')).toHaveTextContent('Сохранено')
    expect(attempts).toBe(2)
    if (!savedBody) throw new Error('retry request was not captured')
    expect((savedBody.hard_criteria as Record<string, unknown>).regions).toEqual(['Москва', 'Казань'])
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

  it('shows immediate feedback, submits once, polls progress and stops when finished', async () => {
    let requests = 0
    let runAccepted = false
    let statusAfterRun = 0
    let releaseRequest = () => {}
    useSupportingAssistantHandlers()
    server.use(
      http.get('*/api/v1/assistant/preferences', () => HttpResponse.json({
        id: 'preference-1', version: 1, note: '', hard_criteria: {},
        soft_criteria: {}, weights: {},
      })),
      http.get('*/api/v1/assistant/status', () => {
        if (!runAccepted) return HttpResponse.json({
          ai_configured: true, ai_status: 'completed', state: 'succeeded', processed: 25, total: 151,
          eligible: 25, matched: 3, ai_calls: 0, ai_succeeded: 0, ai_matches: 0, ai_failures: 0,
          ai_skipped: 25, skipped: 22, pending_candidates: false,
        })
        statusAfterRun += 1
        if (statusAfterRun === 1) return HttpResponse.json({
          ai_configured: true, ai_status: 'running', state: 'running', processed: 10, total: 22,
          eligible: 10, matched: 1, ai_calls: 4, ai_succeeded: 3, ai_matches: 1, ai_failures: 1,
          ai_skipped: 0, skipped: 9, pending_candidates: false, last_checked_at: '2026-08-27T01:00:00Z',
        })
        return HttpResponse.json({
          ai_configured: true, ai_status: 'partial', state: 'succeeded', processed: 22, total: 22,
          eligible: 22, matched: 2, ai_calls: 8, ai_succeeded: 7, ai_matches: 2, ai_failures: 1,
          ai_skipped: 14, skipped: 20, pending_candidates: false,
          finished_at: '2026-08-27T01:00:01Z', last_checked_at: '2026-08-27T01:00:01Z',
        })
      }),
      http.post('*/api/v1/assistant/analyze', async () => {
        requests += 1
        await new Promise<void>((resolve) => { releaseRequest = resolve })
        runAccepted = true
        return HttpResponse.json({ run_id: 'run-full', status: 'queued' }, { status: 202 })
      }),
    )

    const user = userEvent.setup()
    renderPage(<AssistantPage />)
    expect(await screen.findByText(/25 из 151/)).toBeInTheDocument()
    expect(screen.getByText(/Новые вакансии будут автоматически анализироваться AI по полному описанию/)).toBeInTheDocument()
    expect(screen.getByText(/каждая подходящая вакансия может создать платный AI-запрос/i)).toBeInTheDocument()
    expect(screen.queryByText(/20.*запрос|лимит 20/i)).not.toBeInTheDocument()
    expect(screen.getByText(/AI-анализ: не выполнялся · Совпадения: — · Ошибки: 0 · Пропущено AI: 25/)).toBeInTheDocument()
    const runButton = screen.getByRole('button', { name: 'Проверить текущие вакансии' })
    await user.click(runButton)
    expect(screen.queryByRole('dialog', { name: 'Подтверждение действия' })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Подготавливаем список вакансий…' })).toBeDisabled()
    expect(screen.getByText('Подготавливаем список вакансий…', { selector: 'p' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Подготавливаем список вакансий…' }))
    expect(requests).toBe(1)

    releaseRequest()
    expect(await screen.findByText(/10 из 22/)).toBeInTheDocument()
    expect(screen.getByText(/AI-вакансий отправлено: 4 .* Успешно: 3 · Финальных ошибок: 1/)).toBeInTheDocument()
    expect(await screen.findByText('Проверка завершена, но часть AI-запросов не удалась.')).toBeInTheDocument()
    expect(screen.getByText(/22 из 22/)).toBeInTheDocument()
    expect(requests).toBe(1)

    const callsAtCompletion = statusAfterRun
    await new Promise((resolve) => setTimeout(resolve, 150))
    expect(statusAfterRun).toBe(callsAtCompletion)
  }, 10_000)

  it('resumes polling an active run after page refresh', async () => {
    let statusCalls = 0
    useSupportingAssistantHandlers()
    server.use(
      http.get('*/api/v1/assistant/preferences', () => HttpResponse.json({
        id: 'preference-1', version: 1, note: '', hard_criteria: {},
        soft_criteria: {}, weights: {},
      })),
      http.get('*/api/v1/assistant/status', () => {
        statusCalls += 1
        const done = statusCalls > 1
        return HttpResponse.json({
          ai_configured: true, ai_status: done ? 'completed' : 'running',
          state: done ? 'succeeded' : 'running', processed: done ? 12 : 5, total: 12,
          eligible: done ? 12 : 5, matched: done ? 2 : 1, ai_calls: done ? 12 : 5,
          ai_succeeded: done ? 12 : 5, ai_matches: done ? 2 : 1, ai_failures: 0,
          ai_skipped: 0, skipped: done ? 10 : 4, pending_candidates: false,
        })
      }),
    )

    renderPage(<AssistantPage />)
    expect(await screen.findByText(/5 из 12/)).toBeInTheDocument()
    expect(await screen.findByText(/12 из 12/)).toBeInTheDocument()
    const callsAtCompletion = statusCalls
    await new Promise((resolve) => setTimeout(resolve, 150))
    expect(statusCalls).toBe(callsAtCompletion)
  })

  it('explains that saved criteria are required without sending a request', async () => {
    let requests = 0
    useSupportingAssistantHandlers()
    server.use(
      http.get('*/api/v1/assistant/preferences', () => HttpResponse.json({
        note: '', hard_criteria: {}, soft_criteria: {}, weights: {},
      })),
      http.post('*/api/v1/assistant/analyze', () => {
        requests += 1
        return HttpResponse.json({ run_id: 'unexpected', status: 'queued' }, { status: 202 })
      }),
    )

    const user = userEvent.setup()
    renderPage(<AssistantPage />)
    await screen.findByText('Критерии вакансии')
    await user.click(screen.getByRole('button', { name: 'Проверить текущие вакансии' }))

    expect(screen.getByText('Сначала сохраните критерии вакансии, затем запустите анализ.')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'К критериям' })).toBeInTheDocument()
    expect(requests).toBe(0)
  })

  it('shows an active run after refresh and never submits a duplicate', async () => {
    let requests = 0
    useSupportingAssistantHandlers()
    server.use(
      http.get('*/api/v1/assistant/preferences', () => HttpResponse.json({
        id: 'preference-1', version: 1, note: '', hard_criteria: {},
        soft_criteria: {}, weights: {},
      })),
      http.get('*/api/v1/assistant/status', () => HttpResponse.json({
        ai_configured: true, ai_status: 'running', state: 'running', processed: 3, total: 12,
        eligible: 3, matched: 1, ai_calls: 2, ai_succeeded: 2, ai_matches: 1, ai_failures: 0,
        ai_skipped: 0, skipped: 2, pending_candidates: false,
      })),
      http.post('*/api/v1/assistant/analyze', () => {
        requests += 1
        return HttpResponse.json({ run_id: 'unexpected', status: 'queued' }, { status: 202 })
      }),
    )

    const user = userEvent.setup()
    renderPage(<AssistantPage />)
    expect(await screen.findByText('Активная проверка восстановлена. Прогресс обновляется автоматически.')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Показать текущий анализ' }))

    expect(requests).toBe(0)
    expect(screen.getByText(/3 из 12/)).toBeInTheDocument()
  })

  it('shows an API rejection next to the run button', async () => {
    useSupportingAssistantHandlers()
    server.use(
      http.get('*/api/v1/assistant/preferences', () => HttpResponse.json({
        id: 'preference-1', version: 1, note: '', hard_criteria: {},
        soft_criteria: {}, weights: {},
      })),
      http.get('*/api/v1/assistant/status', () => HttpResponse.json({
        ai_configured: true, ai_status: 'completed', state: 'succeeded', processed: 12, total: 12,
        eligible: 12, matched: 1, ai_calls: 0, ai_succeeded: 0, ai_matches: 0, ai_failures: 0,
        ai_skipped: 12, skipped: 11, pending_candidates: false,
      })),
      http.post('*/api/v1/assistant/analyze', () => HttpResponse.json(
        { error: { code: 'CONFLICT', message: 'Анализ уже выполняется', request_id: 'safe-request' } },
        { status: 409 },
      )),
    )

    const user = userEvent.setup()
    renderPage(<AssistantPage />)
    await screen.findByText(/12 из 12/)
    await user.click(screen.getByRole('button', { name: 'Проверить текущие вакансии' }))

    expect(await screen.findByText('Не удалось запустить анализ: Анализ уже выполняется')).toBeInTheDocument()
  })

  it('labels a historical deterministic-only run without false AI matches', async () => {
    useSupportingAssistantHandlers()
    server.use(
      http.get('*/api/v1/assistant/preferences', () => HttpResponse.json({
        id: 'preference-6', version: 6, note: '', hard_criteria: {}, soft_criteria: {}, weights: {},
      })),
      http.get('*/api/v1/assistant/status', () => HttpResponse.json({
        ai_configured: false, state: 'succeeded', ai_status: 'skipped',
        ai_skip_reason: 'run_predates_ai', processed: 1973, total: 1973,
        eligible: 1973, matched: 0, ai_eligible: 0, ai_calls: 0, ai_succeeded: 0,
        ai_matches: 0, ai_failures: 0, ai_skipped: 0, skipped: 1973,
        pending_candidates: false, finished_at: '2026-08-26T23:16:12Z',
      })),
    )

    renderPage(<AssistantPage />)

    expect(await screen.findByText(/1973 из 1973/)).toBeInTheDocument()
    expect(screen.getByText('AI-анализ: не выполнялся · Совпадения: — · Ошибки: 0 · Пропущено AI: 0')).toBeInTheDocument()
    expect(screen.getByText('AI-анализ не запускался: запуск создан до включения AI.')).toBeInTheDocument()
    expect(screen.getByText('Проверка по критериям завершена.')).toBeInTheDocument()
    expect(screen.queryByText(/AI-совпадения: 0/)).not.toBeInTheDocument()
  })
})
