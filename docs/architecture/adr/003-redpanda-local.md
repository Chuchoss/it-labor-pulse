# ADR 003: Redpanda для local Kafka-совместимого брокера

## Context

Phase 2 нужен брокер. Полный Apache Kafka в Docker тяжёлый для ноутбука. Нужна совместимость с Kafka protocol для Go клиентов.

## Decision

**Local (Compose profile `bus`): Redpanda** как optional/default брокер.  
В stage/prod допустим managed Kafka или тот же Redpanda — клиенты используют стандартный Kafka API (`KAFKA_BROKERS`).

Host port local: **19092** (избежать конфликта с ingest gRPC `:9092`).

## Consequences

- (+) Быстрый старт, меньше RAM  
- (+) Те же topics/consumer groups, что в доках  
- (−) Мелкие отличия admin API — для ops использовать `rpk`  
- Документировать оба имени сервиса в Compose (`redpanda` alias `kafka`)  
