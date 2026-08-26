/* oxlint-disable react/only-export-components */
import { createContext, useContext, type ReactNode } from 'react'

export type DisplayCurrency = 'RUB' | 'USD' | 'EUR' | 'CNY'

export function readStoredCurrency(storage: Pick<Storage, 'getItem'>): DisplayCurrency {
  const saved = storage.getItem('lma-display-currency')
  return saved === 'USD' || saved === 'EUR' || saved === 'CNY' ? saved : 'RUB'
}

export function storeCurrency(storage: Pick<Storage, 'setItem'>, currency: DisplayCurrency) {
  storage.setItem('lma-display-currency', currency)
}

interface CurrencyContextValue {
  currency: DisplayCurrency
  setCurrency: (currency: DisplayCurrency) => void
}

const CurrencyContext = createContext<CurrencyContextValue | null>(null)

export function CurrencyProvider({
  value,
  children,
}: {
  value: CurrencyContextValue
  children: ReactNode
}) {
  return <CurrencyContext.Provider value={value}>{children}</CurrencyContext.Provider>
}

export function useCurrency() {
  const context = useContext(CurrencyContext)
  if (!context) throw new Error('CurrencyProvider is required')
  return context
}
