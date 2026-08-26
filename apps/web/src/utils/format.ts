const numberFormatter = new Intl.NumberFormat('ru-RU')
const compactFormatter = new Intl.NumberFormat('ru-RU', {
  notation: 'compact',
  maximumFractionDigits: 1,
})
const dateFormatter = new Intl.DateTimeFormat('ru-RU', {
  day: 'numeric',
  month: 'short',
  year: 'numeric',
})

export const formatNumber = (value?: number | null) =>
  value === undefined || value === null ? '—' : numberFormatter.format(value)

export const formatCompact = (value?: number | null) =>
  value === undefined || value === null ? '—' : compactFormatter.format(value)

export const formatSalary = (value?: number | null, currency = 'RUB') =>
  value === undefined || value === null || value <= 0
    ? '—'
    : new Intl.NumberFormat('ru-RU', {
        style: 'currency',
        currency,
        maximumFractionDigits: 0,
      }).format(value)

export const formatDate = (value?: string | null) => {
  if (!value) return '—'
  const date = new Date(value)
  return Number.isNaN(date.valueOf()) ? '—' : dateFormatter.format(date)
}

export const formatSalaryRange = (
  from?: number | null,
  to?: number | null,
  currency = 'RUB',
) => {
  if (from && to) return `${formatSalary(from, currency)} — ${formatSalary(to, currency)}`
  if (from) return `от ${formatSalary(from, currency)}`
  if (to) return `до ${formatSalary(to, currency)}`
  return 'Не указана'
}
