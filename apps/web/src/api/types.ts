export interface Period {
  from: string
  to: string
}

export interface DashboardSummary {
  period: Period
  vacancies_active: number
  vacancies_new?: number
  median_salary: number
  salary_currency: 'RUB'
  salary_sample_size?: number
  top_roles?: Array<{ role_id: string; title: string; count: number }>
  top_regions?: Array<{ region_id: string; title: string; count: number }>
  generated_at?: string
  cache?: 'HIT' | 'MISS'
}

export interface SalaryPoint {
  period_start: string
  median: number
  p25: number
  p75: number
  sample_size: number
}

export interface SalaryTrends {
  grain?: string
  currency?: 'RUB'
  points?: SalaryPoint[]
}

export interface DemandPoint {
  period_start: string
  active_count: number
  new_count: number
}

export interface DemandTrends {
  grain?: string
  points?: DemandPoint[]
}

export interface SkillStat {
  skill_id?: string
  name: string
  count: number
  share: number
}

export interface TopSkills {
  data?: SkillStat[]
}

export interface Vacancy {
  id?: string
  source?: string
  external_id?: string
  title?: string
  role_id?: string | null
  region_id?: string | null
  salary_from?: number | null
  salary_to?: number | null
  salary_currency?: string | null
  salary_gross?: boolean | null
  published_at?: string | null
  is_active?: boolean
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
