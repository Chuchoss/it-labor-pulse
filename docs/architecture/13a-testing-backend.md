# 13a. Backend testing (Go)

Детали unit / integration / contract для Go-сервисов LMA. Политика и CI — в [13-testing.md](./13-testing.md). Правила нормализации — [15-normalization-rules.md](./15-normalization-rules.md).

---

## Содержание

1. [Инструменты](#1-инструменты)
2. [Unit: normalize и pure logic](#2-unit-normalize-и-pure-logic)
3. [Unit: HH adapter + fixtures](#3-unit-hh-adapter--fixtures)
4. [Unit: checkpoint / content_hash](#4-unit-checkpoint--content_hash)
5. [Integration: PostgreSQL](#5-integration-postgresql)
6. [Integration: Redis](#6-integration-redis)
7. [Integration: HH через httptest](#7-integration-hh-через-httptest)
8. [Phase 2: Kafka / Redpanda](#8-phase-2-kafka--redpanda)
9. [Contract: OpenAPI](#9-contract-openapi)
10. [Contract: gRPC / buf](#10-contract-grpc--buf)
11. [Contract: BFF ↔ Query](#11-contract-bff--query)
12. [Паттерны кода (ориентир)](#12-паттерны-кода-ориентир)

---

## 1. Инструменты

| Задача | Выбор |
|--------|-------|
| Unit | стандартный `testing`, table-driven |
| Asserts | `github.com/stretchr/testify` (`require` / `assert`) |
| HTTP fake | `net/http/httptest` |
| Часы | интерфейс `Clock` + fake (`now` фиксирован) — **без** `time.Now()` в политике FX/date |
| Integration DB/cache | `testcontainers-go` modules postgres/redis **или** сервисы из Compose в CI service containers |
| Migrations | `golang-migrate` CLI в job + вызов из integration setup |
| Proto | `buf lint`, `buf breaking --against '.git#branch=production'` |
| OpenAPI | lint в CI; опц. schemathesis nightly; validation middleware тестируется unit/integration |

Теги:

```go
//go:build integration

package repo_test
```

```bash
go test ./...                           # unit only (integration файлы исключены)
go test -tags=integration ./...         # с Docker
```

---

## 2. Unit: normalize и pure logic

**Пакет (целевой):** `libs/go-common/normalize` (или `apps/normalizer/internal/...` на Phase 1 — один канон, без дублирования правил).

### Обязательные table-driven кейсы

| Группа | Кейс | Ожидание |
|--------|------|----------|
| **mid** | from+to | `(from+to)/2` |
| | только from / только to | mid = from / to |
| | оба null | mid null; в demand, не в salary sample |
| | from > to | swap |
| | from/to ≤ 0 | invalid → null + metric path |
| **gross/net** | gross=true, RUB | net ≈ mid × (1 − TAX_RATE), default 0.13 |
| | gross=false / null | без tax; null → как net + metric |
| **FX** | USD + rate на дату collect | `salary_mid_rub` = mid × rate |
| | HH `RUR` | currency `RUB`, без лишнего FX |
| | FX miss + fallback исчерпан | `salary_mid_rub=null`, metric |
| **outliers** | mid_rub < 10_000 или > 2_000_000 | exclude from salary agg; demand ок |
| **remote** | work_format remote | `is_remote=true` |
| | только keyword | нужен второй сигнал |
| **role** | alias hit | `role_id` set |
| | miss | null + unmapped metric |
| **skill** | known / unknown MVP upsert-by-slug | дедуп в одной вакансии |
| **offered vs survey** | — | тесты analytics helpers не читают survey в `median_salary` |

Фикстуры-компаньоны: `salary_absent.json`, `salary_invalid_outlier.json`, `salary_fx_miss.json`, `salary_rur_to_rub.json` — см. [`testdata/hh/README.md`](../../testdata/hh/README.md).

### Правила unit

- Нет сети, нет реального PG/Redis.
- FX rates — stub map `date → rate`.
- `TAX_RATE` / outlier thresholds — через config в тесте, не через `.env` файл с секретами.
- Предпочтительно golden struct / `cmp.Diff` для draft после normalize.

```bash
go test ./libs/go-common/normalize/...
go test ./apps/normalizer/...
```

---

## 3. Unit: HH adapter + fixtures

**Пакет:** `apps/ingest/internal/hh` (имя ориентир).

| Тест | Вход | Assert |
|------|------|--------|
| Search page parse | `vacancy_search_page.json` | N items, ids, page/pages/found |
| Detail map | `vacancy_detail.json` | `external_id`, title, salary_*, area, employer, skills |
| Missing salary | `salary_absent.json` | draft валиден, salary null |
| Outlier raw | `salary_invalid_outlier.json` | adapter **не** режет outlier — передаёт source values; режет normalizer |
| RUR | `salary_rur_to_rub.json` | draft raw currency сохраняет source fact; normalize → RUB |
| HTML description | detail с тегами | strip script/tags по политике adapter |

Граница ответственности:

```text
adapter: HH JSON → SourceNeutralDraftV1   (без shared salary policy)
normalizer: draft → canonical vacancy     (все правила 15)
```

Тесты адаптера **не** должны требовать `HH_USER_AGENT` для parse-from-file; UA обязателен только у live HTTP client (отдельный unit: «empty UA → fail fast»).

```bash
go test ./apps/ingest/internal/hh/...
```

---

## 4. Unit: checkpoint / content_hash

### content_hash

- Вход: канонический subset (title, salary_*, area, employer, skills set, published_at).
- **Не** включает `collected_at`.
- Порядок skills не влияет (сортировка перед hash).
- Изменение salary → новый hash; повтор с тем же телом → skip body upsert.

### Checkpoint cursor (pure)

Логика Phase 1 ([00-overview](./00-overview.md)):

| Сценарий | Ожидание |
|----------|----------|
| Все drafts страницы normalize+save OK | cursor/page можно сохранить |
| Ошибка на N-й вакансии страницы | cursor **не** двигается |
| Повтор страницы | idempotent upsert по `(source, external_id)` |
| Пустая страница / last page | run completes; checkpoint terminal state |

Вынести decision-функцию в pure package — тестировать без PG; integration потом проверяет запись в `ingest_checkpoints`.

---

## 5. Integration: PostgreSQL

**Цель:** repositories, constraints, миграции.

### Setup

1. Поднять Postgres 16 (testcontainers или CI service).
2. `migrate -path db/migrations -database $DATABASE_URL up`
3. Опционально seed из `testdata/seeds/`.
4. В конце: down или disposable container.

### Сценарии Phase 1

| Сценарий | Assert |
|----------|--------|
| migrate up на пустой БД | success |
| migrate down → up | success (smoke) |
| upsert vacancy same `(source, external_id)` | одна строка; updated fields |
| same `content_hash` | noop body / touch collected_at per policy |
| unique violation concurrent | трактуется как success (idempotent) |
| summary query seeded | median/sample_size согласованы с outlier flags |
| soft-delete / `is_active` | analytics фильтр |

**Не** использовать production Supabase URL с реальными данными в CI — только ephemeral DB.

```bash
go test -tags=integration ./apps/query/internal/repo/...
go test -tags=integration ./apps/ingest/internal/store/...
```

---

## 6. Integration: Redis

| Сценарий | Assert |
|----------|--------|
| cache miss summary | loader called; value set |
| cache hit | loader not called; same payload |
| version bump / key prefix change | miss после инвалидации |
| Redis down | Query degrade to PG (не обязательный 5xx) — если реализовано |

Fake redis (`miniredis`) допустим для unit cache layer; **хотя бы один** testcontainers/redis integration на production.

---

## 7. Integration: HH через httptest

Полный путь страницы **без** интернета:

1. `httptest.NewServer` отдаёт JSON из `testdata/hh/`.
2. Ingest client: `HH_BASE_URL=server.URL`, UA тестовый.
3. Run page → drafts → normalize → PG (можно sqlmock только если не проверяете SQL; лучше real PG на production).

| Сценарий | Assert |
|----------|--------|
| 200 search + detail | vacancies persisted |
| 429 + Retry-After | backoff invoked; eventual success (fake clock) |
| 500 × N | run partial/fail per policy |
| Fixture mode admin trigger | `POST /admin/ingest/runs` создаёт run без внешнего HH |

Живой HH в CI — **запрещён** для автоматических PR/production gates.

---

## 8. Phase 2: Kafka / Redpanda

| Тест | Как |
|------|-----|
| Produce `vacancies.raw` envelope | testcontainers Redpanda / kafka module |
| Consumer normalize idempotent | same message 2× → one OLTP effect |
| Poison → DLQ | invalid schema after N retries |
| Contract schema_version | unknown version → safe fail / metric |

До Phase 2 эти тесты не блокируют Phase 0–1. Foundation `signals.raw` — отдельные fixtures в Phase 5.

---

## 9. Contract: OpenAPI

Канон: [`api/openapi.yaml`](../../api/openapi.yaml).

| Проверка | Когда |
|----------|-------|
| Lint (spectral/redocly) | каждый PR |
| BFF: request validation против схемы (middleware) | unit/integration handlers |
| Response shape golden / schemathesis | production или nightly |
| Breaking change detection | review + lint; major → `/api/v2` |

Минимальные пути для контрактных тестов Phase 1:

- `GET /api/v1/health` (или `/healthz` — как зафиксировано в OpenAPI)
- `GET /api/v1/dashboard/summary`
- `GET /api/v1/roles` (pagination envelope)
- `POST /api/v1/admin/ingest/runs` (auth header stub)
- ошибки: `400 VALIDATION_ERROR`, `502 DEPENDENCY_UNAVAILABLE`, `409 CONFLICT`

---

## 10. Contract: gRPC / buf

Канон: [`libs/proto/lma/`](../../libs/proto/lma/).

```bash
buf lint
buf breaking --against '.git#branch=production'
```

| Фаза | Ожидание |
|------|----------|
| Phase 0–1 | proto могут быть stubs; lint всё равно |
| Phase 2 | BFF↔Query gRPC real; breaking check required |
| Phase 5 | новые RPC Perspectives — additive предпочтительнее breaking |

---

## 11. Contract: BFF ↔ Query

Consumer-driven lite (без полного Pact на старте):

1. **Fake Query** (interface / bufconn) в тестах BFF — проверяет HTTP JSON и error mapping.
2. **Mapper tests:** gRPC/domain → BFF DTO (поля `period`, `median_salary`, `salary_sample_size`, `salary_currency=RUB`).
3. С Phase 2: один integration BFF→Query на localhost/bufconn с in-memory или test DB.

Сценарии:

| # | Сценарий | Ожидание |
|---|----------|----------|
| 1 | GetDashboardSummary OK | 200 JSON schema lite |
| 2 | ListRoles pagination | `data/page/page_size/total` |
| 3 | Query down | 502 `DEPENDENCY_UNAVAILABLE` |
| 4 | bad `from`/`to` | 400 до вызова Query |
| 5 | ingest already running | 409 `CONFLICT` |

```bash
make test-contract
# go test ./apps/bff/internal/api/contract/...
```

---

## 12. Паттерны кода (ориентир)

### Table-driven

```go
func TestSalaryMid(t *testing.T) {
    tests := []struct {
        name string
        from, to *int64
        want     *int64
    }{
        // ...
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := normalize.Mid(tt.from, tt.to)
            require.Equal(t, tt.want, got)
        })
    }
}
```

### Чтение фикстуры

```go
raw := testdata.Read(t, "hh/vacancy_detail.json") // helper от корня модула / embed
draft, err := hhadapter.ParseDetail(raw)
require.NoError(t, err)
```

### Fake clock

```go
type fixedClock struct{ t time.Time }
func (c fixedClock) Now() time.Time { return c.t }
```

Использовать в FX `rate_date` и backoff тестах.

---

## Coverage commands (ориентир)

```bash
go test -cover ./libs/go-common/normalize/...
go test -coverprofile=coverage.out ./libs/go-common/normalize/...
# порог >80% для normalize — проверять в CI script или вручную на Phase 1
```
