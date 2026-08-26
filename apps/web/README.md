# Web

React + TypeScript SPA для Phase 1. Браузер обращается только к публичному
REST BFF `/api/v1`; Vite проксирует этот путь на `http://localhost:8080`.

## Локальный запуск

```powershell
# терминал 1, из корня репозитория
go run ./apps/bff/cmd/bff

# терминал 2
Set-Location apps/web
npm ci
npm run dev
```

Открыть <http://localhost:3000>. Для другого адреса BFF создайте локальный
`apps/web/.env.local` (файл игнорируется git):

```text
VITE_API_BASE_URL=http://localhost:18080/api/v1
```

При прямом URL BFF должен разрешать origin браузера; для обычной разработки
предпочтителен same-origin путь `/api/v1` через Vite proxy.

## Проверки

```powershell
npm run lint
npm run typecheck
npm test
npm run build
```

В клиентское окружение разрешены только публичные `VITE_*` значения. Токены HH,
admin token и строки подключения к БД остаются только на сервере.
