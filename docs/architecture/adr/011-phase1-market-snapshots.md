# ADR 011: Phase 1 market snapshots в PostgreSQL

## Context

Текущее состояние `vacancies` не позволяет достоверно восстановить исторический
`active_count`. ClickHouse и Kafka относятся к Phase 2, но экран рынка нужен в
Phase 1. Успешный bounded ingest-run не доказывает полный охват: all-IT crawl
состоит из нескольких возобновляемых role/date partitions.

## Decision

- Добавить долговечный `ingest_cycles`: статус `complete` выставляется только
  после обработки всех partitions сохранённого all-IT checkpoint.
- Запускать отдельный Go process/package `apps/analytics`, не сетевой сервис.
- После завершения cycle scheduler вызывает daily snapshot; самостоятельный
  worker также читает пропущенные complete markers.
- Хранить дневные и недельные vacancy-demand snapshots в PostgreSQL с
  `method_version=vacancy_demand_v1`.
- `active_count` и `published_count` в BFF читать только из snapshots.
  Реконструкции истории из текущих строк и ручной подстройки значений нет.
- Weekly rollup строить только из daily rows. Неделя с менее чем семью днями
  остаётся `complete=false` и по умолчанию не попадает в ряд.
- Снимки охватывают vacancy demand Phase 1 и не являются multi-source
  Perspectives Phase 5.

## Consequences

- (+) Исторический спрос воспроизводим, идемпотентен и имеет provenance cycle/run.
- (+) Неполный bounded batch не создаёт точку на графике.
- (+) Phase 1 не требует Kafka, ClickHouse или отдельного public/internal API.
- (+) Методика может меняться через новую `method_version`.
- (−) Первый ряд появится только после полного all-IT cycle.
- (−) Полная неделя требует семь complete daily snapshots.
- (−) До Phase 2 PostgreSQL выполняет ограниченную агрегатную работу; длинные
  ряды позже переносятся в ClickHouse без изменения публичного REST.
