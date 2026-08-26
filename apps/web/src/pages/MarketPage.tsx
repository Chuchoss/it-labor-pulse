import InsightsRoundedIcon from '@mui/icons-material/InsightsRounded'
import OpenInNewRoundedIcon from '@mui/icons-material/OpenInNewRounded'
import { LineChart } from '@mui/x-charts/LineChart'
import { useQuery } from '@tanstack/react-query'
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  FormControl,
  InputLabel,
  MenuItem,
  Select,
  Skeleton,
  Stack,
  Typography,
} from '@mui/material'
import { useState } from 'react'
import { api } from '../api/client'
import { ErrorState } from '../components/DataState'

const roleGroups = [
  { value: 'software_development', label: 'Разработка и руководители' },
  { value: 'analytics', label: 'Аналитика' },
  { value: 'quality_assurance', label: 'Тестирование и QA' },
]

function formatDate(value: string | null | undefined) {
  if (!value) return '—'
  return new Date(`${value}T00:00:00Z`).toLocaleDateString('ru-RU')
}

export function MarketPage() {
  const [roleGroup, setRoleGroup] = useState('software_development')
  const [selectedYear, setSelectedYear] = useState<number>()
  const [regionID, setRegionID] = useState('')
  const [grain, setGrain] = useState<'day' | 'week'>('day')
  const coverage = useQuery({
    queryKey: ['market-coverage'],
    queryFn: ({ signal }) => api.marketCoverage(signal),
  })

  const years = coverage.data?.available_years ?? []
  const year = selectedYear && years.includes(selectedYear) ? selectedYear : years.at(-1)
  const from = year ? `${year}-01-01` : ''
  const to = year ? `${year}-12-31` : ''
  const demand = useQuery({
    queryKey: ['market-demand', { from, to, roleGroup, regionID, grain }],
    queryFn: ({ signal }) =>
      api.marketDemand(
        {
          from,
          to,
          role_group: roleGroup,
          region_id: regionID || undefined,
          grain,
        },
        signal,
      ),
    enabled: ['ready', 'degraded'].includes(coverage.data?.status ?? '') && Boolean(year),
  })
  const points = demand.data?.points ?? []
  const canUseWeek = (coverage.data?.complete_weekly_count ?? 0) > 0

  return (
    <Stack spacing={3}>
      <Box>
        <Typography variant="h4" component="h1" sx={{ fontWeight: 800 }}>
          Рынок
        </Typography>
        <Typography color="text.secondary" sx={{ mt: 0.5 }}>
          Снимки спроса по завершённым полным циклам HeadHunter
        </Typography>
      </Box>

      {coverage.isLoading && <Skeleton variant="rounded" height={180} />}
      {coverage.isError && (
        <ErrorState error={coverage.error} onRetry={() => coverage.refetch()} />
      )}

      {coverage.data?.status === 'collecting' && (
        <Alert
          severity="info"
          icon={<InsightsRoundedIcon />}
          action={
            <Button
              component="a"
              href="https://stats.hh.ru/"
              target="_blank"
              rel="noreferrer"
              endIcon={<OpenInNewRoundedIcon />}
              color="inherit"
            >
              Статистика HH
            </Button>
          }
        >
          Полный all-IT цикл ещё не завершён. Планировщик продолжает сбор; график появится
          только после первого достоверного снимка. История за прошлые годы не
          реконструируется.
        </Alert>
      )}

      {coverage.data?.status === 'degraded' && (
        <Alert severity="warning">
          Покрытие неполное: пропущено дней — {coverage.data.missed_daily_count}, незавершено —
          {' '}{coverage.data.incomplete_daily_count}
          {coverage.data.latest_incomplete_date
            ? ` (последний: ${formatDate(coverage.data.latest_incomplete_date)})`
            : ''}. Незавершённые дни не публикуются.
        </Alert>
      )}

      {(coverage.data?.status === 'ready' || coverage.data?.status === 'degraded') && (
        <>
          <Card variant="outlined">
            <CardContent>
              <Stack direction={{ xs: 'column', md: 'row' }} spacing={2}>
                <FormControl size="small" sx={{ minWidth: 250 }}>
                  <InputLabel id="market-direction-label">Направление</InputLabel>
                  <Select
                    labelId="market-direction-label"
                    label="Направление"
                    value={roleGroup}
                    onChange={(event) => setRoleGroup(event.target.value)}
                  >
                    {roleGroups.map((group) => (
                      <MenuItem key={group.value} value={group.value}>
                        {group.label}
                      </MenuItem>
                    ))}
                  </Select>
                </FormControl>
                <FormControl size="small" sx={{ minWidth: 130 }}>
                  <InputLabel id="market-year-label">Год</InputLabel>
                  <Select
                    labelId="market-year-label"
                    label="Год"
                    value={year ?? ''}
                    onChange={(event) => setSelectedYear(Number(event.target.value))}
                  >
                    {years.map((availableYear) => (
                      <MenuItem key={availableYear} value={availableYear}>
                        {availableYear}
                      </MenuItem>
                    ))}
                  </Select>
                </FormControl>
                <FormControl size="small" sx={{ minWidth: 220 }}>
                  <InputLabel id="market-region-label">Регион</InputLabel>
                  <Select
                    labelId="market-region-label"
                    label="Регион"
                    value={regionID}
                    onChange={(event) => setRegionID(event.target.value)}
                  >
                    <MenuItem value="">Вся Россия</MenuItem>
                    {coverage.data.regions.map((region) => (
                      <MenuItem key={region.region_id} value={region.region_id}>
                        {region.name}
                      </MenuItem>
                    ))}
                  </Select>
                </FormControl>
                <FormControl size="small" sx={{ minWidth: 150 }}>
                  <InputLabel id="market-grain-label">Шаг</InputLabel>
                  <Select
                    labelId="market-grain-label"
                    label="Шаг"
                    value={grain}
                    onChange={(event) => setGrain(event.target.value as 'day' | 'week')}
                  >
                    <MenuItem value="day">День</MenuItem>
                    <MenuItem value="week" disabled={!canUseWeek}>
                      Неделя {!canUseWeek && '— нет полной недели'}
                    </MenuItem>
                  </Select>
                </FormControl>
              </Stack>
            </CardContent>
          </Card>

          <Card variant="outlined">
            <CardContent>
              <Typography variant="h6">Покрытие данных</Typography>
              <Box
                sx={{
                  display: 'grid',
                  gridTemplateColumns: { xs: '1fr', sm: 'repeat(2, 1fr)', lg: 'repeat(4, 1fr)' },
                  gap: 2,
                  mt: 2,
                }}
              >
                <CoverageItem label="Источник" value="HeadHunter (официальный API)" />
                <CoverageItem
                  label="Наблюдения"
                  value={`${formatDate(coverage.data.first_observation)} — ${formatDate(coverage.data.last_observation)}`}
                />
                <CoverageItem
                  label="Публикации"
                  value={`${formatDate(coverage.data.publication_from)} — ${formatDate(coverage.data.publication_to)}`}
                />
                <CoverageItem
                  label="Полные снимки"
                  value={`${coverage.data.complete_daily_count} из ${coverage.data.expected_daily_count} дней · ${coverage.data.complete_weekly_count} недель`}
                />
                <CoverageItem
                  label="Следующий discovery"
                  value={new Date(coverage.data.next_scheduled_cycle).toLocaleString('ru-RU')}
                />
                <CoverageItem
                  label="Методика"
                  value={coverage.data.method_version || '—'}
                />
                <CoverageItem
                  label="Последний полный цикл"
                  value={
                    coverage.data.latest_complete_cycle
                      ? new Date(coverage.data.latest_complete_cycle).toLocaleString('ru-RU')
                      : '—'
                  }
                />
              </Box>
              <Typography variant="body2" color="text.secondary" sx={{ mt: 2 }}>
                Активные — дедуплицированные вакансии из полного дневного search-discovery.
                Опубликованные — вакансии с <code>published_at</code> в UTC-окне снимка.
                Навыки и полные карточки обновляются отдельно фоновой hydration.
              </Typography>
            </CardContent>
          </Card>

          <Card variant="outlined">
            <CardContent>
              <Typography variant="h6">Спрос на рынке</Typography>
              <Typography variant="body2" color="text.secondary">
                Исторический active_count хранится в снимках и не восстанавливается из
                текущих вакансий
              </Typography>
              {demand.isLoading && <Skeleton variant="rounded" height={340} sx={{ mt: 2 }} />}
              {demand.isError && (
                <Box sx={{ mt: 2 }}>
                  <ErrorState error={demand.error} onRetry={() => demand.refetch()} compact />
                </Box>
              )}
              {demand.isSuccess && points.length === 0 && (
                <Alert severity="info" sx={{ mt: 2 }}>
                  За выбранный период нет полных {grain === 'week' ? 'недельных' : 'дневных'} снимков.
                </Alert>
              )}
              {points.length > 0 && (
                <LineChart
                  height={360}
                  xAxis={[
                    {
                      data: points.map((point) => new Date(`${point.period_start}T00:00:00Z`)),
                      scaleType: 'time',
                      valueFormatter: (value: Date) =>
                        value.toLocaleDateString('ru-RU', { day: 'numeric', month: 'short' }),
                    },
                  ]}
                  series={[
                    {
                      data: points.map((point) => point.active_count),
                      label: 'Активные',
                      color: '#4f46e5',
                    },
                    {
                      data: points.map((point) => point.published_count),
                      label: 'Опубликованные',
                      color: '#0f766e',
                    },
                  ]}
                  grid={{ horizontal: true }}
                />
              )}
            </CardContent>
          </Card>
        </>
      )}
    </Stack>
  )
}

function CoverageItem({ label, value }: { label: string; value: string }) {
  return (
    <Box>
      <Typography variant="caption" color="text.secondary">
        {label}
      </Typography>
      <Typography variant="body2" sx={{ fontWeight: 650 }}>
        {value}
      </Typography>
    </Box>
  )
}
