import ErrorOutlineRoundedIcon from '@mui/icons-material/ErrorOutlineRounded'
import InboxOutlinedIcon from '@mui/icons-material/InboxOutlined'
import { Alert, Box, Button, Stack, Typography } from '@mui/material'
import { ApiError } from '../api/client'

export function ErrorState({
  error,
  onRetry,
  compact = false,
}: {
  error: Error
  onRetry: () => void
  compact?: boolean
}) {
  const requestId = error instanceof ApiError ? error.requestId : undefined

  return (
    <Alert
      severity="error"
      icon={<ErrorOutlineRoundedIcon />}
      action={
        <Button color="inherit" size="small" onClick={onRetry}>
          Повторить
        </Button>
      }
      sx={{ py: compact ? 0.5 : 1.5 }}
    >
      <Typography variant="body2">{error.message}</Typography>
      {requestId && (
        <Typography variant="caption" sx={{ display: 'block', mt: 0.5 }}>
          ID запроса: {requestId}
        </Typography>
      )}
    </Alert>
  )
}

export function EmptyState({
  title = 'Данных пока нет',
  description = 'Попробуйте изменить период или фильтры.',
}: {
  title?: string
  description?: string
}) {
  return (
    <Stack sx={{ alignItems: 'center', textAlign: 'center', py: 7, px: 2 }}>
      <Box sx={{ color: 'text.secondary', mb: 1 }}>
        <InboxOutlinedIcon fontSize="large" />
      </Box>
      <Typography variant="h6">{title}</Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
        {description}
      </Typography>
    </Stack>
  )
}
