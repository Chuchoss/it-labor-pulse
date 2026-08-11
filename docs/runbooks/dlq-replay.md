# Runbook: DLQ replay (`vacancies.raw.dlq`)

Актуально с **Phase 2** (Kafka/Redpanda). Poison messages после N retry попадают в `vacancies.raw.dlq` ([07-messaging.md](../architecture/07-messaging.md)).

Перед использованием этого runbook должен быть принят отдельный ADR о transactional outbox либо documented replay checkpoint: он определяет, когда допустимо продвинуть ingest cursor и как восстановить страницу при сбое publish.

## Симптомы

- Рост lag/сообщений в `vacancies.raw.dlq`
- Метрика `normalize_errors_total` + DLQ produce
- Алерт «Kafka lag» на DLQ consumer group (если есть) или размер топика

## Когда replay НЕ нужен

| Ситуация | Действие |
|----------|----------|
| Баг нормалайзера уже в проде, payload валидный | Сначала задеплоить fix, потом replay |
| Payload реально битый / не HH JSON | Архивировать/skip; чинить источник |
| Дубликаты уже успешно в PG | Replay безопасен (idempotent), но бесполезен — можно skip |

## Диагностика одного сообщения

1. Прочитать из DLQ (пример Redpanda):

```bash
rpk topic consume vacancies.raw.dlq --num 1 --offset start
```

2. Проверить envelope: `schema_version`, `source`, `external_id`, `ingest_run_id`, `content_hash`.
3. Прогнать payload локально через normalize unit/fixture test.
4. Зафиксировать `reason` из headers/логов (validation, unknown schema, PG hard error).

## Процедура replay

### Безопасный путь (рекомендуется)

1. **Stop** автоматический DLQ-ignore (если есть) — не терять сообщения.
2. Задеплоить фикс normalizer (если нужен).
3. Переложить сообщения DLQ → `vacancies.raw.hh` (или исходный `vacancies.raw.{source}`) **тем же key** `{source}:{external_id}`:

```bash
# схема: выгрузить → отфильтровать → произвести в raw topic
# точная утилита появится как apps/tools/dlq-replay; пока вручную/rpk
```

4. Следить: DLQ не растёт, `normalize_upserts_total` растёт, errors стабильны.
5. Успешно обработанные offsets закоммитить; неудачные оставить/вернуть в DLQ.

### Ограничения

- Replay **батчами** (например 100–500), не flood HH (replay из DLQ **не** должен снова дергать HH — raw уже в сообщении).
- Сохранять `message_id` или писать новый + header `replayed_from`.
- At-least-once: upserts идемпотентны; CH snapshots тоже at-least-once ok.

## Откат

Если replay усугубил ошибки — остановить producer replay, вернуть фикс, не чистить PG.

## Post-check

- [ ] DLQ depth ↓  
- [ ] Нет spike 5xx Query  
- [ ] Выборочно vacancy by `external_id` в PG  
- [ ] При необходимости `INCR meta:cache_version`  

## Связанные

- [ingest-failed.md](./ingest-failed.md)  
- [18-logging-and-incidents.md](../architecture/18-logging-and-incidents.md) — correlate `ingest_run_id` / DLQ в логах  
- [13-testing.md](../architecture/13-testing.md)  
- ADR [001-json-vs-avro.md](../architecture/adr/001-json-vs-avro.md)  
