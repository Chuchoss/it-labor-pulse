import { describe, expect, it, vi } from 'vitest'
import { readStoredCurrency, storeCurrency } from './CurrencyContext'

describe('display currency persistence', () => {
  it('defaults invalid values to RUB and persists supported selection', () => {
    expect(readStoredCurrency({ getItem: () => 'GBP' })).toBe('RUB')
    expect(readStoredCurrency({ getItem: () => 'EUR' })).toBe('EUR')
    expect(readStoredCurrency({ getItem: () => 'KZT' })).toBe('KZT')
    expect(readStoredCurrency({ getItem: () => 'AMD' })).toBe('AMD')

    const setItem = vi.fn()
    storeCurrency({ setItem }, 'AMD')
    expect(setItem).toHaveBeenCalledWith('lma-display-currency', 'AMD')
  })
})
