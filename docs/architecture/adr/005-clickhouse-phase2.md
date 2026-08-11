# ADR 005: ClickHouse только с Phase 2

## Context

Тренды и медианы можно считать в PostgreSQL на малых объёмах. Поднимать CH в day-1 увеличивает DX и ops cost.

## Decision

**Phase 1 (MVP):** агрегаты и короткие тренды из PostgreSQL (+ Redis cache).  
**Phase 2:** ввести ClickHouse (`fact_vacancy_snapshot`), Query читает длинные тренды из CH. Публичный REST **не меняется**.

## Consequences

- (+) Быстрее путь к рабочему дашборду  
- (−) Риск «временного» SQL в PG обрасти — ограничить период/объём, завести issue на CH  
- Миграции CH отдельным каталогом; Compose profile `olap`  
