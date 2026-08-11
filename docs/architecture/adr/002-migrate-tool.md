# ADR 002: Инструмент миграций PG — golang-migrate

## Context

Нужны версионированные SQL-миграции PostgreSQL (и отдельно ClickHouse). Кандидаты: `golang-migrate`, `goose`, `atlas`. Нужно зафиксировать один инструмент на репозиторий.

## Decision

**`golang-migrate`** для PostgreSQL как стандарт репозитория.  
ClickHouse — отдельный runner (HTTP client / migrate files в `migrations/clickhouse`), не смешивать драйверы в одном binary обязательно.

## Consequences

- (+) Широкий стандарт, понятный CI Job  
- (+) File-based SQL, reviewable diffs  
- (−) Меньше «Go embed» удобств, чем у goose — приемлемо  
- Не менять инструмент mid-flight без ADR и одного окна миграции  
