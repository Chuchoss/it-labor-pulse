# Runbook: дневные курсы Банка России

Источник — официальный XML Банка России:
[`XML_daily.asp`](https://www.cbr.ru/development/SXML/). `date_req` передаётся
как `dd/mm/yyyy`; дата в `ValCurs.Date` является датой набора. Выходные и
праздники могут вернуть последнюю зарегистрированную дату. Worker сохраняет
именно дату ответа и нормализует `rub_per_unit = Value / Nominal`.

Это дневной справочный курс, не intraday/realtime, не финансовая рекомендация и
не гарантированный курс платежа.

## Запуск

```bash
make fx-sync
go run ./apps/fx/cmd/sync -date 2026-08-25
go run ./apps/fx/cmd/sync -from 2026-08-01 -to 2026-08-25
make run-fx-scheduler
```

Scheduler запускается одним процессом в `FX_SYNC_UTC_HOUR` (default 06:00 UTC).
PostgreSQL advisory lock исключает дубль. Default run получает сегодня,
предыдущие 7 дней и bounded missing dates текущего покрытия.

## Проверка без vacancy content

```sql
SELECT rate_date, quote_currency, count(*)
FROM fx_rates
GROUP BY rate_date, quote_currency
ORDER BY rate_date DESC, quote_currency;

SELECT status, fetched_dates, upserted_rates, started_at, finished_at
FROM fx_sync_runs
ORDER BY started_at DESC
LIMIT 5;
```

Логи reconciliation содержат только counts по вакансиям/observations и missing
rates/source links. DSN, salary values, titles, employer и raw payload не
логируются.

## Ошибки

- provider timeout/5xx: bounded retry с exponential backoff и jitter;
- rate отсутствует более 7 дней: raw salary сохраняется, canonical salary
  остаётся недоступной;
- BFF никогда не вызывает ЦБ напрямую и продолжает работать с последним
  нестарым кэшем PostgreSQL.
