import { Alert, Button, Card, CardContent, Chip, Divider, Stack, TextField, Typography } from '@mui/material'
import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api/client'

export function AssistantPage() {
  const client = useQueryClient()
  const preferences = useQuery({ queryKey: ['assistant-preferences'], queryFn: api.assistantPreferences })
  const matches = useQuery({ queryKey: ['assistant-matches'], queryFn: api.assistantMatches })
  const telegram = useQuery({ queryKey: ['telegram-status'], queryFn: api.telegramStatus })
  const [note, setNote] = useState('')
  const [confirmed, setConfirmed] = useState(false)
  const save = useMutation({
    mutationFn: () => api.saveAssistantPreferences({
      note,
      hard_criteria: {},
      soft_criteria: {},
      weights: {},
    }),
    onSuccess: () => {
      setConfirmed(true)
      void client.invalidateQueries({ queryKey: ['assistant-preferences'] })
      void client.invalidateQueries({ queryKey: ['assistant-matches'] })
    },
  })
  const link = useMutation({ mutationFn: api.telegramLink })
  const revoke = useMutation({
    mutationFn: api.revokeTelegram,
    onSuccess: () => void client.invalidateQueries({ queryKey: ['telegram-status'] }),
  })

  return (
    <Stack spacing={3}>
      <div>
        <Typography variant="h4" component="h1" sx={{ fontWeight: 800 }}>Мои вакансии</Typography>
        <Typography color="text.secondary">Локальный помощник: сначала прозрачные критерии, затем опциональный AI.</Typography>
      </div>
      <Alert severity="info">Режим разработки. DeepSeek и Telegram выключены по умолчанию; ключи не вводятся в браузере.</Alert>
      <Card variant="outlined"><CardContent>
        <Stack spacing={2}>
          <Typography variant="h6">Что для меня интересная вакансия</Typography>
          <TextField multiline minRows={3} value={note} onChange={(event) => setNote(event.target.value)}
            placeholder="Например: Go backend, удалённо, от 180 000 ₽"
            slotProps={{ htmlInput: { maxLength: 2000 } }} />
          <Typography variant="body2" color="text.secondary">Пока AI не настроен, текст хранится как заметка. Matching использует подтверждённые структурированные поля.</Typography>
          <Button variant="contained" disabled={!note.trim() || save.isPending} onClick={() => void save.mutate()}>
            Сохранить критерии
          </Button>
          {confirmed && <Alert severity="success">Критерии подтверждены и сохранены новой версией.</Alert>}
          {preferences.data?.version && <Chip label={`Версия ${preferences.data.version}`} sx={{ width: 'fit-content' }} />}
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
    </Stack>
  )
}
