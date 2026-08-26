import { Alert, Button, Card, CardContent, Chip, Divider, Stack, TextField, Typography } from '@mui/material'
import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api/client'

export function AssistantPage() {
  const client = useQueryClient()
  const preferences = useQuery({ queryKey: ['assistant-preferences'], queryFn: api.assistantPreferences })
  const preferenceList = useQuery({ queryKey: ['assistant-preference-list'], queryFn: api.assistantPreferenceList })
  const status = useQuery({ queryKey: ['assistant-status'], queryFn: api.assistantStatus, refetchInterval: 3000 })
  const matches = useQuery({ queryKey: ['assistant-matches'], queryFn: api.assistantMatches })
  const telegram = useQuery({ queryKey: ['telegram-status'], queryFn: api.telegramStatus })
  const [note, setNote] = useState<string>()
  const [confirmed, setConfirmed] = useState(false)
  const [confirmAction, setConfirmAction] = useState<'archive' | 'run' | null>(null)
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

  return (
    <Stack spacing={3}>
      <div>
        <Typography variant="h4" component="h1" sx={{ fontWeight: 800 }}>Мои вакансии</Typography>
        <Typography color="text.secondary">Локальный помощник: сначала прозрачные критерии, затем опциональный AI.</Typography>
      </div>
      <Alert severity="info">Режим разработки. DeepSeek и Telegram выключены по умолчанию; ключи не вводятся в браузере.</Alert>
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
          {save.isError && <Alert severity="error">Не удалось сохранить критерии: {save.error.message}</Alert>}
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
            <Button color="warning" disabled={!telegram.data?.linked || revoke.isPending} onClick={() => void revoke.mutate()}>Отозвать</Button>
          </Stack>
        </Stack>
      </CardContent></Card>
      {confirmAction && <div role="dialog" aria-label="Подтверждение действия">
        <Typography>{confirmAction === 'archive' ? 'Архивировать текущую версию? Она останется в истории, совпадения не удаляются.' : 'Запустить bounded-анализ? Будет обработано не более 25 новых вакансий; внешний AI выключен без серверного opt-in.'}</Typography>
        <Button onClick={() => setConfirmAction(null)}>Отмена</Button>
        <Button onClick={() => confirmAction === 'archive' ? void archive.mutate() : void run.mutate()}>Подтвердить</Button>
      </div>}
    </Stack>
  )
}
