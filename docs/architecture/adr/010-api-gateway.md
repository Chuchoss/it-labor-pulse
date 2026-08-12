# ADR 010: Отдельный API Gateway (edge) + BFF (product API)

## Context

Публичный HTTP раньше описывался как «BFF / API Gateway» на `:8080`. Нужен явный edge-слой (TLS позже, CORS, rate-limit, routing, корреляция) без смешивания с агрегацией DTO под UI. Альтернатива — оставить только BFF как perimeter.

## Decision

**Держим оба сервиса:**

| Сервис | Роль | Порт (local) |
|--------|------|--------------|
| `gateway` | Edge: reverse proxy `/api/*`, CORS, rate-limit stub, `request_id`/`traceparent`, позже TLS/auth stub | `:8080` (публичный) |
| `bff` | Backend-for-Frontend: OpenAPI business routes, агрегация, вызовы Query/Ingest | `:8081` (internal) |

```
Client (React) → gateway (:8080) → bff (:8081) → query / ingest (internal)
```

- Бизнес-логику и OpenAPI-handlers **не** кладём в gateway.
- Ingest/Query **не** публикуем через gateway напрямую — только через BFF.
- Контракт `api/openapi.yaml` описывает **BFF API**; gateway прозрачно проксирует `/api/*`.
- ADR 004 остаётся в силе: UI не ходит в Query напрямую; меняется только то, что публичный hop — gateway, а не BFF.

## Consequences

- (+) Чистое разделение edge vs product API; проще позже добавить auth/WAF/canary на gateway
- (+) BFF остаётся местом UI-агрегации без «раздувания» edge
- (−) Лишний hop (+latency) — для MVP приемлемо
- (−) Local DX: нужны два процесса (`make run-bff` + `make run-gateway`) или Compose profile
- Query HTTP debug переносится на `:8083`, чтобы не конфликтовать с BFF `:8081`
