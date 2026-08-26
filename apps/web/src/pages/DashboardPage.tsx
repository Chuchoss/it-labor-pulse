import CalendarMonthRoundedIcon from '@mui/icons-material/CalendarMonthRounded'
import CurrencyRubleRoundedIcon from '@mui/icons-material/CurrencyRubleRounded'
import FiberNewRoundedIcon from '@mui/icons-material/FiberNewRounded'
import WorkRoundedIcon from '@mui/icons-material/WorkRounded'
import { LineChart } from '@mui/x-charts/LineChart'
import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  CircularProgress,
  LinearProgress,
  Skeleton,
  Stack,
  TextField,
  ToggleButton,
  ToggleButtonGroup,
  Tooltip,
  Typography,
  useTheme,
} from '@mui/material'
import { useState, type ReactNode } from 'react'
import { useSearchParams } from 'react-router-dom'
import { api } from '../api/client'
import { EmptyState, ErrorState } from '../components/DataState'
import { formatCompact, formatNumber, formatSalary } from '../utils/format'
import type { AnalyticsParams } from '../api/client'
import type { RankingMetric } from '../api/types'
import { useCurrency } from '../components/CurrencyContext'

const SKILLS_PAGE_SIZE = 10

function isoDate(date: Date) {
  return date.toISOString().slice(0, 10)
}

function defaultPeriod() {
  const to = new Date()
  const from = new Date()
  from.setDate(from.getDate() - 30)
  return { from: isoDate(from), to: isoDate(to) }
}

function MetricCard({
  label,
  value,
  helper,
  icon,
  loading,
}: {
  label: string
  value: string
  helper?: string
  icon: ReactNode
  loading: boolean
}) {
  return (
    <Card variant="outlined">
      <CardContent>
        <Stack
          direction="row"
          sx={{ justifyContent: 'space-between', alignItems: 'flex-start' }}
        >
          <Box>
            <Typography variant="body2" color="text.secondary">
              {label}
            </Typography>
            {loading ? (
              <Skeleton width={110} height={44} />
            ) : (
              <Typography variant="h4" sx={{ mt: 0.5, fontWeight: 750 }}>
                {value}
              </Typography>
            )}
            <Typography variant="caption" color="text.secondary">
              {helper || '\u00a0'}
            </Typography>
          </Box>
          <Box
            sx={{
              p: 1,
              borderRadius: 2,
              color: 'primary.main',
              bgcolor: 'action.hover',
              display: 'flex',
            }}
          >
            {icon}
          </Box>
        </Stack>
      </CardContent>
    </Card>
  )
}

function RankingCardHeader({
  title,
  subtitle,
}: {
  title: string
  subtitle: ReactNode
}) {
  return (
    <Box
      className="dashboard-ranking-card__header"
      sx={{
        minHeight: { lg: (theme) => theme.spacing(5.5) },
      }}
    >
      <Typography variant="h6">{title}</Typography>
      {subtitle}
    </Box>
  )
}

function RankingCard({
  kind,
  title,
  params,
}: {
  kind: 'languages' | 'managerial'
  title: string
  params: AnalyticsParams
}) {
  const [metric, setMetric] = useState<RankingMetric>('count')
  const ranking = useInfiniteQuery({
    queryKey: ['dashboard-ranking', kind, metric, params],
    queryFn: ({ pageParam, signal }) =>
      kind === 'languages'
        ? api.programmingLanguages(params, metric, pageParam, SKILLS_PAGE_SIZE, signal)
        : api.managerialRoles(params, metric, pageParam, SKILLS_PAGE_SIZE, signal),
    initialPageParam: 1,
    getNextPageParam: (lastPage) =>
      lastPage.page * lastPage.page_size < lastPage.total ? lastPage.page + 1 : undefined,
  })
  const items = Array.from(
    new Map(
      (ranking.data?.pages.flatMap((page) => page.data) ?? []).map((item) => [item.id, item]),
    ).values(),
  )
  const total = ranking.data?.pages.at(-1)?.total ?? 0
  const minimumSample = ranking.data?.pages[0]?.min_salary_sample_size ?? 5
  const maxValue = Math.max(
    metric === 'count'
      ? (ranking.data?.pages[0]?.data[0]?.vacancy_count ?? 0)
      : (ranking.data?.pages[0]?.data[0]?.median_salary
        ?? ranking.data?.pages[0]?.data[0]?.median_salary_rub
        ?? 0),
    1,
  )

  return (
    <Card variant="outlined" sx={{ display: 'flex' }}>
      <CardContent sx={{ flex: 1 }}>
        <RankingCardHeader
          title={title}
          subtitle={
            <Tooltip
              title={
                metric === 'salary'
                  ? `Медиана offered salary в ${params.currency} net. Минимальная выборка: ${minimumSample}.`
                  : 'Доля считается среди активных вакансий соответствующего рейтинга за период.'
              }
            >
              <Typography variant="body2" color="text.secondary">
                {metric === 'count'
                  ? 'Активные вакансии за период'
                  : `Медианная зарплата, ${params.currency} net`}
              </Typography>
            </Tooltip>
          }
        />
        <ToggleButtonGroup
          exclusive
          size="small"
          fullWidth
          value={metric}
          onChange={(_, value: RankingMetric | null) => value && setMetric(value)}
          aria-label={`Метрика рейтинга: ${title}`}
          sx={{ mt: 2 }}
        >
          <ToggleButton value="count">По количеству</ToggleButton>
          <ToggleButton value="salary">По зарплате</ToggleButton>
        </ToggleButtonGroup>

        {ranking.isPending && (
          <Stack spacing={2} sx={{ mt: 3 }}>
            {[1, 2, 3, 4, 5].map((item) => (
              <Skeleton key={item} height={28} />
            ))}
          </Stack>
        )}
        {ranking.isError && items.length === 0 && (
          <Box sx={{ mt: 2 }}>
            <ErrorState error={ranking.error} onRetry={() => ranking.refetch()} compact />
          </Box>
        )}
        {ranking.isSuccess && items.length === 0 && (
          <EmptyState
            title={metric === 'salary' ? 'Недостаточно зарплатных данных' : undefined}
          />
        )}
        <Stack spacing={2.1} sx={{ mt: 3 }}>
          {items.map((item) => {
            const value =
              metric === 'count'
                ? item.vacancy_count
                : (item.median_salary ?? item.median_salary_rub ?? 0)
            return (
              <Box key={item.id}>
                <Stack direction="row" sx={{ justifyContent: 'space-between', gap: 1, mb: 0.6 }}>
                  <Typography variant="body2" sx={{ fontWeight: 650 }}>
                    {item.rank}. {item.name}
                  </Typography>
                  <Typography variant="caption" color="text.secondary" sx={{ whiteSpace: 'nowrap' }}>
                    {metric === 'count'
                      ? `${formatCompact(item.vacancy_count)} · ${Math.round(item.share * 100)}%`
                      : `${formatSalary(item.median_salary ?? item.median_salary_rub, params.currency)} · n=${formatNumber(item.salary_sample_size)}`}
                  </Typography>
                </Stack>
                <LinearProgress
                  variant="determinate"
                  value={(value / maxValue) * 100}
                  sx={{ height: 6, borderRadius: 3 }}
                />
              </Box>
            )
          })}
        </Stack>
        {items.length > 0 && (
          <Stack
            direction={{ xs: 'column', sm: 'row' }}
            sx={{ mt: 2.5, gap: 1, alignItems: { sm: 'center' } }}
          >
            {(ranking.hasNextPage || ranking.isFetchNextPageError) && (
              <Button
                variant="outlined"
                onClick={() => void ranking.fetchNextPage()}
                disabled={ranking.isFetchingNextPage}
                startIcon={
                  ranking.isFetchingNextPage ? <CircularProgress size={16} /> : undefined
                }
                aria-label={
                  ranking.isFetchNextPageError
                    ? `Повторить загрузку: ${title}`
                    : `Показать ещё: ${title}`
                }
              >
                {ranking.isFetchNextPageError ? 'Повторить' : 'Показать ещё'}
              </Button>
            )}
            <Typography
              variant="caption"
              color={ranking.isFetchNextPageError ? 'error' : 'text.secondary'}
              aria-live="polite"
            >
              {ranking.isFetchNextPageError
                ? `Не удалось загрузить ещё. Показано ${items.length} из ${total}`
                : `Показано ${items.length} из ${total}`}
            </Typography>
          </Stack>
        )}
      </CardContent>
    </Card>
  )
}

export function DashboardPage() {
  const theme = useTheme()
  const { currency } = useCurrency()
  const fallback = defaultPeriod()
  const [searchParams, setSearchParams] = useSearchParams()
  const from = searchParams.get('from') || fallback.from
  const to = searchParams.get('to') || fallback.to
  const [draftFrom, setDraftFrom] = useState(from)
  const [draftTo, setDraftTo] = useState(to)
  const params = { from, to, currency }

  const summary = useQuery({
    queryKey: ['dashboard', params],
    queryFn: ({ signal }) => api.dashboard(params, signal),
  })
  const salaries = useQuery({
    queryKey: ['salary-trends', params],
    queryFn: ({ signal }) => api.salaryTrends(params, signal),
  })
  const demand = useQuery({
    queryKey: ['demand-trends', params],
    queryFn: ({ signal }) => api.demandTrends(params, signal),
  })
  const skills = useInfiniteQuery({
    queryKey: ['top-skills', params],
    queryFn: ({ pageParam, signal }) =>
      api.topSkills(params, pageParam, SKILLS_PAGE_SIZE, signal),
    initialPageParam: 1,
    getNextPageParam: (lastPage) =>
      lastPage.page * lastPage.page_size < lastPage.total ? lastPage.page + 1 : undefined,
  })

  const applyPeriod = () => {
    if (!draftFrom || !draftTo || draftFrom > draftTo) return
    setSearchParams({ from: draftFrom, to: draftTo })
  }

  const lowSample = (summary.data?.salary_sample_size ?? 0) < 5
  const salaryPoints = salaries.data?.points ?? []
  const demandPoints = demand.data?.points ?? []
  const skillItems = Array.from(
    new Map(
      (skills.data?.pages.flatMap((page) => page.data) ?? []).map((item) => [
        item.skill_id || item.name,
        item,
      ]),
    ).values(),
  )
  const skillTotal = skills.data?.pages.at(-1)?.total ?? 0
  const maxSkillCount = Math.max(skills.data?.pages[0]?.data[0]?.count ?? 0, 1)

  return (
    <Stack spacing={3.5}>
      <Stack
        direction={{ xs: 'column', sm: 'row' }}
        sx={{ justifyContent: 'space-between', alignItems: { sm: 'flex-end' }, gap: 2 }}
      >
        <Box>
          <Typography variant="h4" component="h1" sx={{ fontWeight: 800 }}>
            Обзор рынка
          </Typography>
          <Typography color="text.secondary" sx={{ mt: 0.5 }}>
            Спрос и предлагаемые зарплаты по данным вакансий
          </Typography>
        </Box>
        <Stack
          component="form"
          direction={{ xs: 'column', sm: 'row' }}
          sx={{ gap: 1 }}
          onSubmit={(event) => {
            event.preventDefault()
            applyPeriod()
          }}
        >
          <TextField
            label="С"
            type="date"
            size="small"
            value={draftFrom}
            onChange={(event) => setDraftFrom(event.target.value)}
            slotProps={{ inputLabel: { shrink: true } }}
          />
          <TextField
            label="По"
            type="date"
            size="small"
            value={draftTo}
            onChange={(event) => setDraftTo(event.target.value)}
            error={draftFrom > draftTo}
            slotProps={{ inputLabel: { shrink: true } }}
          />
          <Button type="submit" variant="contained" startIcon={<CalendarMonthRoundedIcon />}>
            Применить
          </Button>
        </Stack>
      </Stack>

      {summary.isError && <ErrorState error={summary.error} onRetry={() => summary.refetch()} />}

      <Box
        sx={{
          display: 'grid',
          gridTemplateColumns: { xs: '1fr', sm: 'repeat(2, 1fr)', xl: 'repeat(4, 1fr)' },
          gap: 2,
        }}
      >
        <MetricCard
          label="Активные вакансии"
          value={formatNumber(summary.data?.vacancies_active)}
          helper="на конец периода"
          icon={<WorkRoundedIcon />}
          loading={summary.isLoading}
        />
        <MetricCard
          label="Новые вакансии"
          value={formatNumber(summary.data?.vacancies_new)}
          helper="за выбранный период"
          icon={<FiberNewRoundedIcon />}
          loading={summary.isLoading}
        />
        <MetricCard
          label="Медианная зарплата"
          value={lowSample ? 'Мало данных' : formatSalary(summary.data?.median_salary, currency)}
          helper={`Выборка: n=${formatNumber(summary.data?.salary_sample_size)}`}
          icon={<CurrencyRubleRoundedIcon />}
          loading={summary.isLoading}
        />
        <MetricCard
          label="Период"
          value={`${from.slice(5).replace('-', '.')} — ${to.slice(5).replace('-', '.')}`}
          helper={summary.data?.cache === 'HIT' ? 'данные из кэша' : 'актуальный расчёт'}
          icon={<CalendarMonthRoundedIcon />}
          loading={summary.isLoading}
        />
      </Box>

      {lowSample && summary.isSuccess && (
        <Alert severity="info">
          Для устойчивой медианы нужно не менее пяти вакансий с указанной зарплатой.
        </Alert>
      )}

      <Box
        sx={{
          display: 'grid',
          gridTemplateColumns: { xs: '1fr', lg: 'repeat(3, minmax(0, 1fr))' },
          gap: 2,
          alignItems: 'stretch',
        }}
      >
        <Card variant="outlined" sx={{ gridColumn: { lg: '1 / -1' } }}>
          <CardContent>
            <Typography variant="h6">Динамика спроса</Typography>
            <Typography variant="body2" color="text.secondary">
              Активные и новые вакансии по неделям
            </Typography>
            {demand.isLoading && <Skeleton variant="rounded" height={300} sx={{ mt: 2 }} />}
            {demand.isError && (
              <Box sx={{ mt: 2 }}>
                <ErrorState error={demand.error} onRetry={() => demand.refetch()} compact />
              </Box>
            )}
            {demand.isSuccess && demandPoints.length === 0 && <EmptyState />}
            {demandPoints.length > 0 && (
              <LineChart
                height={310}
                xAxis={[
                  {
                    data: demandPoints.map((point) => new Date(point.period_start)),
                    scaleType: 'time',
                    valueFormatter: (value: Date) =>
                      value.toLocaleDateString('ru-RU', { day: 'numeric', month: 'short' }),
                  },
                ]}
                series={[
                  {
                    data: demandPoints.map((point) => point.active_count),
                    label: 'Активные',
                    color: theme.palette.chart.primary,
                  },
                  {
                    data: demandPoints.map((point) => point.new_count),
                    label: 'Новые',
                    color: theme.palette.chart.secondary,
                  },
                ]}
                grid={{ horizontal: true }}
              />
            )}
          </CardContent>
        </Card>

        <Card variant="outlined" sx={{ display: 'flex' }}>
          <CardContent sx={{ flex: 1 }}>
            <RankingCardHeader
              title="Топ навыков"
              subtitle={
                <Typography variant="body2" color="text.secondary">
                  Частота в вакансиях за период
                </Typography>
              }
            />
            <Box
              className="dashboard-ranking-card__controls-spacer"
              aria-hidden="true"
              sx={{
                display: { xs: 'none', lg: 'block' },
                height: (theme) => theme.spacing(7),
              }}
            />
            {skills.isPending && (
              <Stack spacing={2} sx={{ mt: 3 }}>
                {[1, 2, 3, 4, 5].map((item) => (
                  <Skeleton key={item} height={28} />
                ))}
              </Stack>
            )}
            {skills.isError && skillItems.length === 0 && (
              <Box sx={{ mt: 2 }}>
                <ErrorState error={skills.error} onRetry={() => skills.refetch()} compact />
              </Box>
            )}
            {skills.isSuccess && skillItems.length === 0 && <EmptyState />}
            <Stack spacing={2.1} sx={{ mt: 3 }}>
              {skillItems.map((skill) => (
                <Box key={skill.skill_id || skill.name}>
                  <Stack direction="row" sx={{ justifyContent: 'space-between', mb: 0.6 }}>
                    <Typography variant="body2" sx={{ fontWeight: 650 }}>
                      {skill.name}
                    </Typography>
                    <Typography variant="caption" color="text.secondary">
                      {formatCompact(skill.count)} · {Math.round(skill.share * 100)}%
                    </Typography>
                  </Stack>
                  <LinearProgress
                    variant="determinate"
                    value={(skill.count / maxSkillCount) * 100}
                    sx={{ height: 6, borderRadius: 3 }}
                  />
                </Box>
              ))}
            </Stack>
            {skillItems.length > 0 && (
              <Stack
                direction={{ xs: 'column', sm: 'row' }}
                sx={{ mt: 2.5, gap: 1, alignItems: { sm: 'center' } }}
              >
                {(skills.hasNextPage || skills.isFetchNextPageError) && (
                  <Button
                    variant="outlined"
                    onClick={() => void skills.fetchNextPage()}
                    disabled={skills.isFetchingNextPage}
                    startIcon={
                      skills.isFetchingNextPage ? <CircularProgress size={16} /> : undefined
                    }
                    aria-label={
                      skills.isFetchNextPageError
                        ? 'Повторить загрузку навыков'
                        : 'Показать ещё навыки'
                    }
                  >
                    {skills.isFetchNextPageError ? 'Повторить' : 'Показать ещё'}
                  </Button>
                )}
                <Typography
                  variant="caption"
                  color={skills.isFetchNextPageError ? 'error' : 'text.secondary'}
                  aria-live="polite"
                >
                  {skills.isFetchNextPageError
                    ? `Не удалось загрузить ещё. Показано ${skillItems.length} из ${skillTotal}`
                    : `Показано ${skillItems.length} из ${skillTotal}`}
                </Typography>
              </Stack>
            )}
          </CardContent>
        </Card>
        <RankingCard kind="languages" title="Языки программирования" params={params} />
        <RankingCard kind="managerial" title="Управленческие IT-роли" params={params} />
      </Box>

      <Card variant="outlined">
        <CardContent>
          <Stack
            direction="row"
            sx={{ justifyContent: 'space-between', alignItems: 'center' }}
          >
            <Box>
              <Typography variant="h6">Предлагаемые зарплаты</Typography>
              <Typography variant="body2" color="text.secondary">
                Медиана и диапазон P25–P75, {currency} net
              </Typography>
            </Box>
            <Chip size="small" label="offered salary" variant="outlined" />
          </Stack>
          {salaries.isLoading && <Skeleton variant="rounded" height={280} sx={{ mt: 2 }} />}
          {salaries.isError && (
            <Box sx={{ mt: 2 }}>
              <ErrorState error={salaries.error} onRetry={() => salaries.refetch()} compact />
            </Box>
          )}
          {salaries.isSuccess && salaryPoints.length === 0 && <EmptyState />}
          {salaryPoints.length > 0 && (
            <LineChart
              height={300}
              xAxis={[
                {
                  data: salaryPoints.map((point) => new Date(point.period_start)),
                  scaleType: 'time',
                  valueFormatter: (value: Date) =>
                    value.toLocaleDateString('ru-RU', { day: 'numeric', month: 'short' }),
                },
              ]}
              yAxis={[{ valueFormatter: (value: number) => formatCompact(value) }]}
              series={[
                {
                  data: salaryPoints.map((point) => point.p25),
                  label: 'P25',
                  color: theme.palette.chart.tertiary,
                },
                {
                  data: salaryPoints.map((point) => point.median),
                  label: 'Медиана',
                  color: theme.palette.chart.primary,
                },
                {
                  data: salaryPoints.map((point) => point.p75),
                  label: 'P75',
                  color: theme.palette.chart.secondary,
                },
              ]}
              grid={{ horizontal: true }}
            />
          )}
          <Typography variant="caption" color="text.secondary">
            Оценка по зарплатным полям вакансий, не зарплатный опрос.
          </Typography>
        </CardContent>
      </Card>
    </Stack>
  )
}
