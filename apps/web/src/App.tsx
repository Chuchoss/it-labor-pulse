import { CssBaseline, ThemeProvider, createTheme } from '@mui/material'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import { AppShell } from './components/AppShell'
import { DashboardPage } from './pages/DashboardPage'
import { MarketPage } from './pages/MarketPage'
import { VacanciesPage } from './pages/VacanciesPage'

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
  const [mode, setMode] = useState<'light' | 'dark'>(() => {
    const saved = localStorage.getItem('lma-color-mode')
    if (saved === 'light' || saved === 'dark') return saved
    return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
  })

  const theme = useMemo(
    () =>
      createTheme({
        colorSchemes: { light: true, dark: true },
        palette: {
          mode,
          primary: { main: mode === 'light' ? '#4f46e5' : '#818cf8' },
          secondary: { main: '#0f766e' },
          background: {
            default: mode === 'light' ? '#f7f8fc' : '#11131a',
            paper: mode === 'light' ? '#ffffff' : '#181b24',
          },
        },
        typography: {
          fontFamily:
            '"Inter", "Segoe UI", Roboto, Helvetica, Arial, sans-serif',
          h4: { letterSpacing: '-0.025em' },
          h6: { fontWeight: 700 },
          button: { textTransform: 'none', fontWeight: 650 },
        },
        shape: { borderRadius: 12 },
        components: {
          MuiCard: {
            styleOverrides: {
              root: { backgroundImage: 'none' },
            },
          },
          MuiButtonBase: {
            defaultProps: { disableRipple: false },
          },
        },
      }),
    [mode],
  )

  const toggleMode = () => {
    setMode((current) => {
      const next = current === 'light' ? 'dark' : 'light'
      localStorage.setItem('lma-color-mode', next)
      return next
    })
  }

  return (
    <QueryClientProvider client={queryClient}>
      <ThemeProvider theme={theme}>
        <CssBaseline />
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
      </ThemeProvider>
    </QueryClientProvider>
  )
}
