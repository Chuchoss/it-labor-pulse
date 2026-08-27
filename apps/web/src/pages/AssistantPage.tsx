import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Alert,
  Autocomplete,
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  CircularProgress,
  Divider,
  FormControl,
  FormControlLabel,
  FormLabel,
  MenuItem,
  Radio,
  RadioGroup,
  Stack,
  Switch,
  TextField,
  Typography,
} from '@mui/material'
import { useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, api } from '../api/client'

const approvedRoleOptions = [
  { id: '96', label: 'Разработчик', group: 'Разработка' },
  { id: '104', label: 'Руководитель группы разработки', group: 'Разработка' },
  { id: '148', label: 'Системный аналитик', group: 'Аналитика' },
  { id: '150', label: 'Бизнес-аналитик', group: 'Аналитика' },
  { id: '156', label: 'BI-аналитик / аналитик данных', group: 'Аналитика' },
  { id: '164', label: 'Продуктовый аналитик', group: 'Аналитика' },
  { id: '124', label: 'Тестировщик', group: 'Контроль качества' },
] as const

const specializationOptions = [
  { id: 'frontend', label: 'Frontend' },
  { id: 'backend', label: 'Backend' },
  { id: 'fullstack', label: 'Fullstack' },
  { id: 'mobile', label: 'Mobile' },
  { id: 'devops_platform', label: 'DevOps / Platform' },
  { id: 'data_ml', label: 'Data / ML' },
  { id: 'other', label: 'Другое' },
] as const

const legacyRoleAliases: Record<string, string[]> = {
  backend: ['96'], 'backend developer': ['96'], frontend: ['96'], 'frontend developer': ['96'],
  fullstack: ['96'], 'full stack': ['96'], 'fullstack developer': ['96'], developer: ['96'],
  programmer: ['96'], 'software developer': ['96'], 'team lead': ['104'], teamlead: ['104'],
  'lead developer': ['104'], qa: ['124'], 'qa engineer': ['124'], tester: ['124'],
  'quality assurance': ['124'], 'system analyst': ['148'], 'systems analyst': ['148'],
  'business analyst': ['150'], 'bi analyst': ['156'], 'data analyst': ['156'], 'product analyst': ['164'],
}

const aiSkipReasonText: Record<string, string> = {
  server_disabled: 'AI-анализ не запускался: выключен на сервере.',
  user_opt_out: 'AI-анализ не запускался: не включён пользователем.',
  run_predates_ai: 'AI-анализ не запускался: запуск создан до включения AI.',
  preferences_changed: 'AI-анализ не запускался: критерии были обновлены.',
  no_eligible: 'AI-анализ не запускался: нет вакансий для AI.',
  budget_exhausted: 'Старый запуск остановлен историческим лимитом вызовов; новые запуски выполняются без лимита количества.',
  already_analyzed: 'AI-анализ не запускался: вакансии уже были обработаны AI.',
  provider_unavailable: 'AI-анализ не запускался: worker не получил разрешение на внешний провайдер.',
  unknown: 'AI-анализ не выполнялся; причина недоступна для старого запуска.',
}

const workerStateText: Record<string, string> = {
  idle: 'ожидает новые вакансии',
  processing: 'обрабатывает вакансии',
  backoff: 'повторит после временной ошибки',
  stopping: 'останавливается',
  offline: 'недоступен',
}

function isManagementLeadershipTitle(title: string) {
  return /team[\s-]?lead|tech(?:nical)?[\s-]?lead|\blead[\s-]+(?:developer|engineer|front)|тим[\s-]?лид|тех[\s-]?лид|руководител|head[\s-]?of|\bcto\b|директор/i.test(title)
}

function normalizeLegacyAlias(value: string) {
  const key = value.trim().toLowerCase().replaceAll('_', ' ').replaceAll('-', ' ').replace(/\s+/g, ' ')
  return legacyRoleAliases[key]
}

function stringList(value: unknown) {
  if (!Array.isArray(value)) return []
  return [...new Set(value
    .filter((item): item is string => typeof item === 'string')
    .map((item) => item.trim())
    .filter(Boolean))]
}

function metric(value: number | undefined) {
  return value === undefined ? '—' : String(value)
}

function TagsField({
  label,
  placeholder,
  value,
  onChange,
}: {
  label: string
  placeholder: string
  value: string[]
  onChange: (value: string[]) => void
}) {
  return (
    <Autocomplete
      multiple
      freeSolo
      options={[] as string[]}
      value={value}
      onChange={(_, values) => onChange(stringList(values))}
      renderInput={(params) => (
        <TextField
          {...params}
          label={label}
          placeholder={value.length === 0 ? placeholder : undefined}
          helperText="Введите значение и нажмите Enter"
        />
      )}
    />
  )
}

function loadedRoleState(hard: Record<string, unknown>, upgraded = false) {
  const approved = Array.isArray(hard.approved_roles)
    ? hard.approved_roles.filter((value): value is string => typeof value === 'string')
    : []
  const legacy = typeof hard.role === 'string' ? normalizeLegacyAlias(hard.role) : undefined
  const knownIDs = new Set<string>(approvedRoleOptions.map((role) => role.id))
  return {
    ids: [...new Set([...approved, ...(legacy ?? [])])].filter((id) => knownIDs.has(id)),
    legacy: hard.role !== undefined ? (legacy ? 'mapped' : 'unknown') : (upgraded ? 'mapped' : null),
  } as const
}

export function AssistantPage() {
  const client = useQueryClient()
  const preferences = useQuery({ queryKey: ['assistant-preferences'], queryFn: api.assistantPreferences })
  const preferenceList = useQuery({ queryKey: ['assistant-preference-list'], queryFn: api.assistantPreferenceList })
  const status = useQuery({
    queryKey: ['assistant-status'],
    queryFn: api.assistantStatus,
    refetchInterval: import.meta.env.MODE === 'test' ? 50 : 10_000,
  })
  const analysisPreview = useQuery({ queryKey: ['assistant-analysis-preview'], queryFn: api.assistantAnalysisPreview })
  const matches = useQuery({ queryKey: ['assistant-matches'], queryFn: api.assistantMatches })
  const telegram = useQuery({ queryKey: ['telegram-status'], queryFn: api.telegramStatus })
  const automation = useQuery({ queryKey: ['assistant-automation'], queryFn: api.assistantAutomation })
  const [note, setNote] = useState<string>()
  const [approvedRoleIDs, setApprovedRoleIDs] = useState<string[]>()
  const [regions, setRegions] = useState<string[]>()
  const [requiredSkills, setRequiredSkills] = useState<string[]>()
  const [excludedSkills, setExcludedSkills] = useState<string[]>()
  const [remoteOnly, setRemoteOnly] = useState<boolean>()
  const [specialization, setSpecialization] = useState<string>()
  const [includeLeadership, setIncludeLeadership] = useState<boolean>()
  const [minSalaryRUB, setMinSalaryRUB] = useState<string>()
  const [legacyRoleResolved, setLegacyRoleResolved] = useState(false)
  const [confirmed, setConfirmed] = useState(false)
  const [confirmAction, setConfirmAction] = useState<'archive' | 'ai' | 'telegram' | 'test' | null>(null)
  const [submittedRunID, setSubmittedRunID] = useState<string>()
  const [runPrerequisiteMissing, setRunPrerequisiteMissing] = useState(false)
  const runSubmissionRef = useRef(false)
  const statusSectionRef = useRef<HTMLDivElement>(null)
  const criteriaSectionRef = useRef<HTMLDivElement>(null)
  const noteValue = note ?? preferences.data?.note ?? ''
  const hardCriteriaValue = preferences.data?.hard_criteria ?? {}
  const softCriteriaValue = preferences.data?.soft_criteria ?? {}
  const weightsValue = preferences.data?.weights ?? {}
  const loadedRoles = loadedRoleState(hardCriteriaValue, preferences.data?.legacy_role_upgraded)
  const approvedRoleIDsValue = approvedRoleIDs ?? loadedRoles.ids
  const regionsValue = regions ?? stringList(hardCriteriaValue.regions)
  const requiredSkillsValue = requiredSkills ?? stringList(hardCriteriaValue.required_skills)
  const excludedSkillsValue = excludedSkills ?? stringList(hardCriteriaValue.excluded_skills)
  const remoteOnlyValue = remoteOnly ?? hardCriteriaValue.remote_only === true
  const savedSpecialization = typeof hardCriteriaValue.specialization === 'string'
    ? hardCriteriaValue.specialization
    : ''
  const specializationValue = specialization ?? savedSpecialization
  const includeLeadershipValue = includeLeadership ?? hardCriteriaValue.include_leadership === true
  const developerSelected = approvedRoleIDsValue.includes('96')
  const savedSalary = typeof hardCriteriaValue.min_salary_rub === 'number' && Number.isFinite(hardCriteriaValue.min_salary_rub)
    ? String(hardCriteriaValue.min_salary_rub)
    : ''
  const minSalaryRUBValue = minSalaryRUB ?? savedSalary
  const parsedSalary = Number(minSalaryRUBValue)
  const salaryError = minSalaryRUBValue.trim() !== '' && (!Number.isFinite(parsedSalary) || parsedSalary < 0)
  const legacyRoleState = legacyRoleResolved ? 'mapped' : loadedRoles.legacy
  const save = useMutation({
    mutationFn: () => {
      if (legacyRoleState === 'unknown') throw new Error('Неизвестная устаревшая роль: выберите утверждённую роль')
      if (salaryError) throw new Error('Минимальная зарплата должна быть не меньше 0')
      const hard: Record<string, unknown> = {
        approved_roles: approvedRoleIDsValue,
        remote_only: remoteOnlyValue,
        include_leadership: includeLeadershipValue,
      }
      if (developerSelected && specializationValue) hard.specialization = specializationValue
      if (regionsValue.length > 0) hard.regions = regionsValue
      if (requiredSkillsValue.length > 0) hard.required_skills = requiredSkillsValue
      if (excludedSkillsValue.length > 0) hard.excluded_skills = excludedSkillsValue
      if (minSalaryRUBValue.trim() !== '') hard.min_salary_rub = parsedSalary
      return api.saveAssistantPreferences({
        note: noteValue,
        hard_criteria: hard,
        soft_criteria: softCriteriaValue,
        weights: weightsValue,
      })
    },
    onSuccess: async () => {
      setConfirmed(true)
      setRunPrerequisiteMissing(false)
      setLegacyRoleResolved(false)
      await client.refetchQueries({ queryKey: ['assistant-preferences'] })
      await client.refetchQueries({ queryKey: ['assistant-preference-list'] })
      await client.invalidateQueries({ queryKey: ['assistant-status'] })
      await client.invalidateQueries({ queryKey: ['assistant-matches'] })
    },
    onMutate: () => setConfirmed(false),
  })
  const link = useMutation({ mutationFn: api.telegramLink })
  const revoke = useMutation({
    mutationFn: api.revokeTelegram,
    onSuccess: () => void client.invalidateQueries({ queryKey: ['telegram-status'] }),
  })
  const archive = useMutation({ mutationFn: () => api.archiveAssistantPreference(preferences.data?.id ?? ''), onSuccess: () => {
    setConfirmAction(null)
    void client.invalidateQueries({ queryKey: ['assistant-preferences'] })
    void client.invalidateQueries({ queryKey: ['assistant-preference-list'] })
  } })
  const run = useMutation({
    mutationFn: api.runAssistantAnalysis,
    onMutate: () => {
      client.setQueryData<import('../api/types').AssistantStatus>(['assistant-status'], (current) => ({
        ai_configured: current?.ai_configured ?? false,
        ai_status: automation.data?.ai_enabled && (current?.ai_configured ?? false) ? 'pending' : 'skipped',
        state: 'queued',
        last_checked_at: new Date().toISOString(),
        processed: 0,
        total: analysisPreview.data?.snapshot_total ?? 0,
        eligible: 0,
        matched: 0,
        ai_calls: 0,
        ai_eligible: 0,
        ai_succeeded: 0,
        ai_matches: 0,
        ai_failures: 0,
        ai_skipped: 0,
        skipped: 0,
        pending_candidates: false,
        worker_offline: false,
        worker_stalled: false,
      }))
    },
    onSuccess: (result) => {
      setSubmittedRunID(result.run_id)
      setRunPrerequisiteMissing(false)
      client.setQueryData<import('../api/types').AssistantStatus>(['assistant-status'], (current) => (
        current ? { ...current, run_id: result.run_id } : current
      ))
      void status.refetch()
      void client.invalidateQueries({ queryKey: ['assistant-matches'] })
    },
    onError: (error) => {
      void status.refetch().then(() => {
        if (error instanceof ApiError && error.status === 409) focusSection(statusSectionRef.current)
      })
    },
    onSettled: () => {
      runSubmissionRef.current = false
    },
  })
  const supersedeRun = useMutation({
    mutationFn: (runID: string) => api.supersedeAssistantAnalysis(runID),
    onSuccess: () => {
      setSubmittedRunID(undefined)
      void status.refetch()
    },
  })
  const focusSection = (section: HTMLDivElement | null) => {
    if (typeof section?.scrollIntoView === 'function') {
      section.scrollIntoView({ behavior: 'smooth', block: 'center' })
    }
    section?.focus({ preventScroll: true })
  }
  const startRun = () => {
    if (status.data?.state === 'queued' || status.data?.state === 'running' || status.data?.state === 'paused') {
      focusSection(statusSectionRef.current)
      return
    }
    if (!preferences.data?.id) {
      setRunPrerequisiteMissing(true)
      focusSection(criteriaSectionRef.current)
      return
    }
    if (runSubmissionRef.current || run.isPending) return
    setRunPrerequisiteMissing(false)
    runSubmissionRef.current = true
    run.mutate()
  }
  const updateAutomation = useMutation({
    mutationFn: (value: { ai_enabled?: boolean; telegram_enabled?: boolean }) => api.updateAssistantAutomation(value),
    onSuccess: () => void client.invalidateQueries({ queryKey: ['assistant-automation'] }),
  })
  const updateTelegramOptIn = useMutation({
    mutationFn: (value: boolean) => api.updateTelegramOptIn(value),
    onSuccess: () => void client.invalidateQueries({ queryKey: ['telegram-status'] }),
  })
  const testTelegram = useMutation({ mutationFn: api.testTelegram, onSuccess: () => void telegram.refetch() })
  const saveError = save.error instanceof ApiError && (save.error.code || save.error.requestId)
    ? `${save.error.message} (${save.error.code ?? 'API_ERROR'}${save.error.requestId ? `, request_id: ${save.error.requestId}` : ''})`
    : save.error?.message
  const activeRunID = submittedRunID ?? status.data?.run_id
  const aiCalls = status.data?.ai_calls
  const aiPending = Math.max(
    0,
    (status.data?.ai_eligible ?? 0) - (status.data?.ai_succeeded ?? 0) - (status.data?.ai_failures ?? 0) - (status.data?.ai_skipped ?? 0),
  )
  const aiBatchCount = status.data?.ai_batches ?? status.data?.ai_http_attempts
  const averageBatch = aiCalls !== undefined && aiBatchCount !== undefined && aiBatchCount > 0
    ? (aiCalls / aiBatchCount).toFixed(1)
    : '—'
  const elapsedMinutes = status.data?.started_at && status.data?.last_checked_at
    ? Math.max((new Date(status.data.last_checked_at).getTime() - new Date(status.data.started_at).getTime()) / 60000, 1 / 60)
    : 0
  const throughput = elapsedMinutes > 0 ? (status.data?.processed ?? 0) / elapsedMinutes : 0
  const remainingMinutes = throughput > 0
    ? Math.ceil(Math.max(0, (status.data?.total ?? 0) - (status.data?.processed ?? 0)) / throughput)
    : null
  const aiFailureSummary = [
    ['ограничение скорости', status.data?.ai_rate_limit],
    ['таймауты', status.data?.ai_timeouts],
    ['некорректный ответ', status.data?.ai_invalid_responses],
    ['авторизация', status.data?.ai_auth],
    ['баланс/квота', status.data?.ai_quota],
    ['ошибки DeepSeek', status.data?.ai_server],
    ['сеть', status.data?.ai_network],
    ['лимит контекста', status.data?.ai_context_limit],
    ['фильтрация контента', status.data?.ai_content_filter],
    ['параметры запроса', status.data?.ai_invalid_request],
  ].filter((item): item is [string, number] => typeof item[1] === 'number' && item[1] > 0)

  return (
    <Stack spacing={3}>
      <div>
        <Typography variant="h4" component="h1" sx={{ fontWeight: 800 }}>Мои вакансии</Typography>
        <Typography color="text.secondary">Локальный помощник: сначала прозрачные критерии, затем опциональный AI.</Typography>
      </div>
      <Alert severity="info">Режим разработки. DeepSeek и Telegram выключены по умолчанию; ключи не вводятся в браузере.</Alert>
      <Card variant="outlined"><CardContent>
        <Stack spacing={1}>
          <Typography variant="h6">Автоматизация</Typography>
          <Typography variant="body2" color="text.secondary">
            Новые вакансии будут автоматически анализироваться AI по полному описанию. Вакансии отправляются
            небольшими пакетами для снижения повторяющегося контекста; общее количество вакансий не ограничено.
            Telegram включается отдельно.
          </Typography>
          <Stack direction="row" sx={{ alignItems: 'center' }}>
            <Switch checked={automation.data?.ai_enabled ?? false} disabled={!status.data?.ai_configured || updateAutomation.isPending}
              onChange={() => setConfirmAction('ai')} slotProps={{ input: { 'aria-label': 'Автоматический AI-анализ' } }} />
            <Typography>Автоматический AI-анализ</Typography>
          </Stack>
          <Stack direction="row" sx={{ alignItems: 'center' }}>
            <Switch checked={automation.data?.telegram_enabled ?? false} disabled={!telegram.data?.configured || updateAutomation.isPending}
              onChange={() => setConfirmAction('telegram')} slotProps={{ input: { 'aria-label': 'Отправлять совпадения в Telegram' } }} />
            <Typography>Отправлять совпадения в Telegram</Typography>
          </Stack>
          <Typography variant="caption" color="text.secondary">
            {automation.data?.activation_at ? `Активировано: ${new Date(automation.data.activation_at).toLocaleString('ru-RU')}` : 'Автоматический режим выключен'}
          </Typography>
        </Stack>
      </CardContent></Card>
      <Card variant="outlined"><CardContent>
        <Stack ref={statusSectionRef} tabIndex={-1} spacing={2} aria-live="polite" aria-busy={run.isPending || status.data?.state === 'queued' || status.data?.state === 'running'}>
          <Typography variant="h6">Статус анализа</Typography>
          <Stack direction="row" spacing={1}>
            <Chip label={status.data?.state === 'unknown' ? 'неизвестно' : status.data?.state ?? 'загрузка'} color={status.data?.state === 'succeeded' ? 'success' : status.data?.state === 'failed' ? 'error' : 'default'} />
            <Button size="small" onClick={() => void status.refetch()}>Обновить</Button>
          </Stack>
          {(run.isPending || status.data?.state === 'queued') && <Stack direction="row" spacing={1} sx={{ alignItems: 'center' }}>
            <CircularProgress size={18} aria-hidden="true" />
            <Typography>Подготавливаем список вакансий…</Typography>
          </Stack>}
          {status.data?.state === 'running' && <Typography>Анализируем все текущие вакансии…</Typography>}
          {status.data?.worker_offline && (
            <Alert severity="error">Сервис обработки недоступен: heartbeat просрочен.</Alert>
          )}
          {!status.data?.worker_offline && status.data?.worker_stalled && (
            <Alert severity="warning">Worker доступен, но прогресс пакетов задерживается.</Alert>
          )}
          {status.data?.worker_phase === 'backoff' && (
            <Alert severity="info">
              Временная ошибка провайдера: повтор
              {status.data.worker_retry_until
                ? ` после ${new Date(status.data.worker_retry_until).toLocaleTimeString('ru-RU')}`
                : ' с безопасной задержкой'}.
            </Alert>
          )}
          <Typography variant="body2" color="text.secondary">
            Сервис обработки: {status.data?.worker_offline
              ? workerStateText.offline
              : workerStateText[status.data?.worker_state ?? 'idle'] ?? 'состояние неизвестно'}.
          </Typography>
          <Typography>
            Проверено по критериям: {metric(status.data?.processed)} из {metric(status.data?.total)}
            {' · '}Предварительно подходят: {metric(status.data?.matched)}
            {' · '}Ожидают AI: {aiPending}
            {' · '}Подтверждено AI: {aiCalls === 0 ? '—' : status.data?.state === 'running' && status.data?.ai_matches === 0 ? 'пока 0' : metric(status.data?.ai_matches)}
          </Typography>
          {(status.data?.state === 'running' || status.data?.state === 'paused') && (
            <Typography variant="body2" color="text.secondary">
              Скорость: {throughput > 0 ? throughput.toFixed(1) : '—'} вакансий/мин
              {' · '}Активные пакеты: {status.data?.worker_active_batches ?? 0} из {status.data?.worker_concurrency ?? '—'}
              {' · '}Осталось: {status.data?.state === 'paused' ? 'приостановлено' : remainingMinutes === null ? 'считаем' : `около ${remainingMinutes} мин`}
            </Typography>
          )}
          <Typography>
            {aiCalls === undefined
              ? 'AI-анализ: телеметрия недоступна · Совпадения: —'
              : aiCalls === 0
              ? 'AI-анализ: не выполнялся · Совпадения: —'
              : `AI проверено: ${metric(status.data?.ai_succeeded)} вакансий · match: ${metric(status.data?.ai_matches)} · review: ${metric(status.data?.ai_reviews)} · reject: ${metric(status.data?.ai_rejects)} · DeepSeek HTTP-запросов: ${metric(status.data?.ai_http_attempts)} · Средний пакет: ${averageBatch} · Повторы: ${metric(status.data?.ai_retries)} · Ошибки: ${metric(status.data?.ai_failures)}`}
            {aiCalls === 0 && ` · Ошибки: ${metric(status.data?.ai_failures)}`}
            {' · '}Пропущено AI: {metric(status.data?.ai_skipped)}
          </Typography>
          {(status.data?.ai_prompt_tokens ?? 0) + (status.data?.ai_completion_tokens ?? 0) > 0 && (
            <Typography variant="body2" color="text.secondary">
              Токены: вход {status.data?.ai_prompt_tokens ?? 0} · выход {status.data?.ai_completion_tokens ?? 0}
              {(status.data?.ai_cached_tokens ?? 0) > 0 ? ` · из кэша ${status.data?.ai_cached_tokens}` : ''}
            </Typography>
          )}
          {aiFailureSummary.length > 0 && (
            <Alert severity="warning">
              Категории финальных ошибок: {aiFailureSummary.map(([label, count]) => `${label} — ${count}`).join('; ')}.
            </Alert>
          )}
          {status.data?.state === 'paused' && (
            <Alert severity="warning">Анализ приостановлен: новые платные запросы не выполняются. Прогресс сохранён.</Alert>
          )}
          {status.data?.state === 'superseded' && (
            <Alert severity="warning">
              {status.data.superseded_from_state === 'succeeded'
                ? 'Старый анализ успел завершиться по прежним критериям и помечен устаревшим.'
                : 'Старый анализ остановлен, потому что критерии изменились.'} Исторические результаты сохранены.
              При необходимости запустите новую ручную проверку.
            </Alert>
          )}
          {aiCalls === 0 && status.data?.ai_skip_reason && (
            <Alert severity="info">
              {aiSkipReasonText[status.data.ai_skip_reason] ?? 'AI-анализ не выполнялся; причина недоступна.'}
            </Alert>
          )}
          <Typography variant="body2" color="text.secondary">{status.data?.state === 'disabled' ? 'AI отключён; проверка по критериям ещё не запускалась.' : status.data?.state === 'never_run' ? 'Проверка ещё не запускалась.' : status.data?.state === 'queued' ? 'Снимок зафиксирован и ожидает начала проверки.' : status.data?.state === 'running' ? (status.data?.ai_retries ? 'Проверка идёт; временные ошибки провайдера повторяются с задержкой.' : 'Проверка идёт небольшими пакетами; новые вакансии попадут в следующий запуск.') : status.data?.state === 'paused' ? 'Проверка безопасно приостановлена и может быть продолжена тем же запуском.' : status.data?.state === 'superseded' ? 'Этот снимок завершён без изменения его исторических результатов.' : status.data?.state === 'unknown' || status.data?.ai_status === 'unknown' ? 'Статус старого запуска недоступен; данные показаны без предположений.' : status.data?.state === 'failed' ? 'Проверка по критериям завершилась с безопасной ошибкой; повторите запуск.' : status.data?.ai_status === 'completed' ? 'Проверка по критериям и AI-анализ завершены.' : status.data?.ai_status === 'partial' || status.data?.ai_status === 'failed' ? 'Проверка по критериям завершена; AI-анализ завершён частично или с ошибкой.' : status.data?.pending_candidates ? 'Есть новые вакансии для автоматической обработки.' : 'Проверка по критериям завершена.'}</Typography>
          {status.data?.finished_at && <Typography variant="body2">Последний анализ: {new Date(status.data.finished_at).toLocaleString('ru-RU')}</Typography>}
          {status.data?.last_checked_at && <Typography variant="caption" color="text.secondary">
            Обновлено: {new Date(status.data.last_checked_at).toLocaleTimeString('ru-RU')}
          </Typography>}
          {status.data?.worker_last_seen_at && <Typography variant="caption" color="text.secondary">
            Последний heartbeat: {new Date(status.data.worker_last_seen_at).toLocaleTimeString('ru-RU')}
          </Typography>}
          <Button type="button" variant="contained" disabled={run.isPending || preferences.isLoading} onClick={startRun}>
            {run.isPending
              ? 'Подготавливаем список вакансий…'
              : status.data?.state === 'queued' || status.data?.state === 'running' || status.data?.state === 'paused'
                ? 'Показать текущий анализ'
                : 'Проверить текущие вакансии'}
          </Button>
          {activeRunID
            && (status.data?.state === 'queued' || status.data?.state === 'running' || status.data?.state === 'paused')
            && (status.data?.current_preference_version ?? 0) > (status.data?.preference_version ?? 0) && (
            <Button type="button" color="warning" variant="outlined" disabled={supersedeRun.isPending}
              onClick={() => supersedeRun.mutate(activeRunID)}>
              {supersedeRun.isPending ? 'Останавливаем…' : 'Остановить старый анализ'}
            </Button>
          )}
          {(status.data?.state === 'queued' || status.data?.state === 'running' || status.data?.state === 'paused') && (
            <Alert severity="info">
              {submittedRunID ? 'Проверка запущена.' : 'Активная проверка восстановлена.'} Прогресс обновляется автоматически.
              {activeRunID && <> ID запуска: {activeRunID}.</>}
            </Alert>
          )}
          {submittedRunID && status.data?.state === 'succeeded' && (
            <Alert severity={status.data.ai_status === 'partial' || status.data.ai_status === 'failed' ? 'warning' : 'success'}>
              {status.data.ai_status === 'partial' || status.data.ai_status === 'failed'
                ? 'Проверка завершена, но часть AI-запросов не удалась.'
                : 'Проверка вакансий завершена.'}
            </Alert>
          )}
          {runPrerequisiteMissing && <Alert severity="warning" action={
            <Button type="button" color="inherit" size="small" onClick={() => focusSection(criteriaSectionRef.current)}>
              К критериям
            </Button>
          }>
            Сначала сохраните критерии вакансии, затем запустите анализ.
          </Alert>}
          {run.isError && <Alert severity="error">Не удалось запустить анализ: {run.error.message}</Alert>}
          {supersedeRun.isError && <Alert severity="error">Не удалось остановить старый анализ: {supersedeRun.error.message}</Alert>}
        </Stack>
      </CardContent></Card>
      <Card variant="outlined"><CardContent>
        <Stack ref={criteriaSectionRef} tabIndex={-1} spacing={2}>
          <Typography variant="h6">Критерии вакансии</Typography>
          <Autocomplete multiple options={approvedRoleOptions} groupBy={(option) => option.group}
            getOptionLabel={(option) => option.label}
            value={approvedRoleOptions.filter((option) => approvedRoleIDsValue.includes(option.id))}
            onChange={(_, values) => {
              setApprovedRoleIDs(values.map((value) => value.id))
              if (legacyRoleState === 'unknown' && values.length > 0) setLegacyRoleResolved(true)
            }}
            renderInput={(params) => <TextField {...params} label="Роли" placeholder="Выберите одну или несколько" />} />
          {developerSelected && <Alert severity="info">
            «Разработчик» — широкая роль HeadHunter. Выберите специализацию, чтобы сузить результаты.
          </Alert>}
          {developerSelected && <TextField
            select
            required
            label="Специализация"
            value={specializationValue}
            onChange={(event) => setSpecialization(event.target.value)}
            helperText="Frontend, Backend и Fullstack проверяются отдельно"
          >
            {specializationOptions.map((option) => <MenuItem key={option.id} value={option.id}>{option.label}</MenuItem>)}
          </TextField>}
          {preferences.data?.legacy_specialization_suggestion && !savedSpecialization && (
            <Alert severity="warning">
              Ранее была указана специализация «{specializationOptions.find(
                (option) => option.id === preferences.data?.legacy_specialization_suggestion,
              )?.label}». Выберите её вручную и сохраните новую версию.
            </Alert>
          )}
          {developerSelected && (
            <Stack spacing={0.5}>
              <FormControlLabel
                control={<Switch checked={includeLeadershipValue} onChange={(event) => setIncludeLeadership(event.target.checked)} />}
                label="Включать руководящие вакансии"
              />
              <Typography variant="body2" color="text.secondary">
                Выключено: скрываются team lead, tech lead, руководитель, директор и CTO. Senior, старший и ведущий остаются обычными специалистами.
              </Typography>
            </Stack>
          )}
          {legacyRoleState === 'unknown' && <Alert severity="warning">
            Не удалось распознать сохранённую роль. Выберите подходящую роль из списка.
          </Alert>}
          <Box sx={{
            display: 'grid',
            gridTemplateColumns: { xs: '1fr', md: 'repeat(2, minmax(0, 1fr))' },
            gap: 2,
          }}>
            <TagsField
              label="Регионы"
              placeholder="Например, Москва"
              value={regionsValue}
              onChange={setRegions}
            />
            <TextField
              label="Минимальная зарплата, ₽"
              type="number"
              value={minSalaryRUBValue}
              onChange={(event) => setMinSalaryRUB(event.target.value)}
              placeholder="Например, 180000"
              error={salaryError}
              helperText={salaryError ? 'Введите число не меньше 0' : 'До вычета налогов, в рублях'}
              slotProps={{ htmlInput: { min: 0, inputMode: 'numeric' } }}
            />
          </Box>
          <FormControl>
            <FormLabel id="work-format-label">Формат работы</FormLabel>
            <RadioGroup
              row
              aria-labelledby="work-format-label"
              value={remoteOnlyValue ? 'remote' : 'any'}
              onChange={(event) => setRemoteOnly(event.target.value === 'remote')}
            >
              <FormControlLabel value="any" control={<Radio />} label="Любой" />
              <FormControlLabel value="remote" control={<Radio />} label="Удалённо" />
            </RadioGroup>
          </FormControl>
          <TagsField
            label="Обязательные навыки"
            placeholder="Например, React"
            value={requiredSkillsValue}
            onChange={setRequiredSkills}
          />
          <Accordion disableGutters>
            <AccordionSummary aria-controls="additional-criteria-content" id="additional-criteria-header">
              <Typography>Дополнительные критерии</Typography>
            </AccordionSummary>
            <AccordionDetails>
              <TagsField
                label="Исключить навыки"
                placeholder="Например, PHP"
                value={excludedSkillsValue}
                onChange={setExcludedSkills}
              />
            </AccordionDetails>
          </Accordion>
          <Accordion disableGutters>
            <AccordionSummary aria-controls="additional-wishes-content" id="additional-wishes-header">
              <Typography>Дополнительные пожелания</Typography>
            </AccordionSummary>
            <AccordionDetails>
              <TextField
                fullWidth
                multiline
                minRows={3}
                label="Пожелания к вакансии"
                value={noteValue}
                onChange={(event) => setNote(event.target.value)}
                placeholder="Например, продуктовая команда и гибкий график"
                slotProps={{ htmlInput: { maxLength: 2000 } }}
              />
            </AccordionDetails>
          </Accordion>
          <Button variant="contained" disabled={save.isPending || salaryError || legacyRoleState === 'unknown' || (developerSelected && !specializationValue)} onClick={() => void save.mutate()}>
            Сохранить критерии
          </Button>
          {save.isError && <Alert severity="error">Не удалось сохранить критерии: {saveError}</Alert>}
          {confirmed && <Typography role="status" color="success.main">Сохранено</Typography>}
          <Accordion disableGutters>
            <AccordionSummary aria-controls="preferences-history-content" id="preferences-history-header">
              <Typography>История настроек</Typography>
            </AccordionSummary>
            <AccordionDetails>
              <Stack spacing={2}>
                {preferences.data?.version && <Chip label={`Версия ${preferences.data.version}${preferences.data.active_from ? ` · ${new Date(preferences.data.active_from).toLocaleString('ru-RU')}` : ''}`} sx={{ width: 'fit-content' }} />}
                <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1}>
                  <Button color="warning" variant="outlined" disabled={!preferences.data?.id || archive.isPending} onClick={() => setConfirmAction('archive')}>Архивировать</Button>
                  <Typography variant="body2" color="text.secondary" sx={{ alignSelf: { sm: 'center' } }}>Всего версий: {preferenceList.data?.length ?? 0}</Typography>
                </Stack>
              </Stack>
            </AccordionDetails>
          </Accordion>
        </Stack>
      </CardContent></Card>
      <Card variant="outlined"><CardContent>
        <Stack spacing={2}>
          <Typography variant="h6">Результаты</Typography>
          {(status.data?.method_version || status.data?.preference_version) && (
            <Typography variant="body2" color="text.secondary">
              {status.data?.method_version ? `Текущие правила: ${status.data.method_version}` : 'Текущие правила: —'}
              {status.data?.preference_version ? ` · критерии версии ${status.data.preference_version}` : ''}
              {' · выдача текущего запуска'}
            </Typography>
          )}
          {matches.isLoading && <Typography>Загрузка…</Typography>}
          {matches.isError && <Alert severity="warning">Assistant API пока не подключён.</Alert>}
          {(() => {
            const visible = (matches.data ?? []).filter((match) => !isManagementLeadershipTitle(match.title ?? ''))
            const suitable = visible.filter((match) => match.decision === 'match')
            const review = visible.filter((match) => match.decision === 'review')
            if (!matches.isLoading && !matches.isError && visible.length === 0) {
              return (
                <Typography color="text.secondary">
                  {status.data?.state === 'running' ? 'Подтверждено на текущем этапе: 0.' : 'Новых совпадений нет.'}
                </Typography>
              )
            }
            return (
              <>
                {suitable.length > 0 && <Typography sx={{ fontWeight: 700 }}>Подходящие</Typography>}
                {suitable.map((match, index) => (
                  <Stack key={`confirmed-${match.vacancy_id ?? 'unknown'}-${index}`} spacing={0.5}>
                    <Typography sx={{ fontWeight: 700 }}>
                      {`${match.title || 'Вакансия'}${match.stage === 'confirmed'
                        ? ` · уверенность AI: ${match.confidence === 'high' ? 'высокая' : match.confidence === 'medium' ? 'средняя' : 'низкая'}`
                        : ' · подтверждено фильтрами'}`}
                    </Typography>
                    <Typography variant="body2">
                      {match.reasons.length > 0 ? match.reasons.slice(0, 3).join(' · ') : 'Причины не указаны.'}
                    </Typography>
                    <Divider />
                  </Stack>
                ))}
                {review.length > 0 && <Typography sx={{ fontWeight: 700 }}>Нужно проверить</Typography>}
                {review.map((match, index) => (
                  <Stack key={`review-${match.vacancy_id ?? 'unknown'}-${index}`} spacing={0.5}>
                    <Typography sx={{ fontWeight: 700 }}>{match.title || 'Вакансия'}</Typography>
                    <Typography variant="body2">
                      {match.reasons.length > 0 ? match.reasons.slice(0, 3).join(' · ') : 'Не все hard-критерии подтверждены.'}
                    </Typography>
                    <Divider />
                  </Stack>
                ))}
              </>
            )
          })()}
        </Stack>
      </CardContent></Card>
      <Card variant="outlined"><CardContent>
        <Stack spacing={2}>
          <Typography variant="h6">Telegram</Typography>
          <Typography variant="body2" color="text.secondary">
            {telegram.data?.linked && telegram.data.opted_in ? 'Уведомления включены.' : 'Бот не подключён.'}
          </Typography>
          {link.data && <Alert severity="success">Откройте одноразовую ссылку: <a href={link.data.deep_link}>{link.data.deep_link}</a></Alert>}
          <Stack direction="row" spacing={1}>
            <Button variant="outlined" disabled={!telegram.data?.configured || link.isPending} onClick={() => void link.mutate()}>Создать ссылку</Button>
            <Button variant="outlined" disabled={!telegram.data?.linked || updateTelegramOptIn.isPending}
              onClick={() => void updateTelegramOptIn.mutate(!telegram.data?.opted_in)}>
              {telegram.data?.opted_in ? 'Отозвать согласие' : 'Согласиться на уведомления'}
            </Button>
            <Button color="warning" disabled={!telegram.data?.linked || revoke.isPending} onClick={() => void revoke.mutate()}>Отозвать</Button>
            <Button color="error" variant="outlined" disabled={!telegram.data?.linked || !telegram.data?.opted_in || testTelegram.isPending}
              onClick={() => setConfirmAction('test')}>Тестовое уведомление</Button>
          </Stack>
          <Typography variant="body2">Очередь: {telegram.data?.pending ?? 0} · отправлено: {telegram.data?.sent ?? 0} · ошибки: {telegram.data?.failed ?? 0} · dead-letter: {telegram.data?.dead_lettered ?? 0}</Typography>
          {telegram.data?.last_error && <Alert severity="warning">Последняя ошибка: {telegram.data.last_error}</Alert>}
          {testTelegram.isError && <Alert severity="error">Тестовая отправка не удалась: {testTelegram.error.message}</Alert>}
        </Stack>
      </CardContent></Card>
      {confirmAction && <div role="dialog" aria-label="Подтверждение действия">
        <Typography>{confirmAction === 'archive'
          ? 'Архивировать текущую версию? Она останется в истории, совпадения не удаляются.'
          : confirmAction === 'ai'
              ? `${automation.data?.ai_enabled ? 'Выключить' : 'Включить'} автоматический AI-анализ? Внешний провайдер может расходовать средства; исторические вакансии не будут обработаны.`
              : confirmAction === 'test'
                ? 'Внимание: будет отправлено одно реальное сообщение в подтверждённый Telegram-чат. Продолжить?'
                : `${automation.data?.telegram_enabled ? 'Выключить' : 'Включить'} отправку совпадений в Telegram? Сначала требуется отдельное согласие на уведомления.`}</Typography>
        <Button onClick={() => setConfirmAction(null)}>Отмена</Button>
        <Button onClick={() => {
          if (confirmAction === 'archive') void archive.mutate()
          else if (confirmAction === 'ai') void updateAutomation.mutate({ ai_enabled: !automation.data?.ai_enabled })
          else if (confirmAction === 'telegram') void updateAutomation.mutate({ telegram_enabled: !automation.data?.telegram_enabled })
          else if (confirmAction === 'test') void testTelegram.mutate()
          setConfirmAction(null)
        }}>Подтвердить</Button>
      </div>}
    </Stack>
  )
}
