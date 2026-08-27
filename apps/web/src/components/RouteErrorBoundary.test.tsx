import { screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { renderPage } from '../test/render'
import { RouteErrorBoundary } from './RouteErrorBoundary'

function BrokenRoute(): never {
  throw new Error('synthetic render failure')
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('RouteErrorBoundary', () => {
  it('shows a useful Russian fallback after an unexpected render error', () => {
    vi.spyOn(console, 'error').mockImplementation(() => undefined)

    renderPage(
      <RouteErrorBoundary>
        <BrokenRoute />
      </RouteErrorBoundary>,
    )

    expect(screen.getByRole('heading', { name: 'Раздел временно недоступен' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Обновить страницу' })).toBeInTheDocument()
  })
})
