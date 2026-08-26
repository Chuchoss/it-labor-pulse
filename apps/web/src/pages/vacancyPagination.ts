import type { Vacancy, VacancyPage } from '../api/types'

export function getNextVacancyPageParam(
  lastPage: VacancyPage,
  allPages: VacancyPage[],
): number | undefined {
  if (lastPage.data.length === 0) return undefined

  const seenBefore = new Set(
    allPages
      .slice(0, -1)
      .flatMap((page) => page.data)
      .map((vacancy) => vacancy.id || `${vacancy.source}-${vacancy.external_id}`),
  )
  const hasNewVacancies = lastPage.data.some(
    (vacancy) => !seenBefore.has(vacancy.id || `${vacancy.source}-${vacancy.external_id}`),
  )
  if (!hasNewVacancies) return undefined

  const loadedThrough = lastPage.page * lastPage.page_size
  if (loadedThrough >= lastPage.total) return undefined
  return lastPage.page + 1
}

export function dedupeVacancies(pages: VacancyPage[]): Vacancy[] {
  const seen = new Set<string>()
  return pages.flatMap((page) =>
    page.data.filter((vacancy) => {
      const key = vacancy.id || `${vacancy.source}-${vacancy.external_id}`
      if (seen.has(key)) return false
      seen.add(key)
      return true
    }),
  )
}
