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

В `.env` задайте `DATABASE_URL` (**Supabase**) и `REDIS_URL` (cloud Redis/Valkey, обычно `rediss://`; **preferred для РФ — Yandex Managed Valkey**). Секреты не коммитить. Upstash может быть недоступен из РФ. Local Docker Redis — optional fallback.

### Phase 0–1 API (BFF)

```bash
make run-bff   # public :8080

curl -s http://localhost:8080/api/v1/health
curl -s "http://localhost:8080/api/v1/dashboard/summary?from=2026-07-01&to=2026-08-01"
```

Публичный вход — `BFF_HTTP_ADDR` (default `:8080`). Phase 1 read-маршруты читают PostgreSQL и требуют `DATABASE_URL`; Redis пока опционален. Отдельный gateway — Target Phase 3+ ([ADR 010](./docs/architecture/adr/010-api-gateway.md)). Health пингует настроенные PostgreSQL и Redis (`checks.*`, `status: degraded` при недоступности; процесс не падает). См. [локальный DX](./docs/architecture/12-local-dev.md) · [контракт API](./api/openapi.yaml).

### Phase 1 ingest (HH → normalize → Postgres)

```bash
make migrate-up
# в .env: DATABASE_URL, HH_USER_AGENT (идентифицирующая строка с контактом)
make ingest-hh              # live HH API
make ingest-hh-fixture      # testdata/hh, без сети к HH
```

Сервис: `apps/ingest` (adapter `internal/hh` → `libs/go-common/normalize` → UPSERT). Параметры: `INGEST_DEFAULT_AREA`, `INGEST_DEFAULT_TEXT`, `INGEST_MAX_PAGES`, `INGEST_PAGE_DELAY_MS`. Живой вызов HH в CI не требуется — unit-тесты на фикстурах/`httptest`.

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
| Архитектура, OpenAPI/proto, HH-фикстуры, cloud/local infra, Phase 0–1 BFF read API, shared normalize, Phase 1 HH ingest → PG (`apps/ingest`) | Web UI, Redis cache и дальше по [фазам](./docs/architecture/00-overview.md) |

## Attribution

Данные вакансий принадлежат площадкам (HH и др.). Соблюдайте ToS, лимиты и User-Agent. Зарплаты на дашборде — оценка по полям salary в вакансиях, не опросы.
