import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it } from 'vitest'
import App from './App'
import { COLOR_MODE_STORAGE_KEY } from './theme'

afterEach(() => {
  localStorage.clear()
  delete document.documentElement.dataset.colorMode
})

describe('color mode toggle', () => {
  it('toggles the active theme and persists the choice', async () => {
    localStorage.setItem(COLOR_MODE_STORAGE_KEY, 'dark')
    document.documentElement.dataset.colorMode = 'dark'
    const user = userEvent.setup()

    render(<App />)
    await user.click(screen.getByRole('button', { name: 'Переключить тему' }))

    expect(localStorage.getItem(COLOR_MODE_STORAGE_KEY)).toBe('light')
    expect(document.documentElement.dataset.colorMode).toBe('light')
    expect(screen.getByRole('button', { name: 'Переключить тему' })).toBeInTheDocument()
  })
})
