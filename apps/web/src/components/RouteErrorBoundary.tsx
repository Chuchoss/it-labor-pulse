import { Alert, Button, Stack, Typography } from '@mui/material'
import { Component, type ErrorInfo, type ReactNode } from 'react'

interface Props {
  children: ReactNode
}

interface State {
  failed: boolean
}

export class RouteErrorBoundary extends Component<Props, State> {
  state: State = { failed: false }

  static getDerivedStateFromError(): State {
    return { failed: true }
  }

  componentDidCatch(_error: Error, _info: ErrorInfo) {
    console.error('Route rendering failed')
  }

  render() {
    if (this.state.failed) {
      return (
        <Stack spacing={2} role="alert">
          <Typography variant="h4" component="h1" sx={{ fontWeight: 800 }}>
            Раздел временно недоступен
          </Typography>
          <Alert severity="error">
            Не удалось отобразить страницу. Обновите её; если ошибка повторится, попробуйте позже.
          </Alert>
          <Button variant="contained" sx={{ width: 'fit-content' }} onClick={() => window.location.reload()}>
            Обновить страницу
          </Button>
        </Stack>
      )
    }
    return this.props.children
  }
}
