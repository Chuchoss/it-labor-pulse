import { describe, expect, it } from 'vitest'
import { formatSalary } from './format'

describe('salary currency formatting', () => {
  it.each([
    ['KZT', /(?:₸|KZT)/],
    ['AMD', /(?:֏|AMD)/],
  ])('formats %s with its symbol or ISO code fallback', (currency, marker) => {
    const formatted = formatSalary(100_000, currency).replace(/[\u00a0\u202f]/g, ' ')

    expect(formatted).toContain('100 000')
    expect(formatted).toMatch(marker)
  })
})
