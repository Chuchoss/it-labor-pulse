# ADR 004: UI ходит только в BFF (не напрямую в Query)

## Context

Query отдаёт аналитику (gRPC + debug HTTP). Можно ли React вызывать Query напрямую и упростить стек?

## Decision

**UI не ходит в Query напрямую.** Публичный perimeter — **gateway** (`:8080`) → **BFF** (`:8081`) REST.  
Query gRPC/HTTP — ClusterIP / localhost only. BFF адаптирует DTO; edge (CORS, rate-limit stub, корреляция) — на gateway ([ADR 010](./010-api-gateway.md)).

## Consequences

- (+) Один CORS/auth/perimeter на gateway; скрыт внутренний контракт  
- (+) Можно менять gRPC без ломки UI  
- (−) Лишний hop gateway→BFF (+latency) — для MVP приемлемо  
- Запрещено публиковать Query Ingress в prod-like  
