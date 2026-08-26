import SearchRoundedIcon from '@mui/icons-material/SearchRounded'
import {
  Box,
  Card,
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
} from '@mui/material'
import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { api } from '../api/client'
import type { Vacancy } from '../api/types'
import { EmptyState, ErrorState } from '../components/DataState'
import { formatDate, formatSalaryRange } from '../utils/format'
import { getRegionLabel } from '../utils/regions'
import { dedupeVacancies, getNextVacancyPageParam } from './vacancyPagination'

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
      <Stack direction="row" useFlexGap sx={{ gap: 0.75, flexWrap: 'wrap' }}>
        {(vacancy.skills ?? []).slice(0, 5).map((skill) => (
          <Chip key={skill} label={skill} size="small" variant="outlined" />
        ))}
      </Stack>
    </Stack>
  )
}

const FIRST_PAGE = 1

export function VacanciesPage() {
  const [searchParams, setSearchParams] = useSearchParams()
  const query = searchParams.get('q') || ''
  const source = searchParams.get('source') || ''
  const onlyActive = searchParams.get('only_active') !== 'false'
  const pageSize = Math.min(Math.max(Number(searchParams.get('page_size')) || 20, 1), 100)
  const [draftQuery, setDraftQuery] = useState(query)
  const sentinelRef = useRef<HTMLDivElement | null>(null)

  const vacancies = useInfiniteQuery({
    queryKey: ['vacancies', { query, source, onlyActive, pageSize }],
    initialPageParam: FIRST_PAGE,
    queryFn: ({ pageParam, signal }) =>
      api.vacancies(
        {
          q: query || undefined,
          source: source || undefined,
          only_active: onlyActive,
          page: pageParam,
          page_size: pageSize,
        },
        signal,
      ),
    getNextPageParam: getNextVacancyPageParam,
  })
  const pages = useMemo(() => vacancies.data?.pages ?? [], [vacancies.data?.pages])
  const rows = useMemo(() => dedupeVacancies(pages), [pages])
  const total = pages[0]?.total ?? 0
  const loadNextPage = vacancies.fetchNextPage

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

  const regionPeriod = useMemo(() => {
    const dates = rows
      .map((vacancy) => vacancy.published_at?.slice(0, 10))
      .filter((date): date is string => Boolean(date && /^\d{4}-\d{2}-\d{2}$/.test(date)))
      .sort()
    return dates.length > 0 ? { from: dates[0], to: dates[dates.length - 1] } : undefined
  }, [rows])
  const regions = useQuery({
    queryKey: ['regions', 'dictionary', regionPeriod],
    queryFn: ({ signal }) =>
      regionPeriod ? api.regions(regionPeriod, signal) : Promise.resolve([]),
    enabled: Boolean(regionPeriod),
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

  const updateParams = (updates: Record<string, string | undefined>) => {
    const next = new URLSearchParams(searchParams)
    next.delete('page')
    Object.entries(updates).forEach(([key, value]) => {
      if (!value) next.delete(key)
      else next.set(key, value)
    })
    setSearchParams(next)
  }

  return (
    <Stack spacing={3}>
      <Box>
        <Typography variant="h4" component="h1" sx={{ fontWeight: 800 }}>
          Вакансии
        </Typography>
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
              updateParams({ q: draftQuery.trim() || undefined })
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
            <Typography color="text.secondary" aria-live="polite">
              Загружено {rows.length} из {total}
            </Typography>
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
                {rows.map((vacancy) => (
                  <TableRow key={vacancy.id || `${vacancy.source}-${vacancy.external_id}`} hover>
                    <TableCell sx={{ minWidth: 320 }}>
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
                ))}
              </TableBody>
            </Table>
          </TableContainer>

          <Stack sx={{ display: { xs: 'flex', md: 'none' }, p: 2 }} spacing={1.5}>
            {rows.map((vacancy) => (
              <Card
                key={vacancy.id || `${vacancy.source}-${vacancy.external_id}`}
                variant="outlined"
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
              </Card>
            ))}
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
