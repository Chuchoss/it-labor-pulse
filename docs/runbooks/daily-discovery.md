# Daily HH discovery

## Семантика

- В `01:00 UTC` process наблюдает предыдущий UTC-день с cutoff `00:00 UTC`.
- Сначала возобновляется старейший `running` cycle. Параллельный день не
  стартует.
- Snapshot публикуется только после всех search pages. Detail hydration и
  skills не блокируют его.
- Если ноутбук выключен, process не работает. После старта собирается последний
  due day; промежуточные дни остаются `missed`, без фиктивного backfill.

## Оценка до live run

Dry plan:

```powershell
go run ./apps/ingest/cmd/ingest -scope it -dry-run
```

Для текущего all-IT плана ожидается около 117 search pages плюс официальный
taxonomy и probe requests. При `INGEST_PAGE_DELAY_MS=350` чистая пауза — около
минуты; с сетью/backoff нормальный SLA значительно меньше 24 часов. Hard ceiling
по умолчанию — 300 HTTP attempts, timeout — 4 часа.

## Запуск

```powershell
# один bounded run/resume
go run ./apps/ingest/cmd/discovery -once

# постоянный daily process
go run ./apps/ingest/cmd/discovery

# отдельная background hydration
$env:INGEST_SCOPE = "it"
go run ./apps/ingest/cmd/scheduler
```

Держите ровно по одному process каждого режима. PostgreSQL advisory locks
предотвращают дубликаты, но лишние process всё равно создают шум.

## Проверка без PII

Проверяйте только агрегаты: status cycle, `completed_pages/expected_pages`,
число observations, `analytics_runs.row_count`, coverage API. Не выводите raw
payload, title, employer, description, URL БД или Authorization.

## Ошибка / 429

Остановите дублирующий process, сохраните checkpoint и дайте встроенному
`Retry-After`/exponential backoff завершиться. Не переводите cycle в `complete`
вручную. Если request ceiling или timeout достигнут, следующий run продолжит
тот же день.
