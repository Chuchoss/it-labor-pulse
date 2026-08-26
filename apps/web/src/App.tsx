import { CssBaseline, ThemeProvider } from '@mui/material'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import { AppShell } from './components/AppShell'
import {
  CurrencyProvider,
  readStoredCurrency,
  storeCurrency,
  type DisplayCurrency,
} from './components/CurrencyContext'
import { DashboardPage } from './pages/DashboardPage'
import { MarketPage } from './pages/MarketPage'
import { VacanciesPage } from './pages/VacanciesPage'
import {
  COLOR_MODE_STORAGE_KEY,
  createAppTheme,
  readInitialColorMode,
  type ColorMode,
} from './theme'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      retry: 1,
      refetchOnWindowFocus: false,
    },
  },
})

export default function App() {
  const [mode, setMode] = useState<ColorMode>(() =>
    readInitialColorMode(
      localStorage,
      window.matchMedia('(prefers-color-scheme: dark)').matches,
    ),
  )
  const [currency, setCurrencyState] = useState<DisplayCurrency>(() => {
    return readStoredCurrency(localStorage)
  })

  const theme = useMemo(() => createAppTheme(mode), [mode])

  const toggleMode = () => {
    setMode((current) => {
      const next = current === 'light' ? 'dark' : 'light'
      localStorage.setItem(COLOR_MODE_STORAGE_KEY, next)
      document.documentElement.dataset.colorMode = next
      return next
    })
  }
  const setCurrency = (next: DisplayCurrency) => {
    storeCurrency(localStorage, next)
    setCurrencyState(next)
  }

  return (
    <QueryClientProvider client={queryClient}>
      <ThemeProvider theme={theme}>
        <CssBaseline />
        <CurrencyProvider value={{ currency, setCurrency }}>
          <BrowserRouter>
            <AppShell mode={mode} onToggleMode={toggleMode}>
              <Routes>
                <Route path="/" element={<DashboardPage />} />
                <Route path="/market" element={<MarketPage />} />
                <Route path="/vacancies" element={<VacanciesPage />} />
                <Route path="*" element={<Navigate to="/" replace />} />
              </Routes>
            </AppShell>
          </BrowserRouter>
        </CurrencyProvider>
      </ThemeProvider>
    </QueryClientProvider>
  )
}
