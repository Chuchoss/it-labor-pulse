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
  TablePagination,
  TableRow,
  TextField,
  Typography,
  FormControlLabel,
  Button,
} from '@mui/material'
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { api } from '../api/client'
import type { Vacancy } from '../api/types'
import { EmptyState, ErrorState } from '../components/DataState'
import { formatDate, formatSalaryRange } from '../utils/format'

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

export function VacanciesPage() {
  const [searchParams, setSearchParams] = useSearchParams()
  const query = searchParams.get('q') || ''
  const source = searchParams.get('source') || ''
  const onlyActive = searchParams.get('only_active') !== 'false'
  const page = Math.max(Number(searchParams.get('page')) || 1, 1)
  const pageSize = Math.min(Math.max(Number(searchParams.get('page_size')) || 20, 1), 100)
  const [draftQuery, setDraftQuery] = useState(query)

  const vacancies = useQuery({
    queryKey: ['vacancies', { query, source, onlyActive, page, pageSize }],
    queryFn: ({ signal }) =>
      api.vacancies(
        {
          q: query || undefined,
          source: source || undefined,
          only_active: onlyActive,
          page,
          page_size: pageSize,
        },
        signal,
      ),
  })

  const updateParams = (updates: Record<string, string | undefined>) => {
    const next = new URLSearchParams(searchParams)
    Object.entries(updates).forEach(([key, value]) => {
      if (!value) next.delete(key)
      else next.set(key, value)
    })
    setSearchParams(next)
  }

  const rows = vacancies.data?.data ?? []

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
              updateParams({ q: draftQuery.trim() || undefined, page: '1' })
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
                  updateParams({ source: event.target.value || undefined, page: '1' })
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
                    updateParams({ only_active: checked ? undefined : 'false', page: '1' })
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

      {vacancies.isError && (
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
                    <TableCell>{vacancy.region_id || 'Не указан'}</TableCell>
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

          <TablePagination
            component="div"
            count={vacancies.data?.total ?? 0}
            page={page - 1}
            onPageChange={(_, nextPage) => updateParams({ page: String(nextPage + 1) })}
            rowsPerPage={pageSize}
            onRowsPerPageChange={(event) =>
              updateParams({ page_size: event.target.value, page: '1' })
            }
            rowsPerPageOptions={[10, 20, 50]}
            labelRowsPerPage="На странице:"
            labelDisplayedRows={({ from: first, to: last, count }) =>
              `${first}–${last} из ${count}`
            }
          />
        </Card>
      )}
    </Stack>
  )
}
