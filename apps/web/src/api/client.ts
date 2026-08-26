import type {
  ApiErrorBody,
  DashboardSummary,
  DemandTrends,
  RegionPage,
  RegionStat,
  RolePage,
  RoleStat,
  SalaryTrends,
  SkillStat,
  TopSkills,
  TrendsCoverage,
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

export interface MarketParams extends Record<string, string | undefined> {
  from: string
  to: string
  role_group?: string
  region_id?: string
  grain?: 'day' | 'week'
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
  marketDemand: (params: MarketParams, signal?: AbortSignal) =>
    get<DemandTrends>('/trends/demand', params, signal),
  topSkills: (
    params: AnalyticsParams,
    page: number,
    pageSize: number,
    signal?: AbortSignal,
  ) => get<TopSkills>('/skills/top', { ...params, page, page_size: pageSize }, signal),
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
      page: number
      page_size: number
    },
    signal?: AbortSignal,
  ) => get<VacancyPage>('/vacancies', params, signal),
}
