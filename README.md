# IT Labor Pulse

Аналитика IT-рынка труда: сбор вакансий (сначала HeadHunter), нормализация, зарплаты и спрос по ролям/регионам, тренды.

## Документация

| | |
|--|--|
| **API (Redoc)** | [chuchoss.github.io/it-labor-pulse](https://chuchoss.github.io/it-labor-pulse/) |
| **API (Swagger)** | [Swagger UI](https://chuchoss.github.io/it-labor-pulse/swagger.html) |
| **Архитектура** | [docs/architecture/](./docs/architecture/) · [индекс](./docs/architecture/README.md) |
| **Контракт** | [`api/openapi.yaml`](./api/openapi.yaml) · [`libs/proto/lma/`](./libs/proto/lma/) |

## Стек

React · Go · PostgreSQL · Redis · Kafka · ClickHouse · gRPC · Kubernetes

## Quickstart

```bash
git clone https://github.com/Chuchoss/it-labor-pulse.git
cd it-labor-pulse
cp .env.example .env   # PowerShell: Copy-Item .env.example .env
```

В `.env` задайте `DATABASE_URL` (**Supabase**) и `REDIS_URL` (**Upstash**, `rediss://`). Секреты не коммитить. Local Docker Redis — optional fallback.

### Phase 0 API (BFF)

```bash
make run-bff   # public :8080

curl -s http://localhost:8080/api/v1/health
```

Публичный вход — `BFF_HTTP_ADDR` (default `:8080`). Отдельный gateway — Target Phase 3+ ([ADR 010](./docs/architecture/adr/010-api-gateway.md)). Если заданы `DATABASE_URL` / `REDIS_URL`, health пингует Postgres и Redis (`checks.*`, `status: degraded` при недоступности; процесс не падает). См. [локальный DX](./docs/architecture/12-local-dev.md) · [Upstash](./docs/architecture/12-local-dev.md#облачный-redis).

## Ветки и CI

| Branch | Роль |
|--------|------|
| `developer` | интеграция (default), auto deploy → **dev** после зелёного `test` |
| `production` | релизы, auto deploy → **prod** после `test` (+ Environment) |

Feature → PR в `developer` → PR в `production`. Gate: [`.github/workflows/ci-cd.yml`](./.github/workflows/ci-cd.yml) — сначала job **`test`**, потом deploy. Подробнее: [10-cicd.md](./docs/architecture/10-cicd.md).

## Статус

CI: GitHub Actions (`test` required on PRs).

| Сейчас | Дальше |
|--------|--------|
| Архитектура, OpenAPI/proto, HH-фикстуры, cloud/local infra, Phase 0 BFF (`GET /api/v1/health`) | Query/ingest и дальше по [фазам](./docs/architecture/00-overview.md) |

## Attribution

Данные вакансий принадлежат площадкам (HH и др.). Соблюдайте ToS, лимиты и User-Agent. Зарплаты на дашборде — оценка по полям salary в вакансиях, не опросы.
