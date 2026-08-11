# HH ingest — детали

## Env (канон имён в `.env.example`)

| Variable | Required | Notes |
|----------|----------|-------|
| `HH_USER_AGENT` | yes for ingest | контакт в строке |
| `HH_BASE_URL` | no | default `https://api.hh.ru` |
| `HH_APP_TOKEN` | optional | server-only |

## Логи (ingest)

Поля: `service=ingest`, `ingest_run_id`, `source=hh`, `vacancy_external_id` при событии о вакансии.  
Не логировать полный raw JSON на info; secrets redact.

## Admin trigger (local)

```bash
curl -X POST http://localhost:8080/api/v1/admin/ingest/runs \
  -H "Content-Type: application/json" \
  -H "X-Admin-Token: $ADMIN_TOKEN" \
  -d '{"source":"hh","mode":"incremental","params":{"area":"1","text":"golang"}}'
```

## Attribution

В UI и README: данные вакансий принадлежат площадкам (HH и др.); соблюдать ToS.
