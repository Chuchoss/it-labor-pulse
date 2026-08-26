# Architecture Decision Records (ADR)

Короткие решения для LMA / IT Labor Market Analytics. Формат: Context → Decision → Consequences.

| ADR | Тема |
|-----|------|
| [001](./001-json-vs-avro.md) | Формат Kafka messages: JSON vs Avro |
| [002](./002-migrate-tool.md) | Инструмент миграций PostgreSQL |
| [003](./003-redpanda-local.md) | Redpanda как локальный брокер (optional) |
| [004](./004-bff-vs-direct-query.md) | BFF vs прямой доступ UI → Query |
| [005](./005-clickhouse-phase2.md) | ClickHouse не в Phase 1 |
| [006](./006-cache-strategy.md) | Cache-aside + version bump |
| [007](./007-multi-source-trend-signals.md) | Multi-source «Тенденции» vs vacancy-only metrics |
| [008](./008-github-pages-openapi.md) | GitHub Pages + Redoc/Swagger UI для OpenAPI |
| [009](./009-otel-loki-tempo.md) | OpenTelemetry + Loki + Tempo (корреляция `trace_id`) |
| [010](./010-api-gateway.md) | MVP: public BFF; отдельный gateway — Target |
| [011](./011-phase1-market-snapshots.md) | Phase 1 market snapshots из полного all-IT cycle |
| [012](./012-dashboard-ranking-scopes.md) | Отдельные listing/management scopes и taxonomy языков |
| [013](./013-daily-discovery-snapshots.md) | Дневной search discovery отдельно от detail hydration |
| [014](./014-official-fx-and-source-links.md) | Официальные дневные FX и source-neutral ссылки |
| [015](./015-personal-assistant-ai-telegram.md) | Персональный assistant, optional AI и Telegram |

Новый ADR: скопировать структуру, следующий номер, ссылка в эту таблицу.
Связь с обзором: [00-overview.md](../00-overview.md).
