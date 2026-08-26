import SearchRoundedIcon from '@mui/icons-material/SearchRounded'
import OpenInNewRoundedIcon from '@mui/icons-material/OpenInNewRounded'
import {
  Box,
  Autocomplete,
  Card,
  CardActionArea,
  CardContent,
  Chip,
  FormControl,
  InputLabel,
  MenuItem,
  Select,
  Skeleton,
  Stack,
  Switch,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  TextField,
  Typography,
  FormControlLabel,
  Button,
  Link,
  Tooltip,
  useMediaQuery,
  useTheme,
} from '@mui/material'
import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { api } from '../api/client'
import type { Vacancy } from '../api/types'
import { EmptyState, ErrorState } from '../components/DataState'
import { formatDate, formatSalaryRange } from '../utils/format'
import { getRegionLabel } from '../utils/regions'
import { useCurrency } from '../components/CurrencyContext'
import {
  dedupeVacancies,
  getNextVacancyPageParam,
  mergeFreshVacancies,
  parseVacancyPollInterval,
  vacancyKey,
} from './vacancyPagination'

function vacancySourceLabel(vacancy: Vacancy) {
  return vacancy.source === 'hh' ? 'hh.ru' : vacancy.source_name || vacancy.source || 'источнике'
}

function vacancyLinkLabel(vacancy: Vacancy) {
  return `Открыть вакансию «${vacancy.title || 'Без названия'}» на ${vacancySourceLabel(vacancy)} в новой вкладке`
}

function VacancyDetails({ vacancy }: { vacancy: Vacancy }) {
  return (
    <Stack spacing={1}>
      <Typography sx={{ fontWeight: 700 }}>{vacancy.title || 'Без названия'}</Typography>
      <Typography variant="body2" color="text.secondary">
        {formatSalaryRange(
          vacancy.salary_from,
          vacancy.salary_to,
          vacancy.salary_currency || 'RUB',
        )}
        {vacancy.salary_gross === true ? ' · до вычета налогов' : ''}
      </Typography>
      {vacancy.salary_rate_date && (
        <Typography variant="caption" color="text.secondary">
          Курс ЦБ на {vacancy.salary_rate_date} · приблизительный официальный дневной курс,
          не live/платёжный
        </Typography>
      )}
      <Stack direction="row" useFlexGap sx={{ gap: 0.75, flexWrap: 'wrap' }}>
        {(vacancy.skills ?? []).slice(0, 5).map((skill) => (
          <Chip key={skill} label={skill} size="small" variant="outlined" />
        ))}
      </Stack>
      {vacancy.source_url && (
        <Typography
          component="span"
          color="primary"
          sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.5, width: 'fit-content' }}
        >
          Открыть на {vacancySourceLabel(vacancy)}
          <OpenInNewRoundedIcon fontSize="small" aria-hidden="true" />
        </Typography>
      )}
    </Stack>
  )
}

const FIRST_PAGE = 1
const FRESHNESS_PAGE_SIZE = 100
const NEW_HIGHLIGHT_DURATION_MS = 8_000
const EMPTY_VACANCY_IDS = new Set<string>()

const configuredPollIntervalMs = parseVacancyPollInterval(
  import.meta.env.VITE_VACANCIES_POLL_INTERVAL_MS,
)

function csvParam(value: string | null) {
  return value?.split(',').map((item) => item.trim()).filter(Boolean) ?? []
}

export function VacanciesPage({
  pollIntervalMs = configuredPollIntervalMs,
  highlightDurationMs = NEW_HIGHLIGHT_DURATION_MS,
}: {
  pollIntervalMs?: number
  highlightDurationMs?: number
}) {
  const { currency } = useCurrency()
  const theme = useTheme()
  const queryClient = useQueryClient()
  const prefersReducedMotion = useMediaQuery('(prefers-reduced-motion: reduce)')
  const [searchParams, setSearchParams] = useSearchParams()
  const query = searchParams.get('q') || ''
  const source = searchParams.get('source') || ''
  const onlyActive = searchParams.get('only_active') !== 'false'
  const roleParam = searchParams.get('role_id')
  const regionParam = searchParams.get('region_id')
  const skillParam = searchParams.get('skill_id')
  const roleIDs = useMemo(() => csvParam(roleParam), [roleParam])
  const regionIDs = useMemo(() => csvParam(regionParam), [regionParam])
  const skillIDs = useMemo(() => csvParam(skillParam), [skillParam])
  const salaryMin = searchParams.get('salary_min') || ''
  const salaryMax = searchParams.get('salary_max') || ''
  const pageSize = Math.min(Math.max(Number(searchParams.get('page_size')) || 20, 1), 100)
  const [draftQuery, setDraftQuery] = useState(query)
  const [draftSalaryMin, setDraftSalaryMin] = useState(salaryMin)
  const [draftSalaryMax, setDraftSalaryMax] = useState(salaryMax)
  const [newVacancyState, setNewVacancyState] = useState<{
    filter: string
    ids: Set<string>
  } | null>(null)
  const [announcementState, setAnnouncementState] = useState<{
    filter: string
    message: string
  } | null>(null)
  const sentinelRef = useRef<HTMLDivElement | null>(null)
  const freshnessBaselineRef = useRef<{ filter: string; ids: Set<string> } | null>(null)
  const highlightTimersRef = useRef(new Map<string, number>())
  const visibleVacancyIDsRef = useRef(new Set<string>())

  const vacancyQueryKey = useMemo(
    () =>
      ['vacancies', { query, source, onlyActive, pageSize, roleIDs, regionIDs, skillIDs, salaryMin, salaryMax, currency }] as const,
    [currency, onlyActive, pageSize, query, regionIDs, roleIDs, salaryMax, salaryMin, skillIDs, source],
  )
  const vacancies = useInfiniteQuery({
    queryKey: vacancyQueryKey,
    initialPageParam: FIRST_PAGE,
    queryFn: ({ pageParam, signal }) =>
      api.vacancies(
        {
          q: query || undefined,
          role_id: roleIDs.join(',') || undefined,
          region_id: regionIDs.join(',') || undefined,
          skill_id: skillIDs.join(',') || undefined,
          salary_min: salaryMin ? Number(salaryMin) : undefined,
          salary_max: salaryMax ? Number(salaryMax) : undefined,
          source: source || undefined,
          only_active: onlyActive,
          page: pageParam,
          page_size: pageSize,
          currency,
        },
        signal,
      ),
    getNextPageParam: getNextVacancyPageParam,
  })
  const pages = useMemo(() => vacancies.data?.pages ?? [], [vacancies.data?.pages])
  const rows = useMemo(() => dedupeVacancies(pages), [pages])
  const total = pages[0]?.total ?? 0
  const loadNextPage = vacancies.fetchNextPage
  const filterSignature = useMemo(() => JSON.stringify(vacancyQueryKey[1]), [vacancyQueryKey])
  const newVacancyIDs =
    newVacancyState?.filter === filterSignature ? newVacancyState.ids : EMPTY_VACANCY_IDS
  const newVacancyAnnouncement =
    announcementState?.filter === filterSignature ? announcementState.message : ''

  useEffect(() => {
    visibleVacancyIDsRef.current = new Set(rows.map(vacancyKey))
  }, [rows])

  useEffect(() => {
    freshnessBaselineRef.current = null
    highlightTimersRef.current.forEach((timer) => window.clearTimeout(timer))
    highlightTimersRef.current.clear()
  }, [filterSignature])

  useEffect(
    () => () => {
      highlightTimersRef.current.forEach((timer) => window.clearTimeout(timer))
      highlightTimersRef.current.clear()
    },
    [],
  )

  const freshness = useQuery({
    queryKey: ['vacancies-freshness', vacancyQueryKey[1]],
    enabled: vacancies.isSuccess && pollIntervalMs > 0,
    queryFn: ({ signal }) =>
      api.vacancies(
        {
          q: query || undefined,
          role_id: roleIDs.join(',') || undefined,
          region_id: regionIDs.join(',') || undefined,
          skill_id: skillIDs.join(',') || undefined,
          salary_min: salaryMin ? Number(salaryMin) : undefined,
          salary_max: salaryMax ? Number(salaryMax) : undefined,
          source: source || undefined,
          only_active: onlyActive,
          page: FIRST_PAGE,
          page_size: FRESHNESS_PAGE_SIZE,
          currency,
        },
        signal,
      ),
    refetchInterval: pollIntervalMs > 0 ? pollIntervalMs : false,
    refetchIntervalInBackground: false,
    refetchOnWindowFocus: true,
    refetchOnReconnect: true,
    retry: 1,
    staleTime: 0,
  })

  useEffect(() => {
    if (!freshness.data) return

    const polledIDs = freshness.data.data.map(vacancyKey)
    const baseline = freshnessBaselineRef.current
    if (!baseline || baseline.filter !== filterSignature) {
      freshnessBaselineRef.current = {
        filter: filterSignature,
        ids: new Set([...polledIDs, ...visibleVacancyIDsRef.current]),
      }
      return
    }

    visibleVacancyIDsRef.current.forEach((id) => baseline.ids.add(id))
    const discoveredIDs = polledIDs.filter((id) => !baseline.ids.has(id))
    polledIDs.forEach((id) => baseline.ids.add(id))
    const discovered = new Set(discoveredIDs)
    queryClient.setQueryData(vacancyQueryKey, (current: typeof vacancies.data) => {
      if (!current) return current
      return {
        ...current,
        pages: mergeFreshVacancies(current.pages, freshness.data, discovered),
      }
    })
    if (discoveredIDs.length === 0) return

    setNewVacancyState((current) => ({
      filter: filterSignature,
      ids: new Set([
        ...(current?.filter === filterSignature ? current.ids : EMPTY_VACANCY_IDS),
        ...discovered,
      ]),
    }))
    setAnnouncementState({
      filter: filterSignature,
      message: `Добавлено ${discovered.size} новых вакансий`,
    })
    discovered.forEach((id) => {
      const previousTimer = highlightTimersRef.current.get(id)
      if (previousTimer !== undefined) window.clearTimeout(previousTimer)
      const timer = window.setTimeout(() => {
        setNewVacancyState((current) => {
          if (!current || current.filter !== filterSignature) return current
          const next = new Set(current.ids)
          next.delete(id)
          return { ...current, ids: next }
        })
        highlightTimersRef.current.delete(id)
      }, highlightDurationMs)
      highlightTimersRef.current.set(id, timer)
    })
  }, [
    filterSignature,
    freshness.data,
    freshness.dataUpdatedAt,
    highlightDurationMs,
    queryClient,
    vacancyQueryKey,
  ])

  const newVacancyStyle = useCallback(
    (vacancy: Vacancy) => {
      if (!newVacancyIDs.has(vacancyKey(vacancy))) return undefined
      return {
        backgroundColor: theme.palette.highlight.new,
        boxShadow: `inset 3px 0 0 ${theme.palette.highlight.newEdge}`,
        animation: prefersReducedMotion ? 'none' : 'vacancy-new-fade 8s ease-out forwards',
        '@keyframes vacancy-new-fade': {
          from: { backgroundColor: theme.palette.highlight.new },
          to: { backgroundColor: theme.palette.highlight.newFade },
        },
      }
    },
    [newVacancyIDs, prefersReducedMotion, theme],
  )

  useEffect(() => {
    const sentinel = sentinelRef.current
    if (
      !sentinel ||
      typeof IntersectionObserver === 'undefined' ||
      !vacancies.hasNextPage ||
      vacancies.isFetchingNextPage
    ) {
      return
    }
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) void loadNextPage()
      },
      { rootMargin: '240px 0px' },
    )
    observer.observe(sentinel)
    return () => observer.disconnect()
  }, [loadNextPage, vacancies.hasNextPage, vacancies.isFetchingNextPage])

  const dictionaryPeriod = useMemo(
    () => ({ from: '2000-01-01', to: new Date().toISOString().slice(0, 10) }),
    [],
  )
  const regions = useQuery({
    queryKey: ['regions', 'dictionary', dictionaryPeriod],
    queryFn: ({ signal }) => api.regions(dictionaryPeriod, signal),
    staleTime: 5 * 60 * 1000,
  })
  const roles = useQuery({
    queryKey: ['roles', 'dictionary', dictionaryPeriod],
    queryFn: ({ signal }) => api.roles(dictionaryPeriod, signal),
    staleTime: 5 * 60 * 1000,
  })
  const skills = useQuery({
    queryKey: ['skills', 'vacancy-dictionary', dictionaryPeriod],
    queryFn: ({ signal }) => api.vacancySkills(dictionaryPeriod, signal),
    staleTime: 5 * 60 * 1000,
  })
  const regionNames = useMemo(
    () =>
      new Map(
        (regions.data ?? []).flatMap((region) => {
          const title = region.title?.trim()
          return region.region_id && title ? [[region.region_id, title] as const] : []
        }),
      ),
    [regions.data],
  )

  const updateParams = useCallback((updates: Record<string, string | undefined>) => {
    const next = new URLSearchParams(searchParams)
    next.delete('page')
    Object.entries(updates).forEach(([key, value]) => {
      if (!value) next.delete(key)
      else next.set(key, value)
    })
    setSearchParams(next)
  }, [searchParams, setSearchParams])

  useEffect(() => {
    if (draftQuery === query) return
    const timer = window.setTimeout(
      () => updateParams({ q: draftQuery.trim() || undefined }),
      400,
    )
    return () => window.clearTimeout(timer)
  }, [draftQuery, query, updateParams])

  const roleOptions = roles.data ?? []
  const regionOptions = regions.data ?? []
  const skillOptions = skills.data?.data ?? []
  const activeFilterCount =
    Number(Boolean(query)) + roleIDs.length + regionIDs.length + skillIDs.length +
    Number(Boolean(salaryMin)) + Number(Boolean(salaryMax)) + Number(Boolean(source)) +
    Number(!onlyActive)

  return (
    <Stack spacing={3}>
      <Box>
        <Stack direction="row" sx={{ alignItems: 'center', justifyContent: 'space-between', gap: 2 }}>
          <Typography variant="h4" component="h1" sx={{ fontWeight: 800 }}>
            Вакансии
          </Typography>
          <Button
            variant="outlined"
            disabled={vacancies.isFetching}
            onClick={() => void queryClient.resetQueries({ queryKey: vacancyQueryKey, exact: true })}
          >
            Обновить
          </Button>
        </Stack>
        <Typography color="text.secondary" sx={{ mt: 0.5 }}>
          Drill-down по нормализованным данным из PostgreSQL
        </Typography>
      </Box>

      <Card variant="outlined">
        <CardContent>
          <Stack
            component="form"
            direction={{ xs: 'column', md: 'row' }}
            useFlexGap
            sx={{
              gap: 1.5,
              alignItems: { md: 'center' },
              flexWrap: { md: 'wrap' },
            }}
            onSubmit={(event) => {
              event.preventDefault()
              updateParams({
                q: draftQuery.trim() || undefined,
                salary_min: draftSalaryMin || undefined,
                salary_max: draftSalaryMax || undefined,
              })
            }}
          >
            <TextField
              label="Поиск по названию"
              placeholder="Например, Go developer"
              value={draftQuery}
              onChange={(event) => setDraftQuery(event.target.value)}
              size="small"
              fullWidth
              sx={{ minWidth: 0, flex: { md: '1 1 auto' }, width: { md: 'auto' } }}
            />
            <Autocomplete
              multiple
              size="small"
              options={regionOptions}
              loading={regions.isLoading}
              value={regionOptions.filter((option) => option.region_id && regionIDs.includes(option.region_id))}
              getOptionLabel={(option) => option.title || 'Регион'}
              isOptionEqualToValue={(option, value) => option.region_id === value.region_id}
              onChange={(_, values) =>
                updateParams({ region_id: values.flatMap((item) => item.region_id ? [item.region_id] : []).join(',') || undefined })
              }
              renderInput={(params) => <TextField {...params} label="Регионы" />}
              sx={{ minWidth: { md: 240 }, flex: { md: '1 1 240px' } }}
            />
            <Autocomplete
              multiple
              size="small"
              options={roleOptions}
              loading={roles.isLoading}
              value={roleOptions.filter((option) => option.role_id && roleIDs.includes(option.role_id))}
              getOptionLabel={(option) => option.title || 'Роль'}
              isOptionEqualToValue={(option, value) => option.role_id === value.role_id}
              onChange={(_, values) =>
                updateParams({ role_id: values.flatMap((item) => item.role_id ? [item.role_id] : []).join(',') || undefined })
              }
              renderInput={(params) => <TextField {...params} label="Роли" />}
              sx={{ minWidth: { md: 240 }, flex: { md: '1 1 240px' } }}
            />
            <Autocomplete
              multiple
              size="small"
              options={skillOptions}
              loading={skills.isLoading}
              value={skillOptions.filter((option) => option.skill_id && skillIDs.includes(option.skill_id))}
              getOptionLabel={(option) => option.name}
              isOptionEqualToValue={(option, value) => option.skill_id === value.skill_id}
              onChange={(_, values) =>
                updateParams({ skill_id: values.flatMap((item) => item.skill_id ? [item.skill_id] : []).join(',') || undefined })
              }
              renderInput={(params) => <TextField {...params} label="Стек / навыки (любой)" />}
              sx={{ minWidth: { md: 280 }, flex: { md: '1 1 280px' } }}
            />
            <TextField
              label="Зарплата от"
              type="number"
              value={draftSalaryMin}
              onChange={(event) => setDraftSalaryMin(event.target.value)}
              slotProps={{ htmlInput: { min: 0, max: 2000000, step: 10000 } }}
              size="small"
              sx={{ width: { xs: '100%', md: 160 } }}
            />
            <TextField
              label="Зарплата до"
              type="number"
              value={draftSalaryMax}
              onChange={(event) => setDraftSalaryMax(event.target.value)}
              slotProps={{ htmlInput: { min: 0, max: 2000000, step: 10000 } }}
              size="small"
              sx={{ width: { xs: '100%', md: 160 } }}
            />
            <FormControl
              size="small"
              sx={{ minWidth: 150, width: { xs: '100%', md: 'auto' }, flexShrink: 0 }}
            >
              <InputLabel id="source-label">Источник</InputLabel>
              <Select
                labelId="source-label"
                label="Источник"
                value={source}
                onChange={(event) =>
                  updateParams({ source: event.target.value || undefined })
                }
              >
                <MenuItem value="">Все</MenuItem>
                <MenuItem value="hh">HeadHunter</MenuItem>
              </Select>
            </FormControl>
            <FormControlLabel
              sx={{ whiteSpace: 'nowrap', flexShrink: 0 }}
              control={
                <Switch
                  checked={onlyActive}
                  onChange={(_, checked) =>
                    updateParams({ only_active: checked ? undefined : 'false' })
                  }
                />
              }
              label="Только активные"
            />
            <Button
              type="submit"
              variant="contained"
              startIcon={<SearchRoundedIcon />}
              sx={{
                width: { xs: '100%', md: 'auto' },
                minWidth: { md: 'max-content' },
                flexShrink: 0,
                whiteSpace: 'nowrap',
              }}
            >
              Найти
            </Button>
            <Button
              variant="text"
              disabled={activeFilterCount === 0}
              onClick={() => {
                setDraftQuery('')
                setDraftSalaryMin('')
                setDraftSalaryMax('')
                setSearchParams({})
              }}
            >
              Сбросить все
            </Button>
          </Stack>
          <Stack direction="row" useFlexGap sx={{ mt: 1.5, gap: 0.75, flexWrap: 'wrap', alignItems: 'center' }}>
            <Chip size="small" label={`Активных фильтров: ${activeFilterCount}`} />
            {regions.isError && <Chip size="small" color="warning" label="Справочник регионов недоступен" />}
            {roles.isError && <Chip size="small" color="warning" label="Справочник ролей недоступен" />}
            {skills.isError && <Chip size="small" color="warning" label="Справочник навыков недоступен" />}
            <Tooltip title="Фильтр пересчитывается сервером по последнему доступному официальному курсу">
              <Typography variant="caption" color="text.secondary">
                Зарплата: {currency}, оценка net. Используется приблизительный официальный
                дневной курс ЦБ, не live-курс и не курс выплаты.
              </Typography>
            </Tooltip>
          </Stack>
        </CardContent>
      </Card>

      {vacancies.isError && rows.length === 0 && (
        <ErrorState error={vacancies.error} onRetry={() => vacancies.refetch()} />
      )}

      {vacancies.isLoading && (
        <Card variant="outlined">
          <CardContent>
            <Stack spacing={1.5}>
              {[1, 2, 3, 4, 5].map((item) => (
                <Skeleton key={item} variant="rounded" height={64} />
              ))}
            </Stack>
          </CardContent>
        </Card>
      )}

      {vacancies.isSuccess && rows.length === 0 && (
        <Card variant="outlined">
          <EmptyState
            title="Вакансии не найдены"
            description="Измените запрос или снимите фильтр активных вакансий."
          />
        </Card>
      )}

      {rows.length > 0 && (
        <Card variant="outlined">
          <Box sx={{ px: 2, pt: 2 }}>
            <Typography color="text.secondary" aria-live="polite" sx={{ mb: 1 }}>
              Загружено {rows.length} из {total}
            </Typography>
            {newVacancyAnnouncement && (
              <Typography color="success.main" role="status" aria-live="polite" sx={{ mt: 0.5 }}>
                {newVacancyAnnouncement}
              </Typography>
            )}
            {freshness.isError && freshness.failureCount >= 2 && (
              <Typography variant="caption" color="text.secondary" role="status">
                Автообновление временно недоступно; список сохранён.
              </Typography>
            )}
          </Box>
          <TableContainer sx={{ display: { xs: 'none', md: 'block' } }}>
            <Table>
              <TableHead>
                <TableRow>
                  <TableCell>Вакансия</TableCell>
                  <TableCell>Источник</TableCell>
                  <TableCell>Регион</TableCell>
                  <TableCell>Опубликована</TableCell>
                  <TableCell>Статус</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {rows.map((vacancy) => {
                  const isNew = newVacancyIDs.has(vacancyKey(vacancy))
                  return (
                    <TableRow
                      key={vacancyKey(vacancy)}
                      data-new-vacancy={isNew || undefined}
                      data-reduced-motion={isNew ? prefersReducedMotion : undefined}
                      sx={{
                        ...newVacancyStyle(vacancy),
                        position: 'relative',
                        ...(vacancy.source_url && {
                          cursor: 'pointer',
                          transition: 'box-shadow 160ms ease',
                          '&:hover': {
                            boxShadow: `inset 0 0 0 1px ${theme.palette.primary.main}`,
                          },
                          '&:focus-within': {
                            outline: '3px solid',
                            outlineColor: 'primary.main',
                            outlineOffset: '-3px',
                          },
                        }),
                      }}
                    >
                      <TableCell sx={{ minWidth: 320 }}>
                        {vacancy.source_url && (
                          <Link
                            href={vacancy.source_url}
                            target="_blank"
                            rel="noopener noreferrer nofollow"
                            aria-label={vacancyLinkLabel(vacancy)}
                            sx={{
                              position: 'absolute',
                              inset: 0,
                              zIndex: 1,
                              '&:focus-visible': { outline: 'none' },
                            }}
                          />
                        )}
                        <VacancyDetails vacancy={vacancy} />
                      </TableCell>
                      <TableCell>{vacancy.source?.toUpperCase() || '—'}</TableCell>
                      <TableCell>{getRegionLabel(vacancy.region_id, regionNames)}</TableCell>
                      <TableCell>{formatDate(vacancy.published_at)}</TableCell>
                      <TableCell>
                        <Chip
                          size="small"
                          color={vacancy.is_active ? 'success' : 'default'}
                          label={vacancy.is_active ? 'Активна' : 'Закрыта'}
                        />
                      </TableCell>
                    </TableRow>
                  )
                })}
              </TableBody>
            </Table>
          </TableContainer>

          <Stack sx={{ display: { xs: 'flex', md: 'none' }, p: 2 }} spacing={1.5}>
            {rows.map((vacancy) => {
              const isNew = newVacancyIDs.has(vacancyKey(vacancy))
              return (
                <Card
                  key={vacancyKey(vacancy)}
                  variant="outlined"
                  data-new-vacancy={isNew || undefined}
                  data-reduced-motion={isNew ? prefersReducedMotion : undefined}
                  sx={{
                    ...newVacancyStyle(vacancy),
                    ...(vacancy.source_url && {
                      transition: 'box-shadow 160ms ease, transform 160ms ease',
                      '&:hover': {
                        boxShadow: 2,
                        transform: 'translateY(-1px)',
                      },
                      '&:focus-within': {
                        outline: '3px solid',
                        outlineColor: 'primary.main',
                        outlineOffset: 2,
                      },
                    }),
                  }}
                >
                  {vacancy.source_url ? (
                    <CardActionArea
                      component="a"
                      href={vacancy.source_url}
                      target="_blank"
                      rel="noopener noreferrer nofollow"
                      aria-label={vacancyLinkLabel(vacancy)}
                      sx={{ '&.Mui-focusVisible': { outline: 'none' } }}
                    >
                      <CardContent>
                        <VacancyDetails vacancy={vacancy} />
                        <Typography variant="body2" color="text.secondary" sx={{ mt: 1.5 }}>
                          {getRegionLabel(vacancy.region_id, regionNames)}
                        </Typography>
                        <Stack
                          direction="row"
                          sx={{ justifyContent: 'space-between', mt: 2 }}
                        >
                          <Typography variant="caption" color="text.secondary">
                            {vacancy.source?.toUpperCase() || '—'} · {formatDate(vacancy.published_at)}
                          </Typography>
                          <Chip
                            size="small"
                            color={vacancy.is_active ? 'success' : 'default'}
                            label={vacancy.is_active ? 'Активна' : 'Закрыта'}
                          />
                        </Stack>
                      </CardContent>
                    </CardActionArea>
                  ) : (
                    <CardContent>
                      <VacancyDetails vacancy={vacancy} />
                      <Typography variant="body2" color="text.secondary" sx={{ mt: 1.5 }}>
                        {getRegionLabel(vacancy.region_id, regionNames)}
                      </Typography>
                      <Stack
                        direction="row"
                        sx={{ justifyContent: 'space-between', mt: 2 }}
                      >
                        <Typography variant="caption" color="text.secondary">
                          {vacancy.source?.toUpperCase() || '—'} · {formatDate(vacancy.published_at)}
                        </Typography>
                        <Chip
                          size="small"
                          color={vacancy.is_active ? 'success' : 'default'}
                          label={vacancy.is_active ? 'Активна' : 'Закрыта'}
                        />
                      </Stack>
                    </CardContent>
                  )}
                </Card>
              )
            })}
          </Stack>

          <Stack sx={{ alignItems: 'center', p: 2 }} spacing={1}>
            <Box ref={sentinelRef} data-testid="vacancy-scroll-sentinel" sx={{ height: 1 }} />
            {vacancies.isFetchNextPageError && (
              <Typography color="error" role="alert">
                Не удалось загрузить следующую страницу.
              </Typography>
            )}
            {vacancies.hasNextPage ? (
              <Button
                variant="outlined"
                disabled={vacancies.isFetchingNextPage}
                onClick={() => void vacancies.fetchNextPage()}
              >
                {vacancies.isFetchingNextPage ? 'Загрузка…' : vacancies.isFetchNextPageError ? 'Повторить' : 'Загрузить ещё'}
              </Button>
            ) : (
              <Typography color="text.secondary">Все доступные вакансии загружены</Typography>
            )}
          </Stack>
        </Card>
      )}
    </Stack>
  )
}
