# 21. Внешние сервисы и провайдеры

Живой реестр внешних сервисов / managed-провайдеров для LMA (IT Labor Pulse).  
Внутренние микросервисы проекта (BFF, Query, Ingest) — в [02-services.md](./02-services.md).

**Не хранить здесь:** пароли, токены, реальные connection strings, kubeconfig. Только имена env-переменных и публичные ссылки. Секреты — [17-secrets-management.md](./17-secrets-management.md), шаблон имён — [`.env.example`](../../.env.example).

Связанные документы: [12-local-dev.md](./12-local-dev.md) (как поднять), [08-integrations-and-extensibility.md](./08-integrations-and-extensibility.md) (адаптеры источников), [01-system-context.md](./01-system-context.md) (C4 внешние системы).

---

## Статусы

| Статус | Значение |
|--------|----------|
| **выбрано** | Текущий выбор для проекта; конфиг ориентируем на него |
| **кандидат** | Рассматриваем; провайдер ещё не зафиксирован |
| **later** | Нужно по фазе/roadmap, выбор отложен |
| **отклонён** | Явно не используем (коротко в заметках) |
| **local fallback** | Опциональный Docker-профиль, не cloud-провайдер |

---

## Реестр

| Сервис (роль) | Провайдер | Статус | Зачем | Где конфиг (.env) | Docs / links | Заметки |
|---------------|-----------|--------|-------|-------------------|--------------|---------|
| PostgreSQL (OLTP) | [Supabase](https://supabase.com) | **выбрано** | Managed Postgres для Phase 0–1 без обязательного `local-pg` | `DATABASE_URL` (+ опц. `POSTGRES_*` для DBeaver) | [Supabase Docs](https://supabase.com/docs), [12 § Облачный PostgreSQL](./12-local-dev.md#облачный-postgresql) | URI из Database settings; обычно `sslmode=require`. Не коммитить URL |
| PostgreSQL (local) | Docker Compose profile `local-pg` | local fallback | Offline / без cloud | `DATABASE_URL` → `localhost:5432`, `POSTGRES_*` | [12-local-dev.md](./12-local-dev.md) | `make up-local` / `make up-local-pg` |
| PostgreSQL (альтернатива) | [Neon](https://neon.tech) | **отклонён** | — | — | — | Рассматривали; **не используем** (выбран Supabase) |
| Redis (cache / locks) | TBD | **кандидат** | Cache-aside, ingest locks | `REDIS_URL` (+ опц. `REDIS_*`) | [06-caching.md](./06-caching.md), [12 § Облачный Redis](./12-local-dev.md#облачный-redis) | Cloud planned; провайдер **ещё не выбран**. Кандидаты: [Upstash](https://upstash.com), [Redis Cloud](https://redis.io/cloud/), Yandex Managed Redis. Часто TLS → `rediss://` |
| Redis (local) | Docker Compose profile `local-redis` | local fallback | Dev без cloud Redis | `REDIS_URL=redis://localhost:6379/0` | [12-local-dev.md](./12-local-dev.md) | `make up-local-redis` |
| HeadHunter API | hh.ru | **later** (Phase 1) | Источник вакансий №1 | `HH_USER_AGENT`, `HH_BASE_URL`, опц. `HH_APP_TOKEN` | [api.hh.ru](https://api.hh.ru/openapi/redoc), [08-integrations…](./08-integrations-and-extensibility.md) | Обязательный идентифицирующий User-Agent; backoff на 429; токены только server-side. Пока не «зарегистрирован» как активный прод-коннект |
| SuperJob API | superjob.ru | **later** | Доп. источник вакансий | TBD (после адаптера) | ToS / API docs провайдера | Новый source = новый adapter; без scraping |
| Remotive API | remotive.com | **later** | Remote jobs | TBD | ToS / API docs провайдера | Target; официальный API only |
| Kafka (messaging) | TBD (local: Redpanda) | **later** (Phase 2) | Async pipeline raw→normalized | `KAFKA_BROKERS`, consumer groups | [07-messaging.md](./07-messaging.md), ADR 003 | Провайдер managed Kafka — TBD; local — Redpanda optional |
| ClickHouse (OLAP) | TBD | **later** (Phase 2) | Trends / тяжёлая аналитика | `CLICKHOUSE_*` | [05-data-model.md](./05-data-model.md), ADR 005 | Провайдер TBD |
| AI provider | TBD | **later** (Phase 4) | AI-анализ / inference | TBD | [08-integrations…](./08-integrations-and-extensibility.md) | Не раньше Phase 4 |
| Edu platform (курсы) | Stepik | **кандидат** (Phase 5) | Learning-interest signals для «Тенденции» | TBD | ToS / API docs провайдера | API availability **не проверена**; аккаунта/ключа нет. Только официальный API |
| Edu platform (курсы) | Coursera | **кандидат** (Phase 5) | Каталог / proxies интереса к обучению | TBD | Partner/API docs | Доступ к API часто партнёрский — статус TBD; без scraping |
| Edu platform (курсы) | Skillbox | **кандидат** (Phase 5) | RU learning signals | TBD | ToS / публичные данные | API **unknown**; не утверждать наличие контракта |
| News / RSS | TBD (официальные RSS / news API) | **кандидат** (Phase 5) | Media-attention mentions технологий/ролей | TBD | [08 § Perspectives](./08-integrations-and-extensibility.md) | Предпочитать RSS/официальные API; Google News и аналоги — только после проверки ToS |
| Articles | Habr | **кандидат** (Phase 5) | Упоминания в статьях / тегах | TBD | ToS / API если есть | Без обхода ToS; attribution обязательна |
| Observability (логи) | Loki / Grafana (ориентир) | **later** | Централизованные логи, дашборды | TBD | [18-logging-and-incidents.md](./18-logging-and-incidents.md), [11-observability-security.md](./11-observability-security.md) | Стек наблюдаемости не зафиксирован как cloud-провайдер |
| CI/CD | GitHub Actions | **выбрано** | lint/test gate → deploy | — (секреты в GitHub Environments, не в `.env`) | [10-cicd.md](./10-cicd.md), [17 §6](./17-secrets-management.md#6-cicd-github-actions), [`.github/workflows/ci-cd.yml`](../../.github/workflows/ci-cd.yml) | Environments: `development` (push `develop`), `production` (push `main` + reviewers). Deploy-секреты только в Environments; fork PR без secrets |
| API docs hosting | GitHub Pages | **выбрано** | Публичный Redoc + Swagger UI для `api/openapi.yaml` | — (секретов нет) | [10-cicd.md](./10-cicd.md) (`docs-pages.yml`), [22-documentation-style.md](./22-documentation-style.md), [ADR 008](./adr/008-github-pages-openapi.md), [`docs/api-site/`](../api-site/) | Source = GitHub Actions. URL: `https://<user>.github.io/<repo>/`. jsDelivr CDN для UI; yaml копируется в site только на build |

---

## Как обновлять

1. **Сначала этот файл** — при выборе или смене провайдера обнови строку (статус, провайдер, ссылки, заметки).
2. При необходимости синхронизируй краткие упоминания в [12-local-dev.md](./12-local-dev.md) и корневом `README.md` (без дублирования длинных таблиц).
3. Спорный или дорогостоящий выбор → короткий ADR в [`adr/`](./adr/) + ссылка из строки «Заметки».
4. Новые env-имена — только через [`.env.example`](../../.env.example) и [17-secrets-management.md](./17-secrets-management.md); сюда — имена переменных, не значения.
5. Не помечай Redis (или другой сервис) как **выбрано**, пока провайдер реально не зафиксирован командой/владельцем проекта.
