# 12. Local Development (DX)

Цель: поднять **Phase 0–1** локально за ~15 минут и понять, что делать при типичных сбоях.

Связанные документы: [00-overview.md](./00-overview.md) (фазы), [09-deployment.md](./09-deployment.md) (Compose), [11-observability-security.md](./11-observability-security.md) (HH User-Agent), [23-observability-tracing.md](./23-observability-tracing.md) (profile `observability`).  
Актуальный выбор провайдеров (Supabase, Yandex Valkey/Redis и т.д.): [21-external-services.md](./21-external-services.md).

---

## Быстрый старт (~15 мин)

### Предусловия

| Инструмент | Версия (ориентир) | Когда нужен |
|------------|-------------------|-------------|
| Docker Desktop + Compose v2 | текущий stable | для **local** Redis/PG; при cloud-everything можно без контейнеров |
| Облачный Postgres (рекомендуется) | free/cheap tier | OLTP без локального контейнера PG |
| Облачный Redis (рекомендуется) | free/cheap tier | cache/locks без локального контейнера Redis |
| Go | 1.22+ | когда появятся сервисы |
| Node.js | 20+ | для `web` (React) |
| Make | опционально | удобные targets; Windows: Git Bash + `make`, либо scoop/choco, либо команды ниже |
| golang-migrate CLI | опционально | или `make migrate-up` через Docker-образ |
| protoc / buf | опционально | генерация из `libs/proto` |
| curl / httpie, DBeaver | по желанию | smoke и GUI к БД |

Docker Desktop нужен, если поднимаете `local-redis` / `local-pg`. При полном cloud path (`DATABASE_URL` + `REDIS_URL`) Compose infra может не стартовать ничего.

### Рекомендуемый путь: облачный PostgreSQL + облачный Redis

Подробно: [Облачный PostgreSQL](#облачный-postgresql), [Облачный Redis](#облачный-redis). Кратко:

1. `cp .env.example .env` → `DATABASE_URL` (`sslmode=require`) + `REDIS_URL` (часто `rediss://`).
2. `make up-cloud` — контейнеры не нужны.
3. Позже: `make migrate-up` против облака; DBeaver → host/user из URL, SSL on.

### Гибрид: cloud PG + Redis в Docker

```bash
make up-local-redis
# docker compose --env-file .env -f deploy/compose/docker-compose.yml --profile local-redis up -d --wait
```

В `.env`: cloud `DATABASE_URL`, local `REDIS_URL=redis://localhost:6379/0`.

### Альтернатива: полный локальный PG + Redis

```bash
make up-local
# docker compose --env-file .env -f deploy/compose/docker-compose.yml --profile local-redis --profile local-pg up -d --wait
```

В `.env` оставить local `DATABASE_URL=...@localhost:5432/...?sslmode=disable` и local `REDIS_URL`.

### Шаги (сейчас: только infra)

1. Клонировать репозиторий и перейти в корень.
2. Скопировать env:

```bash
cp .env.example .env
# PowerShell: Copy-Item .env.example .env
```

3. Заполнить в `.env`:
   - **cloud-everything:** `DATABASE_URL` + `REDIS_URL` (секреты; не коммитить) + `ADMIN_TOKEN`;
   - **local PG / Redis:** local DSN/URL + `ADMIN_TOKEN`.  
   `HH_USER_AGENT` — обязателен **для реального ingest** (позже); для migrate/Redis-smoke можно оставить пустым.

4. Поднять infra (если нужна):

```bash
make up-cloud            # cloud PG + cloud Redis — ничего не стартует
# make up-local-redis    # только контейнер Redis
# make up-local          # Redis + Postgres в Docker
```

PowerShell (без Make):

```powershell
# cloud-everything — Compose не обязателен
# только local Redis
docker compose --env-file .env -f deploy/compose/docker-compose.yml --profile local-redis up -d --wait
# local Redis + local Postgres
docker compose --env-file .env -f deploy/compose/docker-compose.yml --profile local-redis --profile local-pg up -d --wait
```

5. Дождаться health (только если local Redis):

```bash
make wait-ready
# или: docker compose --env-file .env -f deploy/compose/docker-compose.yml --profile local-redis ps
```

6. (Когда появятся SQL-файлы в `migrations/postgres/`) применить схему:

```bash
make migrate-up
```

7. Подключиться к БД/Redis (облако или local — см. [ниже](#подключение-к-postgres--redis), [Облачный PostgreSQL](#облачный-postgresql), [Облачный Redis](#облачный-redis)).

**Сейчас в Compose:** Redis — опционально (`local-redis`); Postgres — опционально (`local-pg`). Profile `mvp` зарезервирован под приложения Phase 0–1.  
**Phase 1:** BFF + React SPA на хосте, ingest/normalize (in-process), PostgreSQL; Redis опционален.

Локальный API (один процесс):

```bash
make run-bff          # public :8080
make run-web          # Vite SPA :3000, /api proxy → BFF :8080

# UI / API
# http://localhost:3000
# http://localhost:8080/api/v1/health
# http://localhost:8080/api/v1/dashboard/summary?from=2026-07-01&to=2026-08-01
# http://localhost:8080/api/v1/vacancies?page=1&page_size=20
# http://localhost:8080/api/v1/roles?from=2026-07-01&to=2026-08-01
# http://localhost:8080/api/v1/regions?from=2026-07-01&to=2026-08-01
# http://localhost:8080/api/v1/skills/top?from=2026-07-01&to=2026-08-01

curl -X POST http://localhost:8080/api/v1/admin/ingest/runs \
  -H "Content-Type: application/json" \
  -H "X-Admin-Token: $ADMIN_TOKEN" \
  -d '{"source":"hh","mode":"incremental","params":{"area":"1","text":"golang"}}'
```

Для публичных read-маршрутов Phase 1 нужен `DATABASE_URL`; Redis остаётся
опциональным и не блокирует чтение из PostgreSQL. Параметры периода имеют
формат `YYYY-MM-DD`, `page` начинается с 1, `page_size` ограничен 100.
Полный контракт и список фильтров: [`api/openapi.yaml`](../../api/openapi.yaml).

PowerShell без Make (в двух терминалах):

```powershell
# терминал 1, корень репозитория
go run ./apps/bff/cmd/bff

# терминал 2
Set-Location apps/web
npm ci
npm run dev
```

По умолчанию `VITE_API_BASE_URL=/api/v1`, а Vite proxy отправляет `/api` на
`http://localhost:8080`; расширять CORS BFF для локальной разработки не нужно.

Отдельный gateway — Target Phase 3+ ([ADR 010](./adr/010-api-gateway.md)).

---

## Compose profiles (идея)

Файлы (целевая раскладка, см. [09-deployment.md](./09-deployment.md)):

```text
deploy/compose/
  docker-compose.yml
  docker-compose.override.yml
```

| Profile | Что поднимает | Когда |
|---------|---------------|-------|
| `mvp` | **цель Phase 0–1:** bff, query, ingest(+normalize in-process), web (пока пусто) | приложения; data plane — cloud или local-* |
| `local-redis` | контейнер `redis` (опционально) | если не используете облачный Redis |
| `local-pg` | контейнер `postgres` (опционально) | если не используете облачный PG |
| `olap` | + clickhouse | Phase 2, перед чтением трендов из CH |
| `bus` | + redpanda/kafka, normalizer как отдельный consumer | Phase 2 |
| `full` | mvp + local-redis + local-pg + olap + bus (+ scheduler, ai-analyzer stub) | локальная «почти prod» проверка |
| `observability` / `obs` | Loki + Tempo + Alloy + Prometheus + Grafana | opt-in; поиск логов по `trace_id` — [23](./23-observability-tracing.md) |

Примеры:

```bash
# cloud DATABASE_URL + REDIS_URL — Compose infra не нужна
# local Redis only (часто с cloud PG)
docker compose --env-file .env -f deploy/compose/docker-compose.yml --profile local-redis up -d --wait
# полный local data plane
docker compose --env-file .env -f deploy/compose/docker-compose.yml --profile local-redis --profile local-pg up -d --wait
# optional observability stack (Grafana http://localhost:3001):
# docker compose --env-file .env -f deploy/compose/docker-compose.yml --profile observability up -d
# make up-obs
# позже:
# docker compose --env-file .env -f deploy/compose/docker-compose.yml --profile mvp --profile olap up -d
# docker compose --env-file .env -f deploy/compose/docker-compose.yml --profile full up -d
```

**Redpanda** предпочтительнее «тяжёлого» Kafka для local (см. ADR в `docs/architecture/adr/`).

---

## Переменные окружения

Источник правды для имён: корневой [`.env.example`](../../.env.example).

| Переменная | Обязательна | Default (local) | Описание |
|------------|-------------|-----------------|----------|
| `HH_USER_AGENT` | **да** для ingest | — | Строка UA с контактом; fail-fast если пусто |
| `HH_BASE_URL` | нет | `https://api.hh.ru` | База HH API |
| `HH_APP_TOKEN` | нет | пусто | Application token, если используете |
| `ADMIN_TOKEN` | для admin | `dev-admin-token` | Заголовок `X-Admin-Token` |
| `DATABASE_URL` | **да** (предпочтительно) | local DSN / cloud DSN | OLTP; cloud: обычно `sslmode=require` |
| `POSTGRES_HOST` / `PORT` / `USER` / `PASSWORD` / `DB` | для local-pg / GUI | localhost / 5432 / lma / … / lma | дискретный fallback; пароль — секрет |
| `REDIS_URL` | **да** (предпочтительно) | `redis://localhost:6379/0` / cloud URL | Cache + locks; cloud TLS: `rediss://` |
| `REDIS_TLS_CA_FILE` | нет | путь к PEM CA | Yandex Valkey TLS; иначе обычно не нужен |
| `REDIS_HOST` / `REDIS_PORT` | нет | localhost / 6379 | Host-порт для Compose publish / GUI |
| `REDIS_ADDR` | fallback | `localhost:6379` | `host:port`, если клиент не читает URL |
| `REDIS_PASSWORD` | нет | пусто | Local обычно без пароля; cloud — в URL или отдельно |
| `BFF_HTTP_ADDR` | нет | `:8080` | Публичный BFF (MVP edge) |
| `VITE_API_BASE_URL` | нет | `/api/v1` | Публичный base path SPA; только несекретное значение |
| `VITE_VACANCIES_POLL_INTERVAL_MS` | нет | `30000` | Интервал проверки новых вакансий; минимум `10000`, `0` отключает polling (например, в тестах) |
| `QUERY_HTTP_ADDR` / `QUERY_GRPC_ADDR` | нет | `:8083` / `:9091` | Query (HTTP debug) |
| `INGEST_HTTP_ADDR` / `INGEST_GRPC_ADDR` | нет | `:8082` / `:9092` | Ingest admin |
| `KAFKA_BROKERS` | Phase 2 | `localhost:9092` | Redpanda/Kafka |
| `CLICKHOUSE_DSN` | Phase 2 | — | OLAP |
| `LOG_LEVEL` | нет | `info` | `debug` для локальной отладки |
| `OTEL_SERVICE_NAME` / `OTEL_EXPORTER_OTLP_ENDPOINT` | Phase 2–3 | — / `http://localhost:4318` | OTLP → Alloy; см. [23](./23-observability-tracing.md) |
| `GRAFANA_PORT` | нет | `3001` | host-порт Grafana (profile `observability`) |
| `CACHE_TTL_SUMMARY_SEC` | нет | `300` | TTL summary |
| `INGEST_LOCK_TTL_SEC` | нет | `2700` | TTL `lock:ingest:{source}` |
| `INGEST_MAX_PAGES` | нет | `5` | Guard страниц; `0` или `all` — все страницы текущего запроса в пределах лимитов HH |
| `INGEST_PER_PAGE` | нет | `100` | Размер страницы HH, `1..100`; `100` минимизирует число search-запросов |
| `INGEST_PAGE_DELAY_MS` | нет | `350` | Пауза между запросами HH; дополнительно действуют `Retry-After` и backoff |
| `INGEST_RUN_TIMEOUT_SEC` | нет | `1800` | Максимальная длительность bounded one-shot ingest run |
| `INGEST_SCOPE` | нет | `query` | `query` — целевой text/area; `it` — все официальные IT-роли HH |
| `INGEST_IT_AREA` | нет | `113` | Россия; менять только при явной смене продуктового гео |
| `INGEST_IT_MAX_PARTITIONS` | нет | `512` | Hard ceiling leaf-partitions плана |
| `INGEST_IT_MAX_REQUESTS` | нет | `500` | Hard budget probe + search/detail запросов одного запуска |
| `INGEST_SCHEDULER_INTERVAL` | нет | `30m` | Интервал bounded all-IT batch; минимум `10m` |
| `INGEST_SCHEDULER_RUN_ON_START` | нет | `true` | Первый batch сразу после старта dedicated scheduler |
| `INGEST_SCHEDULER_MAX_PARTITIONS_PER_BATCH` | нет | `8` | Дополнительный ceiling role/date partitions одного tick |
| `INGEST_SCHEDULER_BACKOFF_INITIAL` / `MAX` | нет | `1m` / `15m` | Exponential backoff после failed batch |
| `INGEST_SCHEDULER_JITTER_PERCENT` | нет | `20` | Симметричный jitter, `0..100` |
| `INGEST_SCHEDULER_SHUTDOWN_TIMEOUT` | нет | `30s` | Bounded wait после отмены текущего run |
| `INGEST_SCHEDULER_TEST_MODE` | нет | `false` | Разрешает интервал `<10m` только для явного local smoke/test |

Секреты только в `.env` (gitignored), не в Compose YAML и не в документации как реальные значения.

Polling экрана `/vacancies` каждые 30 секунд запрашивает через BFF только первую страницу
новейших вакансий с текущими фильтрами. Он показывает строки после записи
очередным ingest-run в PostgreSQL, но сам не запускает HH ingest. Свежесть HH
задаёт `INGEST_SCHEDULER_INTERVAL` (default 30 минут), а не frontend polling.
При скрытой вкладке или offline браузер
приостанавливает polling и повторяет проверку после focus/reconnect.

`INGEST_MAX_PAGES=0` / `all` означает не «все вакансии HH», а все страницы,
которые официальный API сообщает для комбинации `INGEST_DEFAULT_AREA` +
`INGEST_DEFAULT_TEXT`. Глубина одной поисковой выдачи HH ограничена 2000
результатами: при `INGEST_PER_PAGE=100` доступно не более 20 страниц. В коде
также остаётся hard ceiling 100 страниц.

Для полного текущего IT-среза используется отдельный `INGEST_SCOPE=it`: официальный
каталог `/professional_roles`, категория `11`, Россия (`area=113`), затем
partition по одной роли и времени. Обычный старт остаётся безопасным `query` и
никогда неожиданно не запускает полный crawl.

---

## Make targets

Актуальные команды корневого `Makefile` и запланированные расширения:

| Target | Действие | Статус |
|--------|----------|--------|
| `make up-cloud` | cloud PG + cloud Redis: ничего не стартует, напоминание про `.env` | **есть** |
| `make up-local-redis` / `up-redis` | Compose `--profile local-redis` | **есть** |
| `make up-mvp` | alias → `up-local-redis` (пока нет app-сервисов в `mvp`) | **есть** |
| `make up-local-pg` | Compose `--profile local-pg` (Postgres) | **есть** |
| `make up-local` | local-redis + local-pg | **есть** |
| `make down` | Compose down (local-redis + local-pg) | **есть** |
| `make logs` / `make ps` | логи / статус | **есть** |
| `make wait-ready` | health local Redis; позже + BFF `/api/v1/health` | **есть** (infra) |
| `make run-bff` | публичный BFF на `:8080` | **есть** |
| `make run-web` | Vite SPA на `:3000`, proxy `/api` → BFF | **есть** |
| `make psql` / `make redis-cli` | shell в контейнеры (`local-pg` / `local-redis`) | **есть** |
| `make migrate-up` / `migrate-down` | golang-migrate по `DATABASE_URL` (Docker image) | **есть** |
| `make bust-cache` | `INCR meta:cache_version` (local Redis) | **есть** |
| `make up-full` | `--profile full` | позже (Phase 2+) |
| `make up-obs` | Compose `--profile observability` (Loki/Tempo/Grafana) | **есть** (stub) |
| `make ingest-hh` / `ingest-hh-fixture` | one-shot HH → normalize → PG (`apps/ingest`); fixture без live HH | Phase 1 |
| `make ingest-hh-it-plan` | live aggregate-only planning, без vacancy content/записи | Phase 1 |
| `make ingest-hh-it` | bounded/resumable IT crawl; при budget продолжить следующим запуском | Phase 1 |
| `make run-ingest-scheduler` | dedicated scheduler: bounded/resumable all-IT batch по расписанию | Phase 1 |
| `make test` | `go test ./...` + Vitest web | **есть** |
| `make proto` / `openapi-lint` / `smoke` / `fmt` / `lint` | по мере появления tooling | planned |

**Windows без Make** (PowerShell, из корня репо):

```powershell
Copy-Item .env.example .env   # один раз
# cloud-everything — Compose не обязателен
# local Redis
docker compose --env-file .env -f deploy/compose/docker-compose.yml --profile local-redis up -d --wait
# local Redis + local Postgres
docker compose --env-file .env -f deploy/compose/docker-compose.yml --profile local-redis --profile local-pg up -d --wait
docker compose --env-file .env -f deploy/compose/docker-compose.yml --profile local-redis --profile local-pg ps
docker compose --env-file .env -f deploy/compose/docker-compose.yml --profile local-redis logs -f
docker compose --env-file .env -f deploy/compose/docker-compose.yml --profile local-redis --profile local-pg down
# aggregate-only план, затем явный bounded crawl:
go run ./apps/ingest/cmd/ingest -scope it -dry-run
go run ./apps/ingest/cmd/ingest -scope it
# dedicated scheduler; сам загружает корневой .env через godotenv:
$env:INGEST_SCOPE = "it"
go run ./apps/ingest/cmd/scheduler
```

Scheduler использует process-local no-overlap и session-level PostgreSQL
advisory lock `549004801` на отдельном соединении. Второй процесс не ждёт lock,
а пропускает tick. Checkpoint содержит immutable план текущего cycle и номер
следующей role/date partition; failed/canceled batch его не стирает. После
полного cycle следующий tick создаёт новый план. Reconciliation `is_active`
после полного cycle пока отложен: partial batch никогда не деактивирует
невстреченные вакансии.

---

## Облачный PostgreSQL

Рекомендуемый cloud path: **managed Postgres** + **managed Redis** (см. [Облачный Redis](#облачный-redis)). Контейнер `postgres` не обязателен. Гибрид: cloud PG + `make up-local-redis` тоже ок.

### Провайдеры (пет-проект)

Актуальный выбор: **[21-external-services.md](./21-external-services.md)** (сейчас: **Supabase**; Neon — отклонён).  
Тарифы и free tier меняются — **проверь актуальные условия** на сайте провайдера перед регистрацией.

| Провайдер | Зачем смотреть | Заметки |
|-----------|----------------|---------|
| [Supabase](https://supabase.com) | **выбрано** — Postgres + кабинет | бери **URI** из Database settings; SSL обязателен |
| Yandex Cloud Managed PostgreSQL / [Timeweb Cloud](https://timeweb.cloud) | запасной вариант ближе к RU-инфре | trial или дешёвый VPS+PG — смотри текущие акции; не выдумываем цены |

Регистрацию и оплату делаешь **сам** в кабинете провайдера — репозиторий не создаёт облачные аккаунты.

### Шаги

1. Создай проект/БД у провайдера (регион ближе к тебе).
2. Скопируй connection string (часто «Connection string» / «URI»).
3. В корне репо:

```bash
cp .env.example .env
# PowerShell: Copy-Item .env.example .env
```

4. В **своём** `.env` (не коммитить) выставь, например:

```text
DATABASE_URL=postgres://USER:PASSWORD@HOST:5432/DBNAME?sslmode=require
```

Спецсимволы в пароле URL-encode. Подставь реальные `POSTGRES_HOST` / `USER` / `DB` / `PASSWORD` из того же кабинета, если удобно для DBeaver.

5. Redis: либо cloud `REDIS_URL` ([ниже](#облачный-redis)) и `make up-cloud`, либо локальный контейнер:

```bash
make up-local-redis
# PowerShell:
# docker compose --env-file .env -f deploy/compose/docker-compose.yml --profile local-redis up -d --wait
```

6. Когда появятся миграции: `make migrate-up` — ходит в `DATABASE_URL` (облако; образ migrate с хоста, outbound internet).

7. **DBeaver:** New connection → PostgreSQL → Host/Port/DB/User/Password из кабинета → SSL: **require** (или режим провайдера) → Test connection.

Не вставляй боевой URL в issues, скриншоты README или git. См. [17-secrets-management.md](./17-secrets-management.md).

---

## Облачный Redis

**Preferred для РФ:** [Yandex Managed Service for Valkey™](https://yandex.cloud/ru/docs/managed-valkey/) (ранее Managed Redis; протокол Redis-compatible).  
Статус в реестре — **кандидат** (не «выбрано»): [21-external-services.md](./21-external-services.md).  
Docker profile `local-redis` — только optional fallback, не основной DX при cloud path.

Клиент везде один: `REDIS_URL` (`rediss://` при TLS). Тарифы меняются — **смотри актуальные цены в кабинете**, free tier здесь не обещаем.

| Провайдер | Роль | Заметки |
|-----------|------|---------|
| [Yandex Managed Valkey](https://yandex.cloud/ru/docs/managed-valkey/) | **preferred (РФ)** | Карты/доступ из РФ; TLS порт часто `6380`; нужен CA + публичный доступ к хостам для laptop |
| [Selectel](https://docs.selectel.ru/managed-databases/redis/) / [Timeweb Cloud](https://timeweb.cloud/) / [VK Cloud](https://cloud.vk.com/) | альтернативы РФ | Тот же `REDIS_URL`; публичный доступ/ACL — по доке провайдера |
| [Redis Cloud](https://redis.io/cloud/) | опционально | Если кабинет/endpoint доступны из твоей сети |
| [Upstash](https://upstash.com) | не primary для РФ | **Может быть недоступен из РФ** — не рекомендовать как основной путь |

Регистрацию делаешь **сам** — репозиторий не создаёт облачные аккаунты.

### Шаги (Yandex Managed Valkey / Redis)

1. Зарегистрируйся в [Yandex Cloud](https://console.yandex.cloud/) (нужен аккаунт + платёжный аккаунт / способ оплаты — по правилам Yandex Cloud).
2. Создай каталог (folder) и сеть VPC (или сеть по умолчанию).
3. Создай кластер **Managed Service for Valkey™**:
   - нешардированный кластер для учебного DX;
   - **поддержка TLS** — включить;
   - **использовать FQDN вместо IP** — включить (иначе TLS/hostname часто ломаются);
   - задай пароль пользователя;
   - для работы **с ноутбука** — включи **публичный доступ** у хоста(ов); иначе кластер доступен только из VPC (VM в той же сети).
4. Группа безопасности: разреши входящий TCP на порт TLS (обычно **6380**) с твоего IP (или временно шире — на свой риск).
5. В карточке кластера возьми **FQDN** мастера (часто вида `c-<cluster_id>.rw.mdb.yandexcloud.net`), порт TLS и пароль.
6. Скачай CA Yandex (нужен для TLS-клиента):

```bash
# Linux/macOS:
mkdir -p ~/.redis
curl -fsSL -o ~/.redis/YandexInternalRootCA.crt \
  https://storage.yandexcloud.net/cloud-certs/CA.pem

# Windows PowerShell:
# New-Item -ItemType Directory -Force $HOME\.redis | Out-Null
# Invoke-WebRequest -Uri https://storage.yandexcloud.net/cloud-certs/CA.pem `
#   -OutFile $HOME\.redis\YandexInternalRootCA.crt
```

7. В **своём** `.env` (не коммитить):

```text
# Yandex Managed Valkey (TLS, порт часто 6380):
REDIS_URL=rediss://:PASSWORD@c-CLUSTER_ID.rw.mdb.yandexcloud.net:6380/0
REDIS_TLS_CA_FILE=C:\Users\YOU\.redis\YandexInternalRootCA.crt
# или: REDIS_TLS_CA_FILE=/home/YOU/.redis/YandexInternalRootCA.crt

# Local Docker fallback (не основной путь):
# REDIS_URL=redis://localhost:6379/0
```

`rediss://` — Redis/Valkey over TLS (две буквы **s**). Спецсимволы в пароле — URL-encode.  
Пустой user в URL (`rediss://:PASSWORD@host…`) — нормальная форма, если провайдер не требует username.

8. При cloud PG + cloud Redis контейнеры не нужны:

```bash
make up-cloud
make run-bff
curl -s http://localhost:8080/api/v1/health
# ожидай checks.redis: "up" (и checks.database при DATABASE_URL)
```

9. Опционально заполни `REDIS_HOST` / `REDIS_PORT` / `REDIS_PASSWORD` / `REDIS_ADDR` для GUI, которые не читают URL.

10. Проверка с хоста (если есть `redis-cli` с TLS):

```bash
# redis-cli -h HOST -p 6380 --tls --cacert "$REDIS_TLS_CA_FILE" -a 'PASSWORD' PING
```

**Phase 0:** BFF не падает без Redis — без `REDIS_URL` check просто отсутствует; при заданном URL и недоступном Redis — `status: degraded`, `checks.redis: down`. Cache-aside — Phase 1.

**Tradeoffs:** платный managed (смотри прайс); latency до облака; TLS + CA; публичный доступ = поверхность атаки (ограничь SG/ACL своим IP). VPC-only без публичного доступа с ноутбука неудобен.

Не коммить `REDIS_URL` с паролем. См. [17-secrets-management.md](./17-secrets-management.md).

---

## Подключение к Postgres / Redis

Значения — из вашего `.env` (шаблон: [`.env.example`](../../.env.example)).

| Параметр | Postgres (local-pg) | Postgres (cloud) | Redis (local-redis) | Redis (cloud) |
|----------|---------------------|------------------|---------------------|--------------|
| Host | `localhost` | из `DATABASE_URL` / кабинета | `localhost` | из `REDIS_URL` / кабинета |
| Port | `5432` | часто `5432` / `6432` | `6379` | из кабинета |
| User / DB | `lma` / `lma` | из кабинета | — | часто `default` / db `0` |
| Password | `POSTGRES_PASSWORD` | секрет из кабинета | обычно пусто | секрет из кабинета |
| TLS / SSL | off (`sslmode=disable`) | **обычно require** | нет | **обычно `rediss://`** |
| DSN / URL | local `DATABASE_URL` | cloud `DATABASE_URL` | `REDIS_URL=redis://localhost:6379/0` | cloud `REDIS_URL` |

**psql** (только если поднят `local-pg`):

```bash
make psql
# или
docker compose --env-file .env -f deploy/compose/docker-compose.yml --profile local-pg exec postgres psql -U lma -d lma

# с хоста, если установлен psql (local):
psql "postgres://lma:lma_local_password@localhost:5432/lma?sslmode=disable"
# cloud — подставь свой DATABASE_URL:
# psql "$DATABASE_URL"
```

**DBeaver / любой GUI:**

- Local PG: Host `localhost`, Port `5432`, Database/User `lma`, Password из `.env`, SSL off.
- Cloud PG: параметры из кабинета, SSL on / require.

**redis-cli** (local контейнер):

```bash
make redis-cli
# или
docker compose --env-file .env -f deploy/compose/docker-compose.yml --profile local-redis exec redis redis-cli
redis-cli -h localhost -p 6379 PING
```

Сервисы Go (когда появятся) на хосте читают `DATABASE_URL` (или `POSTGRES_*`) и `REDIS_URL` (или `REDIS_ADDR` + `REDIS_PASSWORD`).  
Внутри Docker-сети Compose: при `local-redis` — `redis:6379`; при `local-pg` — `postgres:5432` (сеть `lma_net`).

---

## Порты (local)

| Сервис | Порт | Конфликт частый с |
|--------|------|-------------------|
| web | 3000 | другие SPA / Create React App |
| bff | 8080 | Tomcat, другие API |
| ingest HTTP | 8082 | — |
| query HTTP | 8083 | — |
| query gRPC | 9091 | — |
| ingest gRPC | 9092 | **Kafka/Redpanda** часто тоже 9092 |
| postgres | 5432 | локальный PostgreSQL |
| redis | 6379 | локальный Redis |
| clickhouse | 8123 / 9000 | — |
| kafka/redpanda | 19092 (рекомендация local) или 9092 | конфликт с ingest gRPC |

**Рекомендация:** на local маппить Redpanda как `19092:9092`, а ingest gRPC оставить `9092` внутри сети Compose (с хоста — `localhost:19092` для kafka CLI).

---

## Troubleshooting

### HH: 403 / отказ из‑за User-Agent

**Симптом:** ingest падает сразу, в логах `forbidden` / требование User-Agent.

**Что сделать:**

1. Задать в `.env` непустой `HH_USER_AGENT`, например:

```text
HH_USER_AGENT=LMAStudyProject/0.1 (+https://github.com/<user>/study_project; you@example.com)
```

2. Пересоздать контейнер ingest (`docker compose up -d --force-recreate ingest`).
3. Убедиться, что клиент реально шлёт заголовок `User-Agent` (не подменяет его библиотека).
4. Не использовать пустую строку и не копировать чужой UA без контакта.

См. также [11-observability-security.md](./11-observability-security.md).

### HH: 429 Too Many Requests

- Снизить параллелизм (1 worker), увеличить delay между страницами.
- Смотреть `Retry-After`; не ретраить tight-loop.
- Не запускать второй run — должен сработать lock (`409 CONFLICT`).
- Runbook: [cache-and-locks.md](../runbooks/cache-and-locks.md).

### Порт уже занят

```bash
# Windows (PowerShell): кто слушает 8080
netstat -ano | findstr :8080
```

Варианты: остановить конфликтующий процесс или сменить host-порт в Compose (`8080:8080` → `18080:8080`) и обновить `.env` / документацию локально.

### Postgres «connection refused»

- **Cloud:** проверь `DATABASE_URL`, SSL (`sslmode=require`), IP allowlist/firewall в кабинете, что пароль URL-encoded
- **Local-pg:** `make up-local-pg` / `make up-local`, дождаться healthy: `docker compose … ps`
- Проверить `DATABASE_URL` / `POSTGRES_*` vs то, что реально слушает БД
- Volume с другой парольной политикой: `make down` + удалить volume **только** если можно потерять local data

### Redis «connection refused» / TLS errors

- **Cloud:** проверь `REDIS_URL` (`rediss://` при TLS), пароль URL-encoded, SG/ACL / публичный доступ; для Yandex — `REDIS_TLS_CA_FILE`; VPC-only без публичного доступа с ноутбука не доступны
- **Local-redis:** `make up-local-redis`, дождаться healthy: `docker compose … --profile local-redis ps`
- Не путать `redis://` (без TLS) и `rediss://` (с TLS)

### Redis / cache «странные» данные после смены схемы

```bash
make bust-cache   # нужен local-redis
# cloud: redis-cli -u "$REDIS_URL" INCR meta:cache_version
```

### Ingest `409 Conflict` / застрявший lock

См. [cache-and-locks.md](../runbooks/cache-and-locks.md): проверить TTL, heartbeat, ручной DEL только если run точно мёртв.

### Ingest `failed` / partial

См. [ingest-failed.md](../runbooks/ingest-failed.md).

### DLQ растёт

См. [dlq-replay.md](../runbooks/dlq-replay.md) (актуально с Phase 2).

---

## Что не поднимать в day-1

| Компонент | Когда |
|-----------|-------|
| Kafka / Redpanda | Phase 2 (`profile bus`) |
| ClickHouse | Phase 2 (`profile olap`) — тренды сначала из PG |
| ai-analyzer | Phase 4 |
| Полный k8s | Phase 3 |

Публичный REST (`/api/v1/...`) при этом **не меняется** между фазами — меняется только источник данных за Query.
