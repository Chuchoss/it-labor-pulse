# Labor Market Analytics

Проект аналитики IT-рынка труда: сбор вакансий (сначала HeadHunter), нормализация, анализ зарплат и спроса по ролям/регионам, тренды; позже — AI-анализ.

## Стек (target)

- **Frontend:** React  
- **Backend:** Go  
- **Messaging:** Kafka (local: Redpanda)  
- **RPC:** gRPC (internal)  
- **OLTP:** PostgreSQL  
- **OLAP:** ClickHouse  
- **Cache:** Redis  
- **Deploy:** Kubernetes + CI/CD  
- **Later:** AI model integration  

Реализация кода — поэтапно; сначала зафиксирована архитектура.

## Quickstart

Локальный DX (Phase 0–1): **[docs/architecture/12-local-dev.md](./docs/architecture/12-local-dev.md)**.  
Секреты: **[docs/architecture/17-secrets-management.md](./docs/architecture/17-secrets-management.md)**.

**Рекомендуется (cloud-everything):** облачный PostgreSQL (`DATABASE_URL`) + облачный Redis (`REDIS_URL`, часто `rediss://`) — Compose infra не нужна (`make up-cloud`).  
Актуальный выбор провайдеров: [21-external-services.md](./docs/architecture/21-external-services.md) (Postgres → Supabase; Redis — кандидат).  
Локальные контейнеры — опционально: `local-pg`, `local-redis`. Сервисы Go/React ещё не подключены.

```bash
cp .env.example .env
# PowerShell: Copy-Item .env.example .env
# В .env: DATABASE_URL + REDIS_URL из кабинетов (секреты — не коммитить)
# Либо local DSN/URL и make up-local

make up-cloud         # напоминание: cloud URLs, контейнеры не стартуют
# make up-local-redis # если нужен Redis в Docker
# make up-local       # Redis + Postgres в Docker
# make migrate-up     # когда появятся SQL — против DATABASE_URL
```

Облачный PG: [«Облачный PostgreSQL»](./docs/architecture/12-local-dev.md#облачный-postgresql).  
Облачный Redis: [«Облачный Redis»](./docs/architecture/12-local-dev.md#облачный-redis).

Без Make (PowerShell):

```powershell
# Cloud PG + cloud Redis — Compose можно не запускать
# Только local Redis:
docker compose --env-file .env -f deploy/compose/docker-compose.yml --profile local-redis up -d --wait
# Local Redis + local Postgres:
docker compose --env-file .env -f deploy/compose/docker-compose.yml --profile local-redis --profile local-pg up -d --wait
```

Подключение:

| | |
|--|--|
| Postgres (cloud) | `DATABASE_URL` из кабинета, обычно `sslmode=require`; DBeaver + SSL |
| Postgres (local) | `localhost:5432`, user/db `lma`, `make up-local-pg` / `make psql` |
| Redis (cloud) | `REDIS_URL` (`rediss://…` при TLS); не коммитить |
| Redis (local) | `localhost:6379` / `REDIS_URL=redis://localhost:6379/0` (`make redis-cli`) |

Позже: UI `:3000`, API `:8080`. Контракты уже есть: [`api/openapi.yaml`](./api/openapi.yaml), [`libs/proto/lma/`](./libs/proto/lma/).

## API docs

Канон контракта: [`api/openapi.yaml`](./api/openapi.yaml).

Публичный HTML (GitHub Pages, после push в `main`):

| | |
|--|--|
| Redoc | `https://<user-or-org>.github.io/<repo>/` |
| Swagger UI | `https://<user-or-org>.github.io/<repo>/swagger.html` |

Статика: [`docs/api-site/`](./docs/api-site/). Workflow: [`.github/workflows/docs-pages.yml`](./.github/workflows/docs-pages.yml).  
Один раз: Settings → Pages → Source = **GitHub Actions**. Подробнее: [ADR 008](./docs/architecture/adr/008-github-pages-openapi.md), [03-api.md](./docs/architecture/03-api.md).

Локально (нужен интернет — Redoc/Swagger с jsDelivr): скопируй yaml в site и открой HTML, например:

```bash
cp api/openapi.yaml docs/api-site/openapi.yaml
# затем открой docs/api-site/index.html в браузере (или любой static server)
```

## Документация

Полный архитектурный пакет:

👉 **[docs/architecture/](./docs/architecture/)**

Рекомендуемый порядок чтения: [docs/architecture/README.md](./docs/architecture/README.md).

Полезное рядом:

- Нормализация зарплат: [15-normalization-rules.md](./docs/architecture/15-normalization-rules.md)  
- Тесты: [13-testing.md](./docs/architecture/13-testing.md)  
- Frontend IA: [16-frontend.md](./docs/architecture/16-frontend.md)  
- Agent tooling (Cursor rules/skills/hooks): [19-agent-tooling.md](./docs/architecture/19-agent-tooling.md)  
- Code style: [20-code-style.md](./docs/architecture/20-code-style.md)  
- Внешние сервисы / провайдеры: [21-external-services.md](./docs/architecture/21-external-services.md)  
- Стиль docs / OpenAPI: [22-documentation-style.md](./docs/architecture/22-documentation-style.md)  
- Runbooks: [docs/runbooks/](./docs/runbooks/)  
- ADR: [docs/architecture/adr/](./docs/architecture/adr/)  
- HH fixtures: [testdata/hh/](./testdata/hh/)

## Статус

Сейчас: архитектурная документация, OpenAPI/proto stubs, HH-фикстуры и **локальная/cloud infra** (`deploy/compose` — опциональные `local-redis` / `local-pg`; рекомендуется cloud `DATABASE_URL` + `REDIS_URL`). Код сервисов (Go/React) — по фазам из [00-overview.md](./docs/architecture/00-overview.md).

## Дисклеймер

Данные вакансий принадлежат соответствующим площадкам (HH и др.). Проект учебный; при работе с API соблюдайте их ToS, лимиты и требования к User-Agent. Медианные зарплаты на дашборде — оценка по полям salary в вакансиях (offered), не survey-опросы.
