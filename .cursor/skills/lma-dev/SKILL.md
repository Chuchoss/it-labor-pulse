---

name: lma-dev

description: >-

  Локальный workflow Phase 0–1 для LMA: Compose profiles, .env, make targets,

  migrate, health/smoke. Use when setting up local development, Docker Compose,

  make up-mvp, migrations, troubleshooting local DX, or starting Phase 0/1

  implementation.

---



# LMA Local Dev (Phase 0–1)



## Цель



Поднятие MVP-контура за ~15 мин без Kafka/ClickHouse/полного gRPC (это Phase 2+).



## Чеклист старта



```

- [ ] cp .env.example .env

- [ ] Заполнены ADMIN_TOKEN; DATABASE_URL (cloud, sslmode=require) или local POSTGRES_*;

      REDIS_URL (cloud rediss:// или local redis://) или local REDIS_*; HH_USER_AGENT для ingest

- [ ] .env не в git (особенно cloud DATABASE_URL / REDIS_URL)

- [ ] make up-cloud (cloud PG+Redis) ИЛИ make up-local-redis / make up-local

- [ ] make wait-ready → только если local-redis; позже gateway /api/v1/health

- [ ] make migrate-up (когда есть миграции) — по DATABASE_URL

- [ ] UI :3000, public API :8080 (gateway); BFF internal :8081

```



## Profiles



| Profile | Состав | Фаза |

|---------|--------|------|

| `mvp` | позже: bff, query, ingest(+normalize in-process), web | 0–1 apps |

| `local-redis` | optional redis container | 0–1, если не cloud Redis |

| `local-pg` | optional postgres container | 0–1, если не cloud PG |

| `olap` | + clickhouse | перед CH trends |

| `bus` | + redpanda, separate normalizer | 2 |

| `full` | mvp+local-redis+local-pg+olap+bus | почти prod local |



Рекомендуется: cloud/managed Postgres (`DATABASE_URL`) + cloud/managed Redis (`REDIS_URL`). Актуальный выбор провайдеров: `docs/architecture/21-external-services.md`. Файлы Compose: `deploy/compose/` (см. `12-local-dev.md`).



## Команды



```bash

make up-cloud          # cloud PG + cloud Redis — контейнеры не нужны

make up-local-redis    # optional local Redis (alias: up-redis / up-mvp)

# make up-local        # local Redis + local Postgres

make wait-ready        # local-redis health

make migrate-up

# docker compose --env-file .env -f deploy/compose/docker-compose.yml --profile local-redis up -d --wait

```



## Правила реализации Phase 0–1



1. Не поднимай Kafka/CH/gRPC «на всякий случай».

2. Миграции PG — golang-migrate (ADR 002), не raw DDL в приложении.

3. UI только через gateway → BFF; секреты не в клиент.

4. HH adapter + fixtures; ingest уважает UA и 429 backoff.

5. Стиль: `docs/architecture/20-code-style.md`, `.editorconfig`, `.golangci.yml`.

6. Тесты: `docs/architecture/13-testing.md` (PR = unit; production = integration; nightly = E2E).



## Troubleshooting



Симптомы UA 403, порты, migrate — в `docs/architecture/12-local-dev.md`.  

Secrets: `17-secrets-management.md`. Логи: `18-logging-and-incidents.md`.  

Расширенный список make/env: [reference.md](reference.md).


