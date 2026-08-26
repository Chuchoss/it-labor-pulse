import { getContrastRatio } from '@mui/material/styles'
import { describe, expect, it } from 'vitest'
import {
  COLOR_MODE_STORAGE_KEY,
  createAppTheme,
  readInitialColorMode,
} from './theme'

describe('application theme', () => {
  it('uses storage before OS preference and otherwise follows the OS', () => {
    expect(readInitialColorMode({ getItem: () => 'light' }, true)).toBe('light')
    expect(readInitialColorMode({ getItem: () => 'dark' }, false)).toBe('dark')
    expect(readInitialColorMode({ getItem: () => null }, true)).toBe('dark')
    expect(readInitialColorMode({ getItem: () => null }, false)).toBe('light')
    expect(COLOR_MODE_STORAGE_KEY).toBe('lma-color-mode')
  })

  it.each(['light', 'dark'] as const)(
    '%s palette keeps semantic text and controls above target contrast',
    (mode) => {
      const palette = createAppTheme(mode).palette

      expect(getContrastRatio(palette.text.primary, palette.background.paper)).toBeGreaterThanOrEqual(4.5)
      expect(getContrastRatio(palette.text.secondary, palette.background.paper)).toBeGreaterThanOrEqual(4.5)
      expect(getContrastRatio(palette.primary.main, palette.background.paper)).toBeGreaterThanOrEqual(3)
      expect(getContrastRatio(palette.focusRing, palette.background.default)).toBeGreaterThanOrEqual(3)
      expect(getContrastRatio(palette.highlight.newEdge, palette.highlight.new)).toBeGreaterThanOrEqual(3)
    },
  )

  it('keeps light and dark surfaces clearly separated', () => {
    const light = createAppTheme('light').palette
    const dark = createAppTheme('dark').palette

    expect(light.background.default).toBe('#f6f7fb')
    expect(light.background.paper).toBe('#ffffff')
    expect(dark.background.default).toBe('#090b10')
    expect(dark.background.paper).toBe('#12151c')
    expect(light.surface.muted).not.toBe(light.background.paper)
    expect(dark.surface.elevated).not.toBe(dark.background.default)
  })
})
