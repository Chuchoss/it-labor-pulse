# ADR 006: Cache-aside Redis + version bump

## Context

Нужно кэшировать dashboard/trends без сложной invalidation по ключам и без жёсткой зависимости от Redis availability.

## Decision

**Cache-aside** в Query: GET Redis → miss → PG/CH → SETEX.  
Инвалидация: **`INCR meta:cache_version`** (ключи вида `cache:v{n}:...`).  
Redis down → **fail-open** в PG/CH. Детали ключей/TTL: [06-caching.md](../06-caching.md).

## Consequences

- (+) Простая модель для соло-проекта  
- (+) Bust одной командой после ingest  
- (−) Краткий stale window до TTL / до bump  
- Не использовать `KEYS`/`FLUSHALL` как штатную инвалидацию  
