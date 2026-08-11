# Runbook: Ingest failed / partial

Когда `ingest_runs.status` ∈ {`failed`, `partial`} или алерт «Ingest failed» ([11-observability-security.md](../architecture/11-observability-security.md)).

## Симптомы

- Admin API: `GET /api/v1/admin/ingest/runs/{run_id}` → `failed` / `partial`
- Метрики: `ingest_runs_total{status="failed"}`, рост `hh_429_total`, ошибки produce
- Логи ingest: `ingest_run_id=... level=error`

## Быстрая диагностика (5 мин)

1. Прочитать `error_message` и `stats` run (`fetched`, `published`, `errors`).
2. Классифицировать:

| Паттерн | Вероятная причина |
|---------|-------------------|
| Сразу fail, 0 fetched | UA / auth / сеть до HH |
| Много 429 | Rate limit, слишком агрессивный crawl |
| fetched ≫ published | Kafka/Redpanda недоступен (Phase 2) |
| partial, errors > 0 | Отдельные страницы/вакансии; см. `ingest_run_errors` |
| 409 на старте | Lock занят — [cache-and-locks.md](./cache-and-locks.md) |
| Run повторяет ту же page | Проверить `ingest_checkpoints`: cursor не должен продвигаться после failed page |

3. Проверить env: `HH_USER_AGENT` непустой, `HH_BASE_URL`.
4. `curl -I` / тестовый GET к HH с тем же UA (осторожно с лимитами).
5. Phase 2: брокеры `KAFKA_BROKERS`, consumer lag не обязателен для produce fail.

## Действия

### A. User-Agent / 403

1. Исправить `HH_USER_AGENT` в Secret/`.env`.
2. Пересоздать ingest.
3. Повторить **incremental** с малым `INGEST_MAX_PAGES`.

### B. 429

1. Не ретраить сразу full scan.
2. Увеличить `INGEST_PAGE_DELAY_MS`, max pages ↓.
3. Дождаться окна лимита; перезапуск incremental с `date_from`.

### C. Kafka produce fail (Phase 2)

1. Проверить Redpanda/Kafka up + topic `vacancies.raw.hh`.
2. После восстановления — новый run incremental (idempotent normalize).
3. Не чистить PG вручную.

### D. Partial (часть ошибок)

1. Смотреть `ingest_run_errors` (stage fetch/produce).
2. Если poison single id — можно игнорить; массовые parse errors → фикстура/баг адаптера.
3. Повтор incremental обычно безопасен: Phase 1 сохраняет cursor только после успешной normalize+store всей page; dedup `(source, external_id)` делает повтор идемпотентным.

### E. После успеха

1. Убедиться `status=success` или приемлемый `partial`.
2. Cache bust при необходимости: [cache-and-locks.md](./cache-and-locks.md).
3. Smoke: `GET /api/v1/dashboard/summary`.

## Эскалация / когда копать код

- Стабильный parse fail на валидных HH payload → баг adapter + обновить `testdata/hh`.
- Потеря cursor / повторная полная выгрузка каждый раз → проверить `ingest_checkpoints` и правило commit после успешной page.
- Lock expire mid-run → runbook locks + heartbeat.

## Связанные доки

- [18-logging-and-incidents.md](../architecture/18-logging-and-incidents.md) — общий playbook инцидентов, поиск логов  
- [12-local-dev.md](../architecture/12-local-dev.md)  
- [02-services.md](../architecture/02-services.md) failure modes  
- [07-messaging.md](../architecture/07-messaging.md)  
