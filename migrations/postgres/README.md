# PostgreSQL migrations (golang-migrate)

Инструмент: **golang-migrate** ([ADR 002](../../docs/architecture/adr/002-migrate-tool.md)).

## Naming

```text
NNNNNN_description.up.sql
NNNNNN_description.down.sql
```

Пример: `000001_init.up.sql` / `000001_init.down.sql`.

## Apply

Нужен доступный Postgres: **cloud `DATABASE_URL`** или контейнер `make up-local-pg` / `make up-local`. Redis для migrate не обязателен.

```bash
make migrate-up
# откат одной миграции (только local/dev):
make migrate-down
```

`make migrate-up` читает `.env`: remote/cloud `DATABASE_URL` — as-is; если DSN на `localhost` — ходит в сервис `postgres` через сеть `lma_net`.

Эквивалент через Docker-образ (local-pg):

```bash
docker run --rm --network lma_net \
  -v "$PWD/migrations/postgres:/migrations:ro" \
  migrate/migrate:v4.18.1 \
  -path=/migrations \
  -database "postgres://lma:lma_local_password@postgres:5432/lma?sslmode=disable" \
  up
```

Cloud (подставь свой URL из `.env`, не коммить):

```bash
docker run --rm \
  -v "$PWD/migrations/postgres:/migrations:ro" \
  migrate/migrate:v4.18.1 \
  -path=/migrations \
  -database "postgres://USER:PASSWORD@HOST:5432/DB?sslmode=require" \
  up
```

С хоста (если установлен CLI `migrate`):

```bash
migrate -path migrations/postgres -database "$DATABASE_URL" up
```

Пароль и cloud DSN — только в `.env`, не коммитьте реальные значения.

## Status

Пока схема Phase 0–1 не зафиксирована в SQL — каталог пустой (только этот README).
Миграции появятся вместе с сервисами Query/Ingest.
