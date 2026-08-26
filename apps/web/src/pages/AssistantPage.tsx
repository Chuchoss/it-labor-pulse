import { Alert, Button, Card, CardContent, Chip, Divider, Stack, Switch, TextField, Typography } from '@mui/material'
import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, api } from '../api/client'

export function AssistantPage() {
  const client = useQueryClient()
  const preferences = useQuery({ queryKey: ['assistant-preferences'], queryFn: api.assistantPreferences })
  const preferenceList = useQuery({ queryKey: ['assistant-preference-list'], queryFn: api.assistantPreferenceList })
  const status = useQuery({ queryKey: ['assistant-status'], queryFn: api.assistantStatus, refetchInterval: 3000 })
  const matches = useQuery({ queryKey: ['assistant-matches'], queryFn: api.assistantMatches })
  const telegram = useQuery({ queryKey: ['telegram-status'], queryFn: api.telegramStatus })
  const automation = useQuery({ queryKey: ['assistant-automation'], queryFn: api.assistantAutomation })
  const [note, setNote] = useState<string>()
  const [confirmed, setConfirmed] = useState(false)
  const [confirmAction, setConfirmAction] = useState<'archive' | 'run' | 'ai' | 'telegram' | 'test' | null>(null)
  const noteValue = note ?? preferences.data?.note ?? ''
  const hardCriteriaValue = preferences.data?.hard_criteria ?? {}
  const softCriteriaValue = preferences.data?.soft_criteria ?? {}
  const weightsValue = preferences.data?.weights ?? {}
  const save = useMutation({
    mutationFn: () => api.saveAssistantPreferences({
      note: noteValue,
      hard_criteria: hardCriteriaValue,
      soft_criteria: softCriteriaValue,
      weights: weightsValue,
    }),
    onSuccess: () => {
      setConfirmed(true)
      void client.refetchQueries({ queryKey: ['assistant-preferences'] })
      void client.invalidateQueries({ queryKey: ['assistant-matches'] })
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
  const run = useMutation({ mutationFn: api.runAssistantAnalysis, onSuccess: () => {
    setConfirmAction(null)
    void client.invalidateQueries({ queryKey: ['assistant-status'] })
  } })
  const updateAutomation = useMutation({
    mutationFn: (value: { ai_enabled?: boolean; telegram_enabled?: boolean }) => api.updateAssistantAutomation(value),
    onSuccess: () => void client.invalidateQueries({ queryKey: ['assistant-automation'] }),
  })
  const updateTelegramOptIn = useMutation({
    mutationFn: (value: boolean) => api.updateTelegramOptIn(value),
    onSuccess: () => void client.invalidateQueries({ queryKey: ['telegram-status'] }),
  })
  const testTelegram = useMutation({ mutationFn: api.testTelegram, onSuccess: () => void telegram.refetch() })
  const saveError = save.error instanceof ApiError
    ? `${save.error.message} (${save.error.code ?? 'API_ERROR'}${save.error.requestId ? `, request_id: ${save.error.requestId}` : ''})`
    : save.error?.message

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
            AI анализирует только вакансии, впервые наблюдённые после включения. Это может расходовать лимит провайдера; Telegram включается отдельно.
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
        <Stack spacing={2}>
          <Typography variant="h6">Статус анализа</Typography>
          <Stack direction="row" spacing={1}>
            <Chip label={status.data?.state ?? 'загрузка'} color={status.data?.state === 'succeeded' ? 'success' : status.data?.state === 'failed' ? 'error' : 'default'} />
            <Button size="small" onClick={() => void status.refetch()}>Обновить</Button>
          </Stack>
          {(status.data?.state === 'queued' || status.data?.state === 'running') && <Typography>Анализ выполняется…</Typography>}
          <Typography>Детерминированный анализ: {status.data?.processed ?? 0} вакансий</Typography>
          <Typography>AI-анализ: {status.data?.ai_calls ?? 0} вакансий · Совпадения: {status.data?.matched ?? 0}</Typography>
          <Typography variant="body2" color="text.secondary">{status.data?.state === 'disabled' ? 'AI отключена; внешний провайдер не вызывается.' : status.data?.state === 'never_run' ? 'AI ещё не запускалась.' : status.data?.pending_candidates ? 'Есть кандидаты, ожидающие обработки.' : 'Подходящих кандидатов нет или очередь обработана.'}</Typography>
          {status.data?.finished_at && <Typography variant="body2">Последний анализ: {new Date(status.data.finished_at).toLocaleString('ru-RU')}</Typography>}
          <Button variant="contained" disabled={run.isPending || status.data?.state === 'queued' || status.data?.state === 'running'} onClick={() => setConfirmAction('run')}>Запустить анализ</Button>
          {run.isError && <Alert severity="error">Не удалось запустить анализ: {run.error.message}</Alert>}
        </Stack>
      </CardContent></Card>
      <Card variant="outlined"><CardContent>
        <Stack spacing={2}>
          <Typography variant="h6">Что для меня интересная вакансия</Typography>
          <TextField multiline minRows={3} value={noteValue} onChange={(event) => setNote(event.target.value)}
            placeholder="Например: Go backend, удалённо, от 180 000 ₽"
            slotProps={{ htmlInput: { maxLength: 2000 } }} />
          <Typography variant="subtitle2">Жёсткие критерии</Typography>
          <Typography variant="body2">{Object.entries(hardCriteriaValue).map(([key, value]) => `${key}: ${JSON.stringify(value)}`).join(' · ') || 'Не заданы'}</Typography>
          <Typography variant="subtitle2">Мягкие критерии и веса</Typography>
          <Typography variant="body2">{Object.entries(softCriteriaValue).map(([key, value]) => `${key}: ${JSON.stringify(value)}`).join(' · ') || 'Не заданы'} · {Object.entries(weightsValue).map(([key, value]) => `${key}: ${value}`).join(' · ') || 'Веса не заданы'}</Typography>
          <Typography variant="body2" color="text.secondary">Детерминированный matcher использует hard-критерии. Сохранение создаёт новую версию; старые версии остаются в истории.</Typography>
          <Button variant="contained" disabled={!noteValue.trim() || save.isPending} onClick={() => void save.mutate()}>
            Сохранить критерии
          </Button>
          {save.isError && <Alert severity="error">Не удалось сохранить критерии: {saveError}</Alert>}
          {confirmed && <Alert severity="success">Критерии сохранены. Версия {save.data?.version ?? preferences.data?.version}.</Alert>}
          {preferences.data?.version && <Chip label={`Версия ${preferences.data.version}${preferences.data.active_from ? ` · ${new Date(preferences.data.active_from).toLocaleString('ru-RU')}` : ''}`} sx={{ width: 'fit-content' }} />}
          <Stack direction="row" spacing={1}>
            <Button color="warning" variant="outlined" disabled={!preferences.data?.id || archive.isPending} onClick={() => setConfirmAction('archive')}>Архивировать</Button>
            <Typography variant="body2" color="text.secondary" sx={{ alignSelf: 'center' }}>Всего версий: {preferenceList.data?.length ?? 0}</Typography>
          </Stack>
        </Stack>
      </CardContent></Card>
      <Card variant="outlined"><CardContent>
        <Stack spacing={2}>
          <Typography variant="h6">Совпадения</Typography>
          {matches.isLoading && <Typography>Загрузка…</Typography>}
          {matches.isError && <Alert severity="warning">Assistant API пока не подключён.</Alert>}
          {matches.data?.length === 0 && <Typography color="text.secondary">Новых совпадений нет.</Typography>}
          {matches.data?.map((match) => <Stack key={match.vacancy_id} spacing={0.5}>
            <Typography sx={{ fontWeight: 700 }}>{match.title || 'Вакансия'} · {Math.round(match.score * 100)}%</Typography>
            <Typography variant="body2">{match.reasons.slice(0, 3).join(' · ')}</Typography>
            <Divider />
          </Stack>)}
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
          : confirmAction === 'run'
            ? 'Запустить bounded-анализ? Будет обработано не более 25 новых вакансий; внешний AI выключен без серверного opt-in.'
            : confirmAction === 'ai'
              ? `${automation.data?.ai_enabled ? 'Выключить' : 'Включить'} автоматический AI-анализ? Внешний провайдер может расходовать средства; исторические вакансии не будут обработаны.`
              : confirmAction === 'test'
                ? 'Внимание: будет отправлено одно реальное сообщение в подтверждённый Telegram-чат. Продолжить?'
                : `${automation.data?.telegram_enabled ? 'Выключить' : 'Включить'} отправку совпадений в Telegram? Сначала требуется отдельное согласие на уведомления.`}</Typography>
        <Button onClick={() => setConfirmAction(null)}>Отмена</Button>
        <Button onClick={() => {
          if (confirmAction === 'archive') void archive.mutate()
          else if (confirmAction === 'run') void run.mutate()
          else if (confirmAction === 'ai') void updateAutomation.mutate({ ai_enabled: !automation.data?.ai_enabled })
          else if (confirmAction === 'telegram') void updateAutomation.mutate({ telegram_enabled: !automation.data?.telegram_enabled })
          else if (confirmAction === 'test') void testTelegram.mutate()
          setConfirmAction(null)
        }}>Подтвердить</Button>
      </div>}
    </Stack>
  )
}
