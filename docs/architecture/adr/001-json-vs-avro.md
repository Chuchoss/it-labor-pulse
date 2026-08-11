# ADR 001: JSON envelope для Kafka (не Avro)

## Context

Нужен формат событий `vacancies.raw.*` / `vacancies.normalized` между ingest и normalizer. Avro + Schema Registry даёт строгую эволюцию, но усложняет local DX и ops для учебного проекта.

## Decision

**JSON** с envelope (`schema_version`, `message_id`, `source`, `payload`) и Kafka headers. Avro — опциональный Target при росте числа consumers.

## Consequences

- (+) Простые фикстуры, `kafbat`/`rpk` без registry  
- (+) Быстрый старт Phase 2  
- (−) Слабее совместимость схем — дисциплина `schema_version` и tolerant readers обязательна  
- Миграция на Avro потребует dual-read период  
