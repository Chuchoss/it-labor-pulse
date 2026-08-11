# Runbook: Cache, locks, 429

Операции с Redis: cache bust, stuck ingest lock, реакция на HH 429. Ключи: [06-caching.md](../architecture/06-caching.md).

---

## 1. HH 429 / rate limit

### Симптомы

- Логи ingest: HTTP 429, `hh_429_total` ↑  
- Run `partial`/`failed`, долгий backoff  

### Действия

1. **Не** стартовать параллельный full ingest.
2. Убедиться, что lock работает (второй `POST .../ingest/runs` → 409).
3. Снизить нагрузку: `INGEST_PAGE_DELAY_MS` ↑, `INGEST_MAX_PAGES` ↓, parallelism = 1.
4. Уважать `Retry-After` (клиент HH обязан).
5. Повторить позже **incremental** с узким окном дат.
6. Если 429 на словарях areas/roles — использовать Redis `dict:hh:*` (TTL 24h), не долбить HH.

---

## 2. Stuck lock (`lock:ingest:{source}`)

### Симптомы

- Все ручные/scheduled запуски → **409 CONFLICT**
- В Redis есть ключ, а активного процесса ingest нет
- Run в PG висит `running` дольше обычного

### Диагностика

```bash
redis-cli GET lock:ingest:hh
redis-cli TTL lock:ingest:hh
# сверить value с run_id в PG ingest_runs
```

| Наблюдение | Вывод |
|------------|-------|
| TTL > 0, pod ingest жив | Run ещё идёт — ждать |
| TTL > 0, pod мёртв | Stuck lock |
| ключа нет, но API 409 | баг другого слоя / старый ответ кэша клиента |
| TTL −1 (no expiry) | Ошибка клиента lock — выставить EX |

### Снятие lock (только если run точно мёртв)

Сравнить-and-delete (не слепой DEL чужого run):

```bash
# псевдо: DEL only if value == <run_id>
redis-cli EVAL "if redis.call('get',KEYS[1])==ARGV[1] then return redis.call('del',KEYS[1]) else return 0 end" 1 lock:ingest:hh <run_id>
```

Затем:

1. Пометить run в PG как `failed` с `error_message=lock_expired_or_manual_clear` (если ещё `running`).
2. Запустить новый incremental.
3. Разобрать почему не было heartbeat / TTL слишком короткий.

**Не** снимать lock при живом ingest — получите двойной crawl и 429.

---

## 3. Cache bust

### Когда

- После крупного ingest / normalize, UI показывает старые агрегаты дольше TTL  
- Сломали формат DTO и нужно мгновенно сбросить  
- Ops «данные на дашборде не те»

### Как

```bash
redis-cli INCR meta:cache_version
# или: make bust-cache
```

Ключи `cache:v{n}:...` со старым `n` перестанут читаться; истекут по LRU/TTL.

### Не делать

- `FLUSHALL` / `FLUSHDB` в shared Redis  
- `KEYS cache:*` + DEL в prod  
- Бюстить dict keys без нужды (`dict:hh:areas` — 24h ок)

### Проверка

```bash
curl -s "http://localhost:8080/api/v1/dashboard/summary?from=2026-07-01&to=2026-08-01" | jq .cache
# ожидаем MISS после bust, затем HIT
```

---

## 4. Stale / wrong cache после деплоя

1. `INCR meta:cache_version`
2. Проверить, что Query читает тот же Redis DB/prefix
3. Если в ответе нет поля `cache` — смотреть mapper BFF

---

## Связанные

- [ingest-failed.md](./ingest-failed.md)  
- [18-logging-and-incidents.md](../architecture/18-logging-and-incidents.md) — playbook + поиск 429/lock в логах  
- [12-local-dev.md](../architecture/12-local-dev.md)  
- ADR [006-cache-strategy.md](../architecture/adr/006-cache-strategy.md)  
