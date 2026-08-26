export interface Period {
  from: string
  to: string
}

export interface DashboardSummary {
  period: Period
  vacancies_active: number
  vacancies_new?: number
  median_salary: number
  salary_currency: 'RUB' | 'USD' | 'EUR' | 'CNY' | 'KZT' | 'AMD'
  salary_rate_date?: string | null
  salary_rate_provider?: string
  salary_sample_size?: number
  top_roles?: Array<{ role_id: string; title: string; count: number }>
  top_regions?: Array<{ region_id: string; title: string; count: number }>
  generated_at?: string
  cache?: 'HIT' | 'MISS'
}

export interface SalaryPoint {
  period_start: string
  median: number | null
  p25: number | null
  p75: number | null
  sample_size: number
  rate_date?: string | null
  rate_provider?: string
  coverage_warning?: string
}

export interface SalaryTrends {
  grain?: string
  currency?: 'RUB' | 'USD' | 'EUR' | 'CNY' | 'KZT' | 'AMD'
  points?: SalaryPoint[]
}

export interface DemandPoint {
  period_start: string
  active_count: number
  published_count: number
  new_count: number
  complete: boolean
  source_day_count: number
  median_salary?: number | null
  currency?: 'RUB' | 'USD' | 'EUR' | 'CNY' | 'KZT' | 'AMD'
  rate_date?: string | null
  rate_provider?: string
  coverage_warning?: string
}

export interface DemandTrends {
  grain?: string
  status?: 'ready' | 'no_complete_snapshots'
  source?: string
  method_version?: string
  points?: DemandPoint[]
}

export interface TrendsCoverage {
  status: 'ready' | 'collecting' | 'degraded'
  source: string
  method_version?: string
  available_years: number[]
  first_observation: string | null
  last_observation: string | null
  publication_from: string | null
  publication_to: string | null
  complete_daily_count: number
  complete_weekly_count: number
  expected_daily_count: number
  missed_daily_count: number
  incomplete_daily_count: number
  latest_incomplete_date: string | null
  next_scheduled_cycle: string
  latest_complete_cycle: string | null
  regions: Array<{ region_id: string; name: string }>
}

export interface SkillStat {
  skill_id?: string
  name: string
  count: number
  share: number
}

export interface TopSkills {
  data: SkillStat[]
  page: number
  page_size: number
  total: number
}

export type RankingMetric = 'count' | 'salary'

export interface RankingItem {
  id: string
  name: string
  rank: number
  vacancy_count: number
  share: number
  median_salary_rub: number | null
  median_salary: number | null
  salary_sample_size: number
}

export interface RankingPage {
  data: RankingItem[]
  metric: RankingMetric
  denominator: number
  min_salary_sample_size: number
  page: number
  page_size: number
  total: number
  currency: 'RUB' | 'USD' | 'EUR' | 'CNY' | 'KZT' | 'AMD'
  rate_date?: string | null
  rate_provider?: string
}

export interface Vacancy {
  id?: string
  source?: string
  source_name?: string
  source_url?: string | null
  external_id?: string
  title?: string
  role_id?: string | null
  region_id?: string | null
  salary_from?: number | null
  salary_to?: number | null
  salary_currency?: string | null
  salary_gross?: boolean | null
  salary_from_rub_net?: number | null
  salary_to_rub_net?: number | null
  salary_rate_date?: string | null
  salary_rate_provider?: string
  published_at?: string | null
  first_observed_at?: string | null
  is_fresh?: boolean
  is_active?: boolean
  status?: 'active' | 'inactive' | 'missing_from_last_complete_cycle'
  deactivated_at?: string | null
  skills?: string[]
}

export interface VacancyPage {
  data: Vacancy[]
  page: number
  page_size: number
  total: number
}

export interface RegionStat {
  region_id?: string
  title?: string
}

export interface RegionPage {
  data: RegionStat[]
  page: number
  page_size: number
  total: number
}

export interface RoleStat {
  role_id?: string
  title?: string
}

export interface RolePage {
  data: RoleStat[]
  page: number
  page_size: number
  total: number
}

export interface ApiErrorBody {
  error?: {
    code?: string
    message?: string
    request_id?: string
  }
}

export interface CurrencyRate {
  code: 'RUB' | 'USD' | 'EUR' | 'CNY' | 'KZT' | 'AMD'
  label: string
  symbol: string
  rate_date: string | null
  provider?: string
  stale_days: number | null
  available: boolean
}

export interface CurrenciesResponse {
  base_currency: 'RUB'
  rates: CurrencyRate[]
}

export interface AssistantPreferences {
  id?: string
  version?: number
  note: string
  hard_criteria: Record<string, unknown>
  soft_criteria: Record<string, unknown>
  weights: Record<string, number>
  active_from?: string
  archived_at?: string | null
  legacy_role_upgraded?: boolean
}

export interface AssistantStatus {
  ai_configured: boolean
  state: 'never_run' | 'queued' | 'running' | 'succeeded' | 'failed' | 'disabled'
  started_at?: string
  finished_at?: string
  last_checked_at?: string
  processed: number
  total: number
  eligible: number
  matched: number
  ai_calls: number
  skipped: number
  error_category?: string
  cursor_source?: string
  cursor_observed_at?: string
  pending_candidates: boolean
  provider?: string
  model?: string
  prompt_version?: string
  method_version?: string
}

export interface AssistantMatch {
  vacancy_id: string
  title?: string
  source_url?: string | null
  decision: 'match' | 'reject' | 'review'
  score: number
  method: 'deterministic' | 'ai'
  confidence?: 'low' | 'medium' | 'high' | null
  reasons: string[]
  unknowns: string[]
  conflicts: string[]
  evidence_ids: string[]
}

export interface TelegramStatus {
  configured: boolean
  linked: boolean
  opted_in: boolean
  pending?: number
  sent?: number
  failed?: number
  dead_lettered?: number
  last_error?: string
}

export interface AssistantAutomationSettings {
  ai_enabled: boolean
  telegram_enabled: boolean
  activation_at?: string
  max_ai_calls_per_hour: number
}
