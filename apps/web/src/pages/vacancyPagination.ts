import type { Vacancy, VacancyPage } from '../api/types'

const DEFAULT_POLL_INTERVAL_MS = 30_000
const MIN_POLL_INTERVAL_MS = 10_000

export function parseVacancyPollInterval(value: string | undefined): number {
  if (value === undefined || value.trim() === '') return DEFAULT_POLL_INTERVAL_MS
  const parsed = Number(value)
  if (!Number.isFinite(parsed) || parsed < 0) return DEFAULT_POLL_INTERVAL_MS
  if (parsed === 0) return 0
  return Math.max(parsed, MIN_POLL_INTERVAL_MS)
}

export function vacancyKey(vacancy: Vacancy): string {
  return vacancy.id || `${vacancy.source}-${vacancy.external_id}`
}

export function getNextVacancyPageParam(
  lastPage: VacancyPage,
  allPages: VacancyPage[],
): number | undefined {
  if (lastPage.data.length === 0) return undefined

  const loaded = new Set(allPages.flatMap((page) => page.data).map(vacancyKey)).size
  if (loaded >= lastPage.total) return undefined
  return lastPage.page + 1
}

export function dedupeVacancies(pages: VacancyPage[]): Vacancy[] {
  const seen = new Set<string>()
  return pages.flatMap((page) =>
    page.data.filter((vacancy) => {
      const key = vacancyKey(vacancy)
      if (seen.has(key)) return false
      seen.add(key)
      return true
    }),
  )
}

function compareNewest(left: Vacancy, right: Vacancy): number {
  const leftPublished = left.published_at ? Date.parse(left.published_at) : Number.NEGATIVE_INFINITY
  const rightPublished = right.published_at ? Date.parse(right.published_at) : Number.NEGATIVE_INFINITY
  if (leftPublished !== rightPublished) return rightPublished - leftPublished
  return vacancyKey(left).localeCompare(vacancyKey(right))
}

export function mergeFreshVacancies(
  pages: VacancyPage[],
  freshness: VacancyPage,
  newIDs: ReadonlySet<string>,
): VacancyPage[] {
  if (pages.length === 0) return pages

  const freshByID = new Map(freshness.data.map((vacancy) => [vacancyKey(vacancy), vacancy]))
  const mergedByID = new Map<string, Vacancy>()
  const current = dedupeVacancies(pages)
  const currentIDs = new Set(current.map(vacancyKey))
  const addedCount = freshness.data.filter(
    (vacancy) => newIDs.has(vacancyKey(vacancy)) && !currentIDs.has(vacancyKey(vacancy)),
  ).length
  current.forEach((vacancy) => {
    const key = vacancyKey(vacancy)
    mergedByID.set(key, freshByID.get(key) ?? vacancy)
  })
  freshness.data.forEach((vacancy) => {
    const key = vacancyKey(vacancy)
    if (newIDs.has(key)) mergedByID.set(key, vacancy)
  })

  const merged = [...mergedByID.values()].sort(compareNewest)
  let offset = 0
  return pages.map((page, index) => {
    const size = page.data.length + (index === 0 ? addedCount : 0)
    const data = merged.slice(offset, offset + size)
    offset += size
    return { ...page, data, total: freshness.total }
  })
}
