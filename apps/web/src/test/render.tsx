import { CssBaseline, ThemeProvider, createTheme } from '@mui/material'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render } from '@testing-library/react'
import type { ReactElement } from 'react'
import { MemoryRouter } from 'react-router-dom'
import { CurrencyProvider } from '../components/CurrencyContext'

export function renderPage(ui: ReactElement, initialEntry = '/') {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0 },
    },
  })

  return render(
    <QueryClientProvider client={queryClient}>
      <ThemeProvider theme={createTheme()}>
        <CssBaseline />
        <CurrencyProvider value={{ currency: 'RUB', setCurrency: () => undefined }}>
          <MemoryRouter initialEntries={[initialEntry]}>{ui}</MemoryRouter>
        </CurrencyProvider>
      </ThemeProvider>
    </QueryClientProvider>,
  )
}
