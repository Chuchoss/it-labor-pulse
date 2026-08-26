import { alpha, createTheme, type PaletteMode } from '@mui/material/styles'

declare module '@mui/material/styles' {
  interface Palette {
    surface: {
      elevated: string
      muted: string
    }
    border: {
      subtle: string
      strong: string
    }
    chart: {
      primary: string
      secondary: string
      tertiary: string
      grid: string
      track: string
    }
    highlight: {
      new: string
      newFade: string
      newEdge: string
    }
    focusRing: string
  }

  interface PaletteOptions {
    surface?: Palette['surface']
    border?: Palette['border']
    chart?: Palette['chart']
    highlight?: Palette['highlight']
    focusRing?: string
  }
}

export type ColorMode = PaletteMode

export const COLOR_MODE_STORAGE_KEY = 'lma-color-mode'

export function readInitialColorMode(
  storage: Pick<Storage, 'getItem'>,
  prefersDark: boolean,
): ColorMode {
  const saved = storage.getItem(COLOR_MODE_STORAGE_KEY)
  if (saved === 'light' || saved === 'dark') return saved
  return prefersDark ? 'dark' : 'light'
}

const light = {
  primary: '#5145cd',
  primaryDark: '#4035a8',
  secondary: '#0f766e',
  background: '#f6f7fb',
  paper: '#ffffff',
  elevated: '#ffffff',
  muted: '#eef1f6',
  text: '#171923',
  textSecondary: '#596274',
  border: '#dfe3eb',
  borderStrong: '#c5cbd7',
  chartGrid: '#d9dee8',
  chartTrack: '#e7eaf1',
  chartSecondary: '#0f766e',
  chartTertiary: '#7c6ee6',
  success: '#237a3b',
  successHighlight: '#e4f4e8',
  successEdge: '#2f7d42',
  warning: '#a15c00',
  error: '#bd2c35',
  info: '#1769aa',
  focus: '#5145cd',
} as const

const dark = {
  primary: '#9b94f5',
  primaryDark: '#b3adff',
  secondary: '#57c4b8',
  background: '#090b10',
  paper: '#12151c',
  elevated: '#191d26',
  muted: '#1e232d',
  text: '#f4f6fb',
  textSecondary: '#aeb6c5',
  border: '#2a303b',
  borderStrong: '#414958',
  chartGrid: '#343b48',
  chartTrack: '#262c36',
  chartSecondary: '#57c4b8',
  chartTertiary: '#c0bbff',
  success: '#73c58a',
  successHighlight: '#173424',
  successEdge: '#73c58a',
  warning: '#f1b45d',
  error: '#ff858d',
  info: '#79b8ef',
  focus: '#b3adff',
} as const

export function createAppTheme(mode: ColorMode) {
  const colors = mode === 'light' ? light : dark
  const isDark = mode === 'dark'

  return createTheme({
    palette: {
      mode,
      primary: {
        main: colors.primary,
        dark: colors.primaryDark,
        contrastText: isDark ? '#101218' : '#ffffff',
      },
      secondary: { main: colors.secondary },
      success: { main: colors.success },
      warning: { main: colors.warning },
      error: { main: colors.error },
      info: { main: colors.info },
      background: {
        default: colors.background,
        paper: colors.paper,
      },
      text: {
        primary: colors.text,
        secondary: colors.textSecondary,
      },
      divider: colors.border,
      action: {
        hover: alpha(colors.primary, isDark ? 0.12 : 0.07),
        selected: alpha(colors.primary, isDark ? 0.2 : 0.11),
        focus: alpha(colors.focus, 0.2),
        disabledBackground: colors.muted,
      },
      surface: {
        elevated: colors.elevated,
        muted: colors.muted,
      },
      border: {
        subtle: colors.border,
        strong: colors.borderStrong,
      },
      chart: {
        primary: colors.primary,
        secondary: colors.chartSecondary,
        tertiary: colors.chartTertiary,
        grid: colors.chartGrid,
        track: colors.chartTrack,
      },
      highlight: {
        new: colors.successHighlight,
        newFade: alpha(colors.successHighlight, 0),
        newEdge: colors.successEdge,
      },
      focusRing: colors.focus,
    },
    typography: {
      fontFamily: '"Inter", "Segoe UI", Roboto, Helvetica, Arial, sans-serif',
      h4: { letterSpacing: '-0.025em' },
      h6: { fontWeight: 700 },
      button: { textTransform: 'none', fontWeight: 650 },
    },
    shape: { borderRadius: 12 },
    shadows: [
      'none',
      isDark ? '0 1px 3px rgba(0,0,0,.5)' : '0 1px 3px rgba(20,24,35,.08)',
      isDark ? '0 4px 14px rgba(0,0,0,.42)' : '0 4px 14px rgba(20,24,35,.09)',
      ...Array(22).fill(isDark ? '0 8px 24px rgba(0,0,0,.44)' : '0 8px 24px rgba(20,24,35,.1)'),
    ] as ReturnType<typeof createTheme>['shadows'],
    components: {
      MuiCssBaseline: {
        styleOverrides: {
          ':root': { colorScheme: mode },
          body: { backgroundColor: colors.background },
          '.MuiChartsGrid-line': { stroke: colors.chartGrid },
          '.MuiChartsAxis-line, .MuiChartsAxis-tick': { stroke: colors.borderStrong },
          '.MuiChartsAxis-tickLabel, .MuiChartsLegend-label': { fill: colors.textSecondary },
          '*:focus-visible': {
            outline: `3px solid ${colors.focus}`,
            outlineOffset: 2,
          },
        },
      },
      MuiAppBar: {
        styleOverrides: {
          root: {
            backgroundImage: 'none',
            backgroundColor: alpha(colors.elevated, 0.94),
            borderColor: colors.border,
          },
        },
      },
      MuiDrawer: {
        styleOverrides: {
          paper: {
            backgroundImage: 'none',
            backgroundColor: colors.paper,
            borderColor: colors.border,
          },
        },
      },
      MuiCard: {
        styleOverrides: {
          root: {
            backgroundImage: 'none',
            backgroundColor: colors.paper,
            borderColor: colors.border,
          },
        },
      },
      MuiPaper: {
        styleOverrides: {
          root: { backgroundImage: 'none' },
          elevation8: {
            backgroundColor: colors.elevated,
            border: `1px solid ${colors.border}`,
          },
        },
      },
      MuiTableCell: {
        styleOverrides: {
          root: { borderColor: colors.border },
          head: {
            backgroundColor: colors.muted,
            color: colors.text,
            fontWeight: 700,
          },
        },
      },
      MuiOutlinedInput: {
        styleOverrides: {
          root: {
            backgroundColor: colors.elevated,
            '& .MuiOutlinedInput-notchedOutline': { borderColor: colors.borderStrong },
            '&:hover .MuiOutlinedInput-notchedOutline': { borderColor: colors.primary },
            '&.Mui-focused .MuiOutlinedInput-notchedOutline': { borderWidth: 2 },
          },
        },
      },
      MuiToggleButton: {
        styleOverrides: {
          root: {
            borderColor: colors.borderStrong,
            color: colors.textSecondary,
            '&.Mui-selected': {
              color: colors.text,
              backgroundColor: alpha(colors.primary, isDark ? 0.22 : 0.12),
            },
          },
        },
      },
      MuiChip: {
        styleOverrides: {
          root: {
            borderColor: colors.borderStrong,
            '&.MuiChip-filled.MuiChip-colorDefault': { backgroundColor: colors.muted },
          },
        },
      },
      MuiSkeleton: {
        styleOverrides: {
          root: { backgroundColor: colors.chartTrack },
        },
      },
      MuiLinearProgress: {
        styleOverrides: {
          root: { backgroundColor: colors.chartTrack },
        },
      },
      MuiAlert: {
        styleOverrides: {
          root: { border: `1px solid ${colors.borderStrong}` },
        },
      },
      MuiTooltip: {
        styleOverrides: {
          tooltip: {
            backgroundColor: isDark ? '#e9ecf4' : '#242834',
            color: isDark ? '#14171e' : '#ffffff',
            boxShadow: isDark ? '0 4px 14px rgba(0,0,0,.5)' : '0 4px 14px rgba(20,24,35,.18)',
          },
        },
      },
      MuiMenu: {
        styleOverrides: {
          paper: {
            backgroundColor: colors.elevated,
            border: `1px solid ${colors.border}`,
          },
        },
      },
      MuiPopover: {
        styleOverrides: {
          paper: {
            backgroundColor: colors.elevated,
            border: `1px solid ${colors.border}`,
          },
        },
      },
      MuiListItemButton: {
        styleOverrides: {
          root: {
            '&.Mui-selected': {
              backgroundColor: alpha(colors.primary, isDark ? 0.2 : 0.11),
              color: colors.primary,
            },
          },
        },
      },
      MuiButtonBase: {
        defaultProps: { disableRipple: false },
      },
    },
  })
}
