import type {
  ApiErrorBody,
  DashboardSummary,
  DemandTrends,
  SalaryTrends,
  TopSkills,
  VacancyPage,
} from './types'

const API_BASE_URL = (import.meta.env.VITE_API_BASE_URL || '/api/v1').replace(/\/$/, '')
const REQUEST_TIMEOUT_MS = 10_000

export class ApiError extends Error {
  readonly status?: number
  readonly code?: string
  readonly requestId?: string

  constructor(
    message: string,
    status?: number,
    code?: string,
    requestId?: string,
  ) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.requestId = requestId
  }
}

async function get<T>(
  path: string,
  params: Record<string, string | number | boolean | undefined>,
  signal?: AbortSignal,
): Promise<T> {
  const url = new URL(`${API_BASE_URL}${path}`, window.location.origin)
  Object.entries(params).forEach(([key, value]) => {
    if (value !== undefined && value !== '') url.searchParams.set(key, String(value))
  })

  const timeoutSignal = AbortSignal.timeout(REQUEST_TIMEOUT_MS)
  const requestSignal = signal ? AbortSignal.any([signal, timeoutSignal]) : timeoutSignal

  let response: Response
  try {
    response = await fetch(url, {
      headers: { Accept: 'application/json' },
      signal: requestSignal,
    })
  } catch (error) {
    if (error instanceof DOMException && error.name === 'AbortError') throw error
    throw new ApiError('Не удалось связаться с API. Проверьте, запущен ли BFF.')
  }

  if (!response.ok) {
    const body = (await response.json().catch(() => ({}))) as ApiErrorBody
    throw new ApiError(
      body.error?.message || `API вернул ошибку ${response.status}`,
      response.status,
      body.error?.code,
      body.error?.request_id || response.headers.get('X-Request-ID') || undefined,
    )
  }

  return response.json() as Promise<T>
}

export interface AnalyticsParams extends Record<string, string | undefined> {
  from: string
  to: string
  role_id?: string
  region_id?: string
}

export const api = {
  dashboard: (params: AnalyticsParams, signal?: AbortSignal) =>
    get<DashboardSummary>('/dashboard/summary', params, signal),
  salaryTrends: (params: AnalyticsParams, signal?: AbortSignal) =>
    get<SalaryTrends>('/trends/salaries', { ...params, grain: 'week' }, signal),
  demandTrends: (params: AnalyticsParams, signal?: AbortSignal) =>
    get<DemandTrends>('/trends/demand', { ...params, grain: 'week' }, signal),
  topSkills: (params: AnalyticsParams, signal?: AbortSignal) =>
    get<TopSkills>('/skills/top', { ...params, limit: 10 }, signal),
  vacancies: (
    params: {
      q?: string
      source?: string
      only_active?: boolean
      page: number
      page_size: number
    },
    signal?: AbortSignal,
  ) => get<VacancyPage>('/vacancies', params, signal),
}
