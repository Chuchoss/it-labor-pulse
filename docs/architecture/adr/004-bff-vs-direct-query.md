# ADR 004: UI ходит только в BFF (не напрямую в Query)

## Context

Query отдаёт аналитику (gRPC + debug HTTP). Можно ли React вызывать Query напрямую и упростить стек?

## Decision

**Единственная публичная поверхность — BFF REST** (`bff :8080`).  
Query gRPC/HTTP — ClusterIP / localhost only. BFF адаптирует DTO, auth stub, edge rate-limit, `request_id`.

## Consequences

- (+) Один CORS/auth/perimeter; скрыт внутренний контракт  
- (+) Можно менять gRPC без ломки UI  
- (−) Лишний hop (+latency) — для MVP приемлемо  
- Запрещено публиковать Query Ingress в prod-like  
