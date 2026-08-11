# Архитектурная документация

IT Labor Market Analytics (LMA) — платформа аналитики рынка труда: сбор вакансий (HH и далее другие источники), нормализация, анализ зарплат и спроса по ролям/регионам, тренды; позже — AI-анализ; **Phase 5 Target** — секция «Тенденции» (Perspectives) на multi-source сигналах (jobs + edu + media).

## Целевой стек

| Слой | Технология |
|------|------------|
| Frontend | React |
| Backend | Go |
| Messaging | Kafka (local: Redpanda) |
| Internal RPC | gRPC |
| OLTP | PostgreSQL |
| OLAP | ClickHouse |
| Cache / locks | Redis |
| Deploy | Kubernetes + CI/CD |
| Later | AI model integration |

## Контракты (канонические пути)

| Артефакт | Путь |
|----------|------|
| OpenAPI (публичный REST BFF) | [`api/openapi.yaml`](../../api/openapi.yaml) |
| OpenAPI HTML (GitHub Pages) | [`docs/api-site/`](../api-site/) → [Redoc](https://chuchoss.github.io/it-labor-pulse/) / [Swagger UI](https://chuchoss.github.io/it-labor-pulse/swagger.html) ([ADR 008](./adr/008-github-pages-openapi.md)) |
| Protobuf (internal gRPC) | [`libs/proto/lma/`](../../libs/proto/lma/) |
| Env example | [`.env.example`](../../.env.example) |
| HH fixtures | [`testdata/hh/`](../../testdata/hh/) |

Не использовать устаревшие пути вроде `proto/lma/...` в корне без `libs/`.

## Порядок чтения

| # | Документ | Зачем читать |
|---|----------|-------------|
| 0 | [00-overview.md](./00-overview.md) | Vision, фазы, quality attributes |
| 1 | [01-system-context.md](./01-system-context.md) | C4 context/container, внешние системы |
| 2 | [02-services.md](./02-services.md) | Сервисы, порты, ownership данных |
| 3 | [03-api.md](./03-api.md) | REST + gRPC контракты |
| 4 | [04-sequence-diagrams.md](./04-sequence-diagrams.md) | Ключевые сценарии (ingest, cache, AI) |
| 5 | [05-data-model.md](./05-data-model.md) | PostgreSQL + ClickHouse; Target trend_signals / scores |
| 6 | [06-caching.md](./06-caching.md) | Redis keys, TTL, invalidation |
| 7 | [07-messaging.md](./07-messaging.md) | Kafka topics, DLQ, idempotency |
| 8 | [08-integrations-and-extensibility.md](./08-integrations-and-extensibility.md) | Адаптеры источников, AI, Perspectives / «Тенденции» |
| 9 | [09-deployment.md](./09-deployment.md) | Compose + Kubernetes |
| 10 | [10-cicd.md](./10-cicd.md) | GitHub Actions: ветки developer/production, gate test→deploy, branch protection, миграции |
| 11 | [11-observability-security.md](./11-observability-security.md) | Logs/metrics/traces, ToS, SLO lite; указатели на secrets и logging |
| 12 | [12-local-dev.md](./12-local-dev.md) | Local DX, Compose profiles, make, troubleshooting |
| 13 | [13-testing.md](./13-testing.md) | Стратегия тестов: пирамида, DoD, CI-матрица, план по фазам |
| 13a | [13a-testing-backend.md](./13a-testing-backend.md) | Go: unit, integration (PG/Redis/Kafka), OpenAPI/gRPC contract |
| 13b | [13b-testing-frontend-e2e.md](./13b-testing-frontend-e2e.md) | React (Vitest/MSW) + Playwright E2E, seed |
| 15 | [15-normalization-rules.md](./15-normalization-rules.md) | Salary/FX/roles/remote, offered vs survey |
| 16 | [16-frontend.md](./16-frontend.md) | MVP экраны, states, endpoints; Target «Тенденции» |
| 17 | [17-secrets-management.md](./17-secrets-management.md) | Секреты: инвентарь, local/Compose/K8s/CI, ротация по фазам |
| 18 | [18-logging-and-incidents.md](./18-logging-and-incidents.md) | Логи по фазам, Loki, поиск, playbook инцидентов |
| 19 | [19-agent-tooling.md](./19-agent-tooling.md) | Cursor rules / skills / hooks для агентов |
| 20 | [20-code-style.md](./20-code-style.md) | Единый стиль: Go, TS/React, SQL, proto, commits |
| 21 | [21-external-services.md](./21-external-services.md) | Реестр внешних сервисов / провайдеров (Supabase, Redis candidates, HH…) |
| 22 | [22-documentation-style.md](./22-documentation-style.md) | Единый стиль docs, OpenAPI, proto, ADR и чеклист смены API |
| ADR | [adr/](./adr/) | Короткие архитектурные решения |
| Ops | [../runbooks/](../runbooks/) | Ingest failed, DLQ replay, cache/locks |

**Минимум для старта реализации:** 00 → 12 → 17 → 18 → 02 → 05 → 15 → **13** → 07 → 03.

Перед Phase 1 ingest: прочитать [13-testing.md](./13-testing.md) (test-first / DoD) и фикстуры [`testdata/hh/`](../../testdata/hh/).

**Метки в документах:**

- **MVP** — то, что делаем в первых фазах
- **Target** — целевая архитектура (полный стек)
- **Phase 5 / Perspectives** — «Тенденции» multi-source; ADR [007](./adr/007-multi-source-trend-signals.md)

Код приложения (Go/React) в этом пакете не описан как реализация — только архитектура, контракты, фикстуры и runbooks.
