const DATE_ONLY_PATTERN = /^(\d{4})-(\d{2})-(\d{2})$/

export function parseDateOnly(value: string) {
  const match = DATE_ONLY_PATTERN.exec(value.trim())
  if (!match) return null
  const year = Number(match[1])
  const month = Number(match[2])
  const day = Number(match[3])
  const date = new Date(Date.UTC(year, month - 1, day))
  if (
    date.getUTCFullYear() !== year ||
    date.getUTCMonth() !== month - 1 ||
    date.getUTCDate() !== day
  ) {
    return null
  }
  return date
}

export function normalizeDateOnly(value: string) {
  const trimmed = value.trim()
  return parseDateOnly(trimmed) ? trimmed : value
}

export function validatePublishedDateRange(from: string, to: string) {
  const fromDate = from ? parseDateOnly(from) : null
  const toDate = to ? parseDateOnly(to) : null
  if (from && !fromDate) return { field: 'from' as const, message: 'Введите дату в формате ГГГГ-ММ-ДД' }
  if (to && !toDate) return { field: 'to' as const, message: 'Введите дату в формате ГГГГ-ММ-ДД' }
  if (fromDate && toDate) {
    const days = (toDate.getTime() - fromDate.getTime()) / 86_400_000
    if (days < 0) return { field: 'to' as const, message: 'Дата «до» не может быть раньше даты «от»' }
    if (days > 366) return { field: 'to' as const, message: 'Диапазон не может превышать 366 дней' }
  }
  return null
}
