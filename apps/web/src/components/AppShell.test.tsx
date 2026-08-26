import { CssBaseline, ThemeProvider, createTheme } from '@mui/material'
import { QueryClient, QueryClientProvider, useQuery } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { useState } from 'react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, describe, expect, it } from 'vitest'
import { api } from '../api/client'
import { server } from '../test/server'
import { formatSalary } from '../utils/format'
import { AppShell } from './AppShell'
import {
  CurrencyProvider,
  readStoredCurrency,
  storeCurrency,
  useCurrency,
  type DisplayCurrency,
} from './CurrencyContext'

function SalaryProbe() {
  const { currency } = useCurrency()
  const summary = useQuery({
    queryKey: ['salary-probe', currency],
    queryFn: ({ signal }) =>
      api.dashboard({ from: '2026-08-01', to: '2026-08-26', currency }, signal),
  })
  return (
    <span data-testid="salary-probe">
      {summary.data ? formatSalary(summary.data.median_salary, currency) : 'Загрузка'}
    </span>
  )
}

function renderShell() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  })

  function Harness() {
    const [currency, setCurrencyState] = useState<DisplayCurrency>(() =>
      readStoredCurrency(localStorage),
    )
    const setCurrency = (next: DisplayCurrency) => {
      storeCurrency(localStorage, next)
      setCurrencyState(next)
    }

    return (
      <CurrencyProvider value={{ currency, setCurrency }}>
        <MemoryRouter>
          <AppShell mode="light" onToggleMode={() => undefined}>
            <span>Текущая валюта: {currency}</span>
            <SalaryProbe />
          </AppShell>
        </MemoryRouter>
      </CurrencyProvider>
    )
  }

  return render(
    <QueryClientProvider client={queryClient}>
      <ThemeProvider theme={createTheme()}>
        <CssBaseline />
        <Harness />
      </ThemeProvider>
    </QueryClientProvider>,
  )
}

afterEach(() => localStorage.clear())

describe('AppShell salary currency selector', () => {
  it('shows a labeled global selector, backend options and persists the choice', async () => {
    const user = userEvent.setup()
    const firstRender = renderShell()
    const select = await screen.findByRole('combobox', { name: 'Валюта зарплат' })

    expect(select).toHaveTextContent('RUB')
    await user.click(select)
    const listbox = screen.getByRole('listbox')
    for (const code of ['RUB', 'USD', 'EUR', 'CNY']) {
      expect(within(listbox).getByRole('option', { name: new RegExp(`^${code}`) })).toBeVisible()
    }
    await user.click(within(listbox).getByRole('option', { name: /^USD/ }))

    expect(await screen.findByText('Текущая валюта: USD')).toBeInTheDocument()
    expect(localStorage.getItem('lma-display-currency')).toBe('USD')

    firstRender.unmount()
    renderShell()
    expect(await screen.findByRole('combobox', { name: 'Валюта зарплат' })).toHaveTextContent(
      'USD',
    )
  })

  it('falls back to usable RUB when currency metadata is unavailable', async () => {
    localStorage.setItem('lma-display-currency', 'EUR')
    server.use(
      http.get('*/api/v1/currencies', () =>
        HttpResponse.json({ error: { message: 'unavailable' } }, { status: 503 }),
      ),
    )

    renderShell()

    await waitFor(() =>
      expect(screen.getByRole('combobox', { name: 'Валюта зарплат' })).toHaveTextContent('RUB'),
    )
    expect(screen.getByRole('status', { name: /используется RUB/ })).toBeInTheDocument()
    expect(localStorage.getItem('lma-display-currency')).toBe('RUB')
  })

  it('refetches and formats salary data for every supported currency', async () => {
    const values: Record<DisplayCurrency, number> = {
      RUB: 251150,
      USD: 2973.47,
      EUR: 2532.18,
      CNY: 21164.91,
    }
    const requestedCurrencies: string[] = []
    server.use(
      http.get('*/api/v1/dashboard/summary', ({ request }) => {
        const currency = (new URL(request.url).searchParams.get('currency') ||
          'RUB') as DisplayCurrency
        requestedCurrencies.push(currency)
        return HttpResponse.json({
          period: { from: '2026-08-01', to: '2026-08-26' },
          vacancies_active: 1,
          vacancies_new: 1,
          median_salary: values[currency],
          salary_currency: currency,
          salary_sample_size: 10,
          salary_rate_date: currency === 'RUB' ? null : '2026-08-26',
        })
      }),
    )
    const user = userEvent.setup()
    renderShell()

    for (const currency of ['RUB', 'USD', 'EUR', 'CNY'] as const) {
      const select = await screen.findByRole('combobox', { name: 'Валюта зарплат' })
      if (!select.textContent?.includes(currency)) {
        await user.click(select)
        await user.click(screen.getByRole('option', { name: new RegExp(`^${currency}`) }))
      }
      await waitFor(() =>
        expect(screen.getByTestId('salary-probe')).toHaveTextContent(
          formatSalary(values[currency], currency).replace(/[\u00a0\u202f]/g, ' '),
        ),
      )
    }

    expect(requestedCurrencies).toEqual(expect.arrayContaining(['RUB', 'USD', 'EUR', 'CNY']))
  })
})
