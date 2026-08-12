# ADR 010: API Gateway — deferred; MVP = public BFF

## Status

**Revised (MVP).** Отдельный `gateway` **не** входит в Phase 0–1. Код `apps/gateway` удалён, чтобы не путать local DX.

## Context

Кратко жил вариант «edge gateway `:8080` → internal BFF `:8081`» (reverse proxy, CORS/rate-limit stub, корреляция). Для учебного MVP это давало лишний hop и два процесса без выгоды. Нужен один публичный HTTP вход.

## Decision

### MVP (Phase 0–2, local/dev)

**Один публичный edge = BFF** на `:8080`.

```
Client (React) → BFF (:8080) → query / ingest (internal)
```

| Сервис | Роль | Порт (local) |
|--------|------|--------------|
| `bff` | Публичный REST (OpenAPI), edge-обязанности MVP (CORS/корреляция по мере появления), агрегация DTO | `:8080` (публичный) |
| `query` | Analytics; HTTP debug | `:8083` / gRPC `:9091` |
| `ingest` | Admin/health | `:8082` / gRPC `:9092` |

- UI ходит только в BFF — ADR 004 в силе (не в Query напрямую).
- Контракт `api/openapi.yaml` = публичный BFF API.
- Ingest/Query наружу не публикуем.

### Target (Phase 3+, optional)

Отдельный **`gateway`** как edge перед BFF — когда появятся multi-service prod-like нужды: TLS/auth stub на edge, WAF/canary, rate-limit вне product API, несколько upstreams.

```
Client → gateway (:8080) → bff (internal) → query / ingest
```

До этого момента gateway — только docs/Target, без обязательного кода в репо.

## Consequences

- (+) Проще local DX: `make run-bff` → `curl localhost:8080/api/v1/health`
- (+) Нет путаницы портов gateway/BFF
- (−) Edge-политики (auth/rate-limit) живут в BFF до появления отдельного gateway
- Query HTTP остаётся на `:8083`, ingest на `:8082` (как в [02-services.md](../02-services.md))
