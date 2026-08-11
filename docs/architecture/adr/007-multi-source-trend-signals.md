# ADR 007: Multi-source trend signals vs vacancy-only metrics

## Context

MVP (Phase 1) уже даёт salary/demand time series по вакансиям (`/trends/salaries`, `/trends/demand`). Продукт хочет секцию **«Тенденции» (Perspectives)** — понять, какие IT-направления выглядят перспективнее, используя не только job boards, но и образовательные платформы, новости и статьи.

Риск: смешать vacancy OLTP/OLAP модель с разнородными сигналами или обещать «прогноз будущего» без методики.

## Decision

1. **Vacancy-only metrics** остаются каноном Phase 1–4 для дашборда зарплат/спроса; экран `/trends` не переименовывается в «Тенденции».
2. **Perspectives** — отдельный Target-контур (Phase 5): adapter → `NeutralTrendSignalV1` → `trend_signals` → aggregator → `trend_scores_daily` → API/UI.
3. Composite score — **версионируемая эвристика** (`score_version`, веса в конфиге), с обязательным disclaimer в UI; не ML-prophecy.
4. Источники edu/news — только при разрешённом API/RSS/ToS; тот же adapter pattern, что для job sources ([08](../08-integrations-and-extensibility.md)).
5. Kafka topic `signals.raw` — опциональный foundation с Phase 2; обязательные collectors — не раньше Phase 5.

## Consequences

- (+) MVP Phase 0–1 не раздувается edu/news scope  
- (+) Job demand переиспользуется как одна нога composite score  
- (+) Этичные границы (no forbidden scraping) зафиксированы до реализации  
- (−) До Phase 5 UI «Тенденции» отсутствует; не обещать в Phase 1 demo  
- (−) Нужны отдельные таблицы/миграции и scoring job (не писать миграции до Phase 5)
