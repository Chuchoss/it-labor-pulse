# IT Labor Pulse

Аналитика IT-рынка труда: сбор вакансий (сначала HeadHunter), нормализация, зарплаты и спрос по ролям/регионам, тренды.

## Документация

| | |
|--|--|
| **API (Redoc)** | [chuchoss.github.io/it-labor-pulse](https://chuchoss.github.io/it-labor-pulse/) |
| **API (Swagger)** | [Swagger UI](https://chuchoss.github.io/it-labor-pulse/swagger.html) |
| **Архитектура** | [docs/architecture/](./docs/architecture/) · [индекс](./docs/architecture/README.md) |
| **Контракт** | [`api/openapi.yaml`](./api/openapi.yaml) · [`libs/proto/lma/`](./libs/proto/lma/) |

## Стек

React · Go · PostgreSQL · Redis · Kafka · ClickHouse · gRPC · Kubernetes

## Quickstart

```bash
git clone https://github.com/Chuchoss/it-labor-pulse.git
cd it-labor-pulse
cp .env.example .env   # PowerShell: Copy-Item .env.example .env
```

В `.env` задайте `DATABASE_URL` (рекомендуется **Supabase**) и при необходимости `REDIS_URL`. Секреты не коммитить.

Дальше — только [локальный DX](./docs/architecture/12-local-dev.md) (Compose-профили, migrate, cloud vs local).

## Статус

| Сейчас | Дальше |
|--------|--------|
| Архитектура, OpenAPI/proto, HH-фикстуры, cloud/local infra | Код сервисов по [фазам](./docs/architecture/00-overview.md) |

## Attribution

Данные вакансий принадлежат площадкам (HH и др.). Соблюдайте ToS, лимиты и User-Agent. Зарплаты на дашборде — оценка по полям salary в вакансиях, не опросы.
