# PostgreSQL migrations (golang-migrate)

Инструмент: **golang-migrate** ([ADR 002](../../docs/architecture/adr/002-migrate-tool.md)).  
Схема Phase 1 — [05-data-model.md](../../docs/architecture/05-data-model.md).

## Naming

```text
NNNNNN_description.up.sql
NNNNNN_description.down.sql
```

## Files (Phase 1)

| Version | Contents |
|---------|----------|
| `000001_sources` | `sources` + seed `hh` |
| `000002_dictionaries` | `regions`, `region_external_ids`, `roles`, `role_aliases`, `skills`, `skill_aliases`, `employers` |
| `000003_vacancies` | `vacancies`, `vacancy_skills` |
| `000004_ingest` | `ingest_runs`, `ingest_checkpoints`, `ingest_run_errors` |
| `000005_hh_role_scope` | Утверждённые HH role groups и reconciliation scope |
| `000006_market_analytics` | `ingest_cycles`, analytics runs, daily/weekly demand snapshots |

UUID PK: `DEFAULT gen_random_uuid()` (Supabase / PG 13+). AI / Perspectives tables — не здесь (Phase 4–5).

## Apply

Нужен доступный Postgres: **cloud `DATABASE_URL`** или контейнер `make up-local-pg` / `make up-local`. Redis для migrate не обязателен.

```bash
make migrate-up
# откат одной миграции (только local/dev):
make migrate-down
```

`make migrate-up` читает `.env`: remote/cloud `DATABASE_URL` — as-is; если DSN на `localhost` — ходит в сервис `postgres` через сеть `lma_net`.

### Cloud / Supabase

- Session pooler (`:5432`, user `postgres.<ref>`) — предпочтительно; `make migrate-up` добавит `sslmode=require` и `connect_timeout=60`, если их нет в URL.
- Transaction pooler (`:6543`) часто ломает `schema_migrations` / advisory locks — не использовать для migrate.
- SSL: `?sslmode=require` в `DATABASE_URL`.
- Если `migrate` завис на `pg_advisory_lock` / DDL и оставил `idle in transaction` (часто `(aborted)`), **роль pooler обычно не может** `pg_terminate_backend`. Выполни в **Supabase → SQL Editor** (privileged):

```sql
SELECT pid, state, left(query, 120) AS query
FROM pg_stat_activity
WHERE datname = current_database()
  AND pid <> pg_backend_pid()
  AND (
    state ILIKE 'idle in transaction%'
    OR query ILIKE '%schema_migrations%'
    OR query ILIKE '%pg_advisory_lock%'
    OR query ILIKE '%CREATE TABLE sources%'
  );

SELECT pg_terminate_backend(pid)
FROM pg_stat_activity
WHERE datname = current_database()
  AND pid <> pg_backend_pid()
  AND (
    state ILIKE 'idle in transaction%'
    OR query ILIKE '%schema_migrations%'
    OR query ILIKE '%pg_advisory_lock%'
    OR query ILIKE '%CREATE TABLE sources%'
  );
```

На Windows `lib/pq` (`postgres://`) к session pooler иногда зависает — предпочтительнее образ `migrate/migrate` или CLI, собранный с тегом `pgx5`, и URL вида `pgx5://...`.  
Если видишь `EMAXCONNSESSION` / `max clients reached in session mode` — session pooler (free tier ~15) забит orphaned-клиентами; тот же `pg_terminate_backend` в SQL Editor обязателен, иначе новые подключения не откроются.  
Затем снова `make migrate-up` (или CLI `migrate ... up`).  
Проверка: `SELECT table_name FROM information_schema.tables WHERE table_schema='public' ORDER BY 1;`

Эквивалент через Docker-образ:

```bash
# local-pg
docker run --rm --network lma_net \
  -v "$PWD/migrations/postgres:/migrations:ro" \
  migrate/migrate:v4.18.1 \
  -path=/migrations \
  -database "postgres://lma:lma_local_password@postgres:5432/lma?sslmode=disable" \
  up

# cloud — URL из .env, не коммить
docker run --rm \
  -v "$PWD/migrations/postgres:/migrations:ro" \
  migrate/migrate:v4.18.1 \
  -path=/migrations \
  -database "$DATABASE_URL" \
  up
```

С хоста (если установлен CLI `migrate`):

```bash
migrate -path migrations/postgres -database "$DATABASE_URL" up
```

Пароль и cloud DSN — только в `.env`, не коммитьте реальные значения.

## Rules

1. Не править уже применённые файлы — только новая версия  
2. Forward + backward (`.up` / `.down`) где безопасно  
3. Версия схемы — таблица `schema_migrations` (golang-migrate)
