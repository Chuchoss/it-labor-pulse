import type {
  ApiErrorBody,
  DashboardSummary,
  CurrenciesResponse,
  DemandTrends,
  RegionPage,
  RankingMetric,
  RankingPage,
  RegionStat,
  RolePage,
  RoleStat,
  SalaryTrends,
  AssistantMatch,
  AssistantStatus,
  SkillStat,
  TopSkills,
  TrendsCoverage,
  VacancyPage,
} from './types'

const API_BASE_URL = (import.meta.env.VITE_API_BASE_URL || '/api/v1').replace(/\/$/, '')
const REQUEST_TIMEOUT_MS = 10_000
const ASSISTANT_DEV_SUBJECT = import.meta.env.VITE_ASSISTANT_DEV_SUBJECT || 'local-dev-user'

const assistantStates = new Set([
  'never_run', 'queued', 'running', 'paused', 'succeeded', 'failed', 'disabled', 'superseded',
])
const assistantAIStates = new Set([
  'not_run', 'pending', 'running', 'completed', 'partial', 'failed', 'skipped',
])
const assistantCounterFields = [
  'processed', 'total', 'eligible', 'matched', 'ai_calls', 'ai_eligible', 'ai_succeeded',
  'ai_matches', 'ai_reviews', 'ai_rejects', 'ai_failures', 'ai_skipped',
  'ai_http_attempts', 'ai_retries', 'ai_batches',
  'ai_prompt_tokens', 'ai_completion_tokens', 'ai_cached_tokens', 'ai_rate_limit', 'ai_timeouts',
  'ai_invalid_responses', 'ai_auth', 'ai_quota', 'ai_server', 'ai_network', 'ai_context_limit',
  'ai_content_filter', 'ai_invalid_request', 'skipped',
  'preference_version', 'current_preference_version', 'superseded_by_preference_version',
  'worker_active_batches', 'worker_concurrency',
] as const

function asRecord(value: unknown): Record<string, unknown> {
  return value !== null && typeof value === 'object' ? value as Record<string, unknown> : {}
}

function optionalString(value: unknown) {
  return typeof value === 'string' && value.length > 0 ? value : undefined
}

function optionalNumber(value: unknown) {
  return typeof value === 'number' && Number.isFinite(value) && value >= 0 ? value : undefined
}

function stringArray(value: unknown) {
  return Array.isArray(value) ? value.filter((item): item is string => typeof item === 'string') : []
}

export function normalizeAssistantStatus(value: unknown): AssistantStatus {
  const source = asRecord(value)
  const normalized: Record<string, unknown> = { ...source }
  normalized.state = typeof source.state === 'string' && assistantStates.has(source.state)
    ? source.state
    : 'unknown'
  normalized.ai_status = typeof source.ai_status === 'string' && assistantAIStates.has(source.ai_status)
    ? source.ai_status
    : 'unknown'
  normalized.ai_configured = typeof source.ai_configured === 'boolean' ? source.ai_configured : undefined
  normalized.pending_candidates = typeof source.pending_candidates === 'boolean'
    ? source.pending_candidates
    : undefined
  for (const field of assistantCounterFields) normalized[field] = optionalNumber(source[field])
  return normalized as unknown as AssistantStatus
}

export function normalizeAssistantMatches(value: unknown): AssistantMatch[] {
  if (!Array.isArray(value)) return []
  return value.map((item) => {
    const source = asRecord(item)
    const score = optionalNumber(source.score ?? source.Score)
    return {
      vacancy_id: optionalString(source.vacancy_id ?? source.VacancyID),
      title: optionalString(source.title ?? source.Title),
      source_url: optionalString(source.source_url ?? source.SourceURL) ?? null,
      decision: optionalString(source.decision ?? source.Decision) as AssistantMatch['decision'],
      score: score !== undefined && score <= 1 ? score : undefined,
      method: optionalString(source.method ?? source.Method) as AssistantMatch['method'],
      stage: optionalString(source.stage ?? source.Stage) as AssistantMatch['stage'],
      confidence: optionalString(source.confidence ?? source.Confidence) as AssistantMatch['confidence'],
      reasons: stringArray(source.reasons ?? source.Reasons),
      unknowns: stringArray(source.unknowns ?? source.Unknowns),
      conflicts: stringArray(source.conflicts ?? source.Conflicts),
      evidence_ids: stringArray(source.evidence_ids ?? source.Evidence),
    }
  })
}

function headersFor(path: string, headers: Record<string, string> = {}) {
  return path.startsWith('/assistant/')
    ? { ...headers, 'X-Dev-User': ASSISTANT_DEV_SUBJECT }
    : headers
}

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
      headers: headersFor(path, { Accept: 'application/json' }),
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

async function mutate<T>(path: string, method: string, body?: unknown, idempotencyKey?: string): Promise<T> {
  const url = new URL(`${API_BASE_URL}${path}`, window.location.origin)
  let response: Response
  try {
    response = await fetch(url, {
      method,
      headers: headersFor(path, {
        Accept: 'application/json',
        'Content-Type': 'application/json',
        ...(idempotencyKey ? { 'Idempotency-Key': idempotencyKey } : {}),
      }),
      body: body === undefined ? undefined : JSON.stringify(body),
    })
  } catch {
    throw new ApiError('API недоступен. Проверьте, запущен ли BFF.')
  }
  if (!response.ok) {
    const error = (await response.json().catch(() => ({}))) as ApiErrorBody
    throw new ApiError(
      error.error?.message || `API вернул ошибку ${response.status}`,
      response.status,
      error.error?.code,
      error.error?.request_id || response.headers.get('X-Request-ID') || undefined,
    )
  }
  return (response.status === 204 ? undefined : response.json()) as Promise<T>
}

export interface AnalyticsParams extends Record<string, string | undefined> {
  from: string
  to: string
  role_id?: string
  region_id?: string
  currency?: string
}

export interface MarketParams extends Record<string, string | undefined> {
  from: string
  to: string
  role_group?: string
  region_id?: string
  grain?: 'day' | 'week'
  currency?: string
}

const DICTIONARY_PAGE_SIZE = 100

async function getAllRegions(
  params: Pick<AnalyticsParams, 'from' | 'to'>,
  signal?: AbortSignal,
): Promise<RegionStat[]> {
  const regions: RegionStat[] = []
  let page = 1

  while (true) {
    const response = await get<RegionPage>(
      '/regions',
      { ...params, page, page_size: DICTIONARY_PAGE_SIZE },
      signal,
    )
    regions.push(...response.data)

    if (regions.length >= response.total || response.data.length === 0) return regions
    page += 1
  }
}

async function getAllRoles(
  params: Pick<AnalyticsParams, 'from' | 'to'>,
  signal?: AbortSignal,
): Promise<RoleStat[]> {
  const roles: RoleStat[] = []
  let page = 1
  while (true) {
    const response = await get<RolePage>(
      '/roles',
      { ...params, page, page_size: DICTIONARY_PAGE_SIZE },
      signal,
    )
    roles.push(...response.data)
    if (roles.length >= response.total || response.data.length === 0) return roles
    page += 1
  }
}

async function getAllSkills(
  params: Pick<AnalyticsParams, 'from' | 'to'>,
  signal?: AbortSignal,
): Promise<TopSkills> {
  const skills: SkillStat[] = []
  let page = 1
  let total = 0

  while (page === 1 || skills.length < total) {
    const response = await get<TopSkills>(
      '/skills/top',
      { ...params, page, page_size: DICTIONARY_PAGE_SIZE },
      signal,
    )
    total = response.total
    skills.push(...response.data)
    if (response.data.length === 0) break
    page += 1
  }

  return { data: skills, page: 1, page_size: DICTIONARY_PAGE_SIZE, total }
}

export const api = {
  dashboard: (params: AnalyticsParams, signal?: AbortSignal) =>
    get<DashboardSummary>('/dashboard/summary', params, signal),
  salaryTrends: (params: AnalyticsParams, signal?: AbortSignal) =>
    get<SalaryTrends>('/trends/salaries', { ...params, grain: 'week' }, signal),
  demandTrends: (params: AnalyticsParams, signal?: AbortSignal) =>
    get<DemandTrends>('/trends/demand', { ...params, grain: 'week' }, signal),
  marketCoverage: (signal?: AbortSignal) =>
    get<TrendsCoverage>('/trends/coverage', {}, signal),
  currencies: (signal?: AbortSignal) =>
    get<CurrenciesResponse>('/currencies', {}, signal),
  marketDemand: (params: MarketParams, signal?: AbortSignal) =>
    get<DemandTrends>('/trends/demand', params, signal),
  topSkills: (
    params: AnalyticsParams,
    page: number,
    pageSize: number,
    signal?: AbortSignal,
  ) => get<TopSkills>('/skills/top', { ...params, page, page_size: pageSize }, signal),
  programmingLanguages: (
    params: AnalyticsParams,
    metric: RankingMetric,
    page: number,
    pageSize: number,
    signal?: AbortSignal,
  ) =>
    get<RankingPage>(
      '/rankings/programming-languages',
      { ...params, metric, page, page_size: pageSize },
      signal,
    ),
  managerialRoles: (
    params: AnalyticsParams,
    metric: RankingMetric,
    page: number,
    pageSize: number,
    signal?: AbortSignal,
  ) =>
    get<RankingPage>(
      '/rankings/managerial-roles',
      { ...params, metric, page, page_size: pageSize },
      signal,
    ),
  regions: getAllRegions,
  roles: getAllRoles,
  vacancySkills: getAllSkills,
  vacancies: (
    params: {
      q?: string
      role_id?: string
      region_id?: string
      skill_id?: string
      salary_min?: number
      salary_max?: number
      source?: string
      only_active?: boolean
      published_from?: string
      published_to?: string
      page: number
      page_size: number
      currency?: string
    },
    signal?: AbortSignal,
  ) => get<VacancyPage>('/vacancies', params, signal),
  assistantPreferences: () => get<import('./types').AssistantPreferences>('/assistant/preferences', {}),
  assistantPreferenceList: () => get<import('./types').AssistantPreferences[]>('/assistant/preferences/list', {}),
  saveAssistantPreferences: (value: import('./types').AssistantPreferences) =>
    mutate<import('./types').AssistantPreferences>('/assistant/preferences', 'PATCH', value, crypto.randomUUID()),
  archiveAssistantPreference: (id: string) =>
    mutate<void>('/assistant/preferences/archive', 'POST', { id }),
  assistantStatus: () => get<unknown>('/assistant/status', {}).then(normalizeAssistantStatus),
  assistantAnalysisPreview: () => get<{ snapshot_total: number }>('/assistant/analyze', {}),
  runAssistantAnalysis: () => mutate<{ run_id: string; status: string }>('/assistant/analyze', 'POST', undefined, crypto.randomUUID()),
  supersedeAssistantAnalysis: (runId: string) =>
    mutate<{ run_id: string; state: 'superseded' }>('/assistant/analyze/supersede', 'POST', { run_id: runId }),
  assistantMatches: () => get<unknown>('/assistant/matches', {}).then(normalizeAssistantMatches),
  telegramStatus: () => get<import('./types').TelegramStatus>('/assistant/telegram', {}),
  assistantAutomation: () => get<import('./types').AssistantAutomationSettings>('/assistant/automation', {}),
  updateAssistantAutomation: (value: Partial<import('./types').AssistantAutomationSettings>) =>
    mutate<import('./types').AssistantAutomationSettings>('/assistant/automation', 'PATCH', value),
  updateTelegramOptIn: (optedIn: boolean) => mutate<{ opted_in: boolean }>('/assistant/telegram', 'PATCH', { opted_in: optedIn }),
  telegramLink: () => mutate<{ deep_link: string; expires_at: string }>('/assistant/telegram/link', 'POST'),
  revokeTelegram: () => mutate<void>('/assistant/telegram', 'DELETE'),
  testTelegram: () => mutate<{ provider_message_id: number }>('/assistant/telegram/test', 'POST', { confirm: true }),
}
