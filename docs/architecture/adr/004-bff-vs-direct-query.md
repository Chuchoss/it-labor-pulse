# ADR 004: UI ходит только в BFF (не напрямую в Query)

## Context

Query отдаёт аналитику (gRPC + debug HTTP). Можно ли React вызывать Query напрямую и упростить стек?

## Decision

**UI не ходит в Query напрямую.** Публичный perimeter MVP — **BFF** REST (`:8080`).  
Query gRPC/HTTP — ClusterIP / localhost only. BFF адаптирует DTO; edge-политики (CORS/rate-limit/auth) — в BFF до появления optional gateway ([ADR 010](./010-api-gateway.md)).

## Consequences

- (+) Один CORS/auth/perimeter на публичном BFF; скрыт внутренний контракт  
- (+) Можно менять gRPC без ломки UI  
- (−) Target: отдельный gateway добавит hop — только при необходимости  
- Запрещено публиковать Query Ingress в prod-like  
