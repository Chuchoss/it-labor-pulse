import DarkModeOutlinedIcon from '@mui/icons-material/DarkModeOutlined'
import DashboardRoundedIcon from '@mui/icons-material/DashboardRounded'
import LightModeOutlinedIcon from '@mui/icons-material/LightModeOutlined'
import MenuRoundedIcon from '@mui/icons-material/MenuRounded'
import QueryStatsRoundedIcon from '@mui/icons-material/QueryStatsRounded'
import WarningAmberRoundedIcon from '@mui/icons-material/WarningAmberRounded'
import WorkOutlineRoundedIcon from '@mui/icons-material/WorkOutlineRounded'
import {
  AppBar,
  Box,
  CircularProgress,
  Container,
  Drawer,
  FormControl,
  IconButton,
  InputLabel,
  List,
  ListItemButton,
  ListItemIcon,
  ListItemText,
  MenuItem,
  Select,
  Stack,
  Toolbar,
  Tooltip,
  Typography,
} from '@mui/material'
import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link, useLocation } from 'react-router-dom'
import { api } from '../api/client'
import { useCurrency, type DisplayCurrency } from './CurrencyContext'

const drawerWidth = 236
const fallbackCurrencies = [{ code: 'RUB', label: 'Российский рубль', symbol: '₽' }] as const
const navigation = [
  { label: 'Обзор', path: '/', icon: <DashboardRoundedIcon /> },
  { label: 'Рынок', path: '/market', icon: <QueryStatsRoundedIcon /> },
  { label: 'Вакансии', path: '/vacancies', icon: <WorkOutlineRoundedIcon /> },
]

interface AppShellProps {
  children: ReactNode
  mode: 'light' | 'dark'
  onToggleMode: () => void
}

export function AppShell({ children, mode, onToggleMode }: AppShellProps) {
  const [mobileOpen, setMobileOpen] = useState(false)
  const location = useLocation()
  const { currency, setCurrency } = useCurrency()
  const currencies = useQuery({
    queryKey: ['currencies'],
    queryFn: ({ signal }) => api.currencies(signal),
    staleTime: 30 * 60 * 1000,
  })
  const currencyOptions = useMemo(() => {
    if (!currencies.data) {
      return currency === 'RUB'
        ? fallbackCurrencies
        : [...fallbackCurrencies, { code: currency, label: currency, symbol: currency }]
    }
    return currencies.data.rates.filter((item) => item.available)
  }, [currencies.data, currency])
  const selectedRate = currencies.data?.rates.find((item) => item.code === currency)
  const rateLabel =
    currency === 'RUB'
      ? 'Базовая валюта зарплат — RUB. Пересчёт не применяется.'
      : selectedRate?.rate_date
        ? `Курс ЦБ на ${selectedRate.rate_date}. Приблизительный официальный дневной курс; не live-курс и не курс выплаты.`
        : 'Курс ЦБ недоступен. Зарплаты остаются доступны в RUB.'

  useEffect(() => {
    if (currencies.isError && currency !== 'RUB') {
      setCurrency('RUB')
      return
    }
    if (
      currencies.data &&
      !currencies.data.rates.some((item) => item.code === currency && item.available)
    ) {
      setCurrency('RUB')
    }
  }, [currencies.data, currencies.isError, currency, setCurrency])

  const drawer = (
    <Stack sx={{ height: '100%', bgcolor: 'background.paper' }}>
      <Toolbar sx={{ px: 2.5 }}>
        <Box
          sx={{
            width: 36,
            height: 36,
            borderRadius: 2,
            bgcolor: 'primary.main',
            color: 'primary.contrastText',
            display: 'grid',
            placeItems: 'center',
            fontWeight: 800,
          }}
        >
          LP
        </Box>
        <Box sx={{ ml: 1.5 }}>
          <Typography variant="subtitle1" sx={{ fontWeight: 800, lineHeight: 1.1 }}>
            IT Labor Pulse
          </Typography>
          <Typography variant="caption" color="text.secondary">
            Аналитика вакансий
          </Typography>
        </Box>
      </Toolbar>
      <List sx={{ px: 1.5, pt: 2 }}>
        {navigation.map((item) => (
          <ListItemButton
            key={item.path}
            component={Link}
            to={item.path}
            selected={location.pathname === item.path}
            onClick={() => setMobileOpen(false)}
            sx={{ borderRadius: 2, mb: 0.5 }}
          >
            <ListItemIcon sx={{ minWidth: 40 }}>{item.icon}</ListItemIcon>
            <ListItemText primary={item.label} />
          </ListItemButton>
        ))}
      </List>
      <Box sx={{ mt: 'auto', p: 2.5 }}>
        <Typography variant="caption" color="text.secondary">
          Phase 1 · данные вакансий
        </Typography>
      </Box>
    </Stack>
  )

  return (
    <Box sx={{ display: 'flex', minHeight: '100vh' }}>
      <AppBar
        position="fixed"
        elevation={0}
        color="inherit"
        sx={{
          width: { md: `calc(100% - ${drawerWidth}px)` },
          ml: { md: `${drawerWidth}px` },
          borderBottom: 1,
          borderColor: 'divider',
          backdropFilter: 'blur(12px)',
          bgcolor: 'surface.elevated',
        }}
      >
        <Toolbar>
          <IconButton
            aria-label="Открыть навигацию"
            edge="start"
            onClick={() => setMobileOpen(true)}
            sx={{ mr: 2, display: { md: 'none' } }}
          >
            <MenuRoundedIcon />
          </IconButton>
          <Typography
            variant="body2"
            color="text.secondary"
            sx={{ flexGrow: 1, display: { xs: 'none', sm: 'block' } }}
          >
            Рынок IT в России
          </Typography>
          <FormControl
            size="small"
            sx={{
              width: { xs: 148, sm: 210 },
              mr: { xs: 0.5, sm: 1 },
              ml: { xs: 'auto', sm: 0 },
              flexShrink: 0,
            }}
          >
            <InputLabel id="salary-currency-label">Валюта зарплат</InputLabel>
            <Select
              labelId="salary-currency-label"
              label="Валюта зарплат"
              aria-describedby="salary-currency-description"
              value={currency}
              onChange={(event) => setCurrency(event.target.value as DisplayCurrency)}
              renderValue={(code) => {
                const option = currencyOptions.find((item) => item.code === code)
                return option ? `${option.code} · ${option.symbol}` : code
              }}
              endAdornment={
                currencies.isLoading ? (
                  <CircularProgress
                    size={16}
                    aria-label="Загрузка курсов валют"
                    sx={{ mr: 2.5 }}
                  />
                ) : undefined
              }
            >
              {currencyOptions.map((item) => (
                <MenuItem key={item.code} value={item.code}>
                  {item.code} · {item.label} · {item.symbol}
                </MenuItem>
              ))}
            </Select>
            <Box
              component="span"
              id="salary-currency-description"
              sx={{
                position: 'absolute',
                width: 1,
                height: 1,
                p: 0,
                m: -1,
                overflow: 'hidden',
                clip: 'rect(0 0 0 0)',
                whiteSpace: 'nowrap',
                border: 0,
              }}
            >
              {rateLabel}
            </Box>
          </FormControl>
          {currencies.isError && (
            <Tooltip title="Курсы временно недоступны — зарплаты показаны в RUB.">
              <Box
                role="status"
                aria-label="Курсы валют временно недоступны; используется RUB"
                sx={{ display: 'flex', mr: 0.5 }}
              >
                <WarningAmberRoundedIcon
                  color="warning"
                  fontSize="small"
                  aria-hidden="true"
                />
              </Box>
            </Tooltip>
          )}
          <Tooltip title={mode === 'light' ? 'Тёмная тема' : 'Светлая тема'}>
            <IconButton aria-label="Переключить тему" onClick={onToggleMode}>
              {mode === 'light' ? <DarkModeOutlinedIcon /> : <LightModeOutlinedIcon />}
            </IconButton>
          </Tooltip>
        </Toolbar>
      </AppBar>

      <Box component="nav" sx={{ width: { md: drawerWidth }, flexShrink: { md: 0 } }}>
        <Drawer
          variant="temporary"
          open={mobileOpen}
          onClose={() => setMobileOpen(false)}
          ModalProps={{ keepMounted: true }}
          sx={{
            display: { xs: 'block', md: 'none' },
            '& .MuiDrawer-paper': { width: drawerWidth },
          }}
        >
          {drawer}
        </Drawer>
        <Drawer
          variant="permanent"
          sx={{
            display: { xs: 'none', md: 'block' },
            '& .MuiDrawer-paper': { width: drawerWidth, boxSizing: 'border-box' },
          }}
          open
        >
          {drawer}
        </Drawer>
      </Box>

      <Box component="main" sx={{ flexGrow: 1, minWidth: 0 }}>
        <Toolbar />
        <Container maxWidth="xl" sx={{ py: { xs: 3, md: 4 } }}>
          {children}
        </Container>
        <Box
          component="footer"
          sx={{ borderTop: 1, borderColor: 'divider', bgcolor: 'surface.muted', px: 3, py: 2.5 }}
        >
          <Typography variant="caption" color="text.secondary">
            Данные вакансий: HeadHunter. Медианные зарплаты — оценка по указанным
            в вакансиях значениям (offered), приведённым к net упрощённо; это не
            опрос и не офер кандидату. Соблюдаются условия использования источника.
          </Typography>
        </Box>
      </Box>
    </Box>
  )
}
