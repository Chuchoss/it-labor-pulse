# Карта docs и ADR

## Порядок чтения для реализации

`00` → `12` → `21` → `17` → `18` → `02` → `05` → `15` → `13` → `07` → `03`

Тесты: `13-testing.md`, `13a-testing-backend.md`, `13b-testing-frontend-e2e.md`.

Внешние провайдеры (Supabase / Redis candidates / HH…): `21-external-services.md`.

Стиль docs / OpenAPI / proto: `22-documentation-style.md`.

Tracing / поиск по `trace_id`: `23-observability-tracing.md`.

## Ключевые ADR

| ADR | Решение |
|-----|---------|
| 001 | Kafka messages: JSON (не Avro) на старте |
| 002 | PG migrations: golang-migrate |
| 003 | Local broker: Redpanda optional |
| 004 | UI → BFF, не напрямую в Query |
| 005 | ClickHouse с Phase 2, не Phase 1 |
| 006 | Cache-aside + version bump |
| 007 | Multi-source Perspectives / «Тенденции» vs vacancy-only |
| 008 | GitHub Pages + Redoc/Swagger UI для OpenAPI |
| 009 | OpenTelemetry + Loki + Tempo (`trace_id` correlation) |

## Шаблон ADR

```markdown
# ADR NNN: Заголовок

## Context
…

## Decision
…

## Consequences
- (+) …
- (−) …
```

После создания: добавить строку в `docs/architecture/adr/README.md`.

## Канонические пути контрактов

- OpenAPI: `api/openapi.yaml`
- Proto: `libs/proto/lma/`
- Env names: `.env.example`
- HH fixtures: `testdata/hh/`
