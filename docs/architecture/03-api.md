# 03. API

## Versioning strategy

| API | Strategy |
|-----|----------|
| Public REST | Prefix `/api/v1/...`. Breaking changes → `/api/v2`. Additive fields OK в minor |
| Internal gRPC | Package `lma.<domain>.v1`; breaking → `v2` package + parallel deploy |
| Kafka messages | Поле `schema_version` в envelope; consumers backward-compatible |
| AI prompts | `prompt_version` string в БД, не в URL |

Deprecation: минимум один релиз с `Sunset` header / docs warning.

---

## Public REST (Client → BFF)

Публичная точка входа MVP — **BFF** (`:8080` local). OpenAPI описывает **BFF product API** (см. [ADR 010](./adr/010-api-gateway.md)). Отдельный gateway — **Target** Phase 3+ (optional).

Base URL: `https://{host}/api/v1` (через BFF; local `http://localhost:8080/api/v1`)  
Content-Type: `application/json`  
Auth: **MVP** — отсутствует (local/dev). **Target** — auth stub на edge (BFF или будущий gateway) + `Authorization: Bearer <JWT>` / `X-API-Key`.

### Error model

```json
{
  "error": {
    "code": "RATE_LIMITED",
    "message": "Too many requests",
    "details": { "retry_after_sec": 30 },
    "request_id": "01JABC..."
  }
}
```

| HTTP | code (пример) | Когда |
|------|---------------|-------|
| 400 | `VALIDATION_ERROR` | Bad query/body |
| 401 | `UNAUTHORIZED` | Target auth |
| 403 | `FORBIDDEN` | Target authz |
| 404 | `NOT_FOUND` | Unknown id |
| 409 | `CONFLICT` | Ingest already running |
| 429 | `RATE_LIMITED` | Edge limit |
| 502/503 | `DEPENDENCY_UNAVAILABLE` | gRPC/CH down |
| 500 | `INTERNAL` | Unexpected |

### Pagination

Query params:

| Param | Default | Max |
|-------|---------|-----|
| `page` | 1 | — |
| `page_size` | 20 | 100 |

Response envelope:

```json
{
  "data": [],
  "page": 1,
  "page_size": 20,
  "total": 1234
}
```

Cursor-pagination (Target) для больших списков: `cursor`, `next_cursor`.

### Common query filters

Фильтр поддерживается только теми endpoint, где он перечислен ниже; это не общий обязательный набор для каждого маршрута.

| Param | Type | Описание |
|-------|------|----------|
| `role_id` | uuid/string | Каноническая роль |
| `region_id` | uuid/string | Регион |
| `from` | date ISO | Начало периода |
| `to` | date ISO | Конец периода |
| `source` | enum | `hh`, `superjob`, ... |
| `currency` | — | В MVP параметр отсутствует: все salary-поля и агрегаты возвращаются только в `RUB` |

---

### Endpoints

#### `GET /api/v1/dashboard/summary`

Сводка для главной.

**Query:** `from`, `to`, `region_id?`, `role_id?`

**200 Response:**

```json
{
  "period": { "from": "2026-07-01", "to": "2026-08-01" },
  "vacancies_active": 15234,
  "vacancies_new": 1204,
  "median_salary": 250000,
  "salary_currency": "RUB",
  "salary_sample_size": 8901,
  "top_roles": [
    { "role_id": "role_go_dev", "title": "Go Developer", "count": 420 }
  ],
  "top_regions": [
    { "region_id": "reg_msk", "title": "Москва", "count": 5100 }
  ],
  "generated_at": "2026-08-11T12:00:00Z",
  "cache": "HIT"
}
```

#### `GET /api/v1/roles`

Список ролей со спросом.

**Query:** filters + pagination + `sort=count|median_salary`

```json
{
  "data": [
    {
      "role_id": "role_go_dev",
      "title": "Go Developer",
      "vacancies_count": 420,
      "median_salary": 280000,
      "p25_salary": 220000,
      "p75_salary": 350000,
      "currency": "RUB"
    }
  ],
  "page": 1,
  "page_size": 20,
  "total": 85
}
```

#### `GET /api/v1/roles/{role_id}`

Детали роли. Временные ряды запрашиваются отдельными `/trends/*` маршрутами.

#### `GET /api/v1/regions`

Аналогично roles: count, median salary.

#### `GET /api/v1/regions/{region_id}`

#### `GET /api/v1/trends/salaries`

Ряд медиан/перцентилей.

**Query:** `from`, `to`, `role_id?`, `region_id?`, `grain=day|week|month`. Валюта MVP фиксирована: `RUB`.

```json
{
  "grain": "week",
  "currency": "RUB",
  "points": [
    {
      "period_start": "2026-07-01",
      "median": 245000,
      "p25": 200000,
      "p75": 310000,
      "sample_size": 1200
    }
  ]
}
```

#### `GET /api/v1/trends/demand`

Количество активных/опубликованных вакансий из persisted snapshots.

**Query:** `from`, `to`, `role_group?`, `region_id?`, `source?`,
`grain=day|week`. Группы: `software_development`, `analytics`,
`quality_assurance`.

- `active_count` никогда не реконструируется из текущего `vacancies`;
- `published_count` — `published_at` в UTC-окне snapshot;
- `new_count` — deprecated alias `published_count`;
- weekly API возвращает только `complete=true` (`source_day_count=7`);
- до первого полного all-IT cycle ответ: `status=no_complete_snapshots`,
  `points=[]`, без фиктивных точек.

#### `GET /api/v1/trends/coverage`

Покрытие для `/market`: `available_years`, first/last observation,
publication range, число complete daily/weekly snapshots, latest complete cycle,
source/method version и регионы, присутствующие в snapshots. До первого снимка
возвращает `status=collecting` и пустые годы.

#### `GET /api/v1/trends/perspectives` (Target, Phase 5)

Рейтинг IT-направлений по composite heuristic score («Тенденции»).  
**Не** путать с `/trends/salaries` и `/trends/demand` (vacancy-only MVP).

**Query:** `from`, `to`, `role_family?`, `page`, `page_size`, опц. `score_version`

```json
{
  "period": { "from": "2026-07-01", "to": "2026-08-01" },
  "score_version": "perspectives_v1",
  "disclaimer": "Composite heuristic from jobs, learning, and media signals — not a forecast.",
  "data": [
    {
      "direction_key": "platform-engineering",
      "title": "Platform Engineering",
      "role_family": "backend",
      "composite_score": 0.82,
      "demand_component": 0.9,
      "learning_component": 0.7,
      "media_component": 0.75,
      "coverage": { "jobs": true, "edu": true, "media": true },
      "delta": 0.05
    }
  ],
  "page": 1,
  "page_size": 20,
  "total": 40
}
```

OpenAPI: `x-lifecycle: target`.

#### `GET /api/v1/trends/perspectives/{direction_key}` (Target, Phase 5)

Деталь направления: ряд `composite_score` + компоненты по `grain`.

#### `GET /api/v1/skills/top`

**Query:** filters + `limit` (default 20)

```json
{
  "data": [
    { "skill_id": "sk_kubernetes", "name": "Kubernetes", "count": 980, "share": 0.18 }
  ]
}
```

#### `GET /api/v1/vacancies`

Поиск/список (OLTP). Для UI drill-down, не для тяжёлой аналитики.

**Query:** `q`, `role_id`, `region_id`, `skill_id`, `salary_min`,
`salary_max`, `source`, `only_active`, pagination.

- `role_id`, `region_id`, `skill_id`: один UUID или до 20 UUID через запятую;
  внутри одного фильтра семантика **ANY**. Для навыков достаточно совпадения
  хотя бы одного выбранного skill.
- `salary_min` / `salary_max`: границы по канонической `salary_mid` в RUB,
  приведённой к оценке net. При заданной границе вакансии без salary не
  совпадают.
- Публичная выдача всегда ограничена утверждёнными семействами ролей
  `software_development`, `analytics`, `quality_assurance`; unresolved и
  out-of-scope строки не возвращаются даже с `only_active=false`.
- Сортировка стабильна: `published_at DESC NULLS LAST, id`; смена фильтра
  начинает пагинацию с первой страницы.

```json
{
  "data": [
    {
      "id": "3fa85f64-5717-4562-b3fc-2c963f66afa6",
      "source": "hh",
      "external_id": "123456",
      "title": "Senior Go Developer",
      "role_id": "role_go_dev",
      "region_id": "reg_msk",
      "salary_from": 300000,
      "salary_to": 400000,
      "salary_currency": "RUB",
      "salary_gross": true,
      "published_at": "2026-08-10T10:00:00Z",
      "is_active": true,
      "skills": ["Go", "Kafka", "PostgreSQL"]
    }
  ],
  "page": 1,
  "page_size": 20,
  "total": 420
}
```

#### `POST /api/v1/admin/ingest/runs` (admin)

Ручной запуск ingest.

**Body:**

```json
{
  "source": "hh",
  "mode": "incremental",
  "params": {
    "area": "1",
    "text": "golang",
    "date_from": "2026-08-10"
  }
}
```

`mode`: `full` | `incremental`

**202 Accepted:**

```json
{
  "run_id": "01JRUN...",
  "status": "queued",
  "source": "hh"
}
```

**409** если lock уже держится.

#### `GET /api/v1/admin/ingest/runs/{run_id}`

```json
{
  "run_id": "01JRUN...",
  "source": "hh",
  "mode": "incremental",
  "status": "running",
  "started_at": "2026-08-11T10:00:00Z",
  "finished_at": null,
  "stats": {
    "fetched": 500,
    "published": 500,
    "errors": 2
  },
  "error_message": null
}
```

`status`: `queued|running|success|partial|failed`

#### `POST /api/v1/admin/ai/analyses` (Target)

```json
{
  "type": "role_trend",
  "role_id": "role_go_dev",
  "region_id": "reg_msk",
  "from": "2026-05-01",
  "to": "2026-08-01",
  "prompt_version": "trend_v1"
}
```

**202:**

```json
{
  "job_id": "01JAI...",
  "status": "queued"
}
```

#### `GET /api/v1/ai/insights/{id}` (Target)

```json
{
  "id": "01JAI...",
  "type": "role_trend",
  "status": "completed",
  "summary": "Спрос на Go в Москве стабилен...",
  "bullets": ["Median +5% QoQ", "Kubernetes чаще в требованиях"],
  "prompt_version": "trend_v1",
  "model": "gpt-4.1-mini",
  "needs_human_review": false,
  "created_at": "2026-08-11T12:00:00Z"
}
```

#### `GET /api/v1/health`

Публичный health (или только `/healthz` без `/api`).

HTTP **200** всегда в Phase 0 (удобный liveness). Тело:

| Поле | Описание |
|------|----------|
| `status` | `ok` или `degraded` (если настроенный Postgres/Redis не отвечает) |
| `checks.database` | `up` / `down` — только если задан `DATABASE_URL` |
| `checks.redis` | `up` / `down` — только если задан `REDIS_URL` (cloud `rediss://` или local) |

Без `REDIS_URL` / `DATABASE_URL` соответствующий check не включается; процесс стартует (Phase 0 optional deps).

---

## OpenAPI sketch (фрагмент)

```yaml
openapi: 3.1.0
info:
  title: Labor Market Analytics API
  version: 1.0.0
paths:
  /api/v1/dashboard/summary:
    get:
      operationId: getDashboardSummary
      parameters:
        - in: query
          name: from
          schema: { type: string, format: date }
          required: true
        - in: query
          name: to
          schema: { type: string, format: date }
          required: true
      responses:
        "200":
          description: Summary
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/DashboardSummary"
  /api/v1/admin/ingest/runs:
    post:
      operationId: triggerIngest
      security:
        - bearerAuth: []
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: "#/components/schemas/TriggerIngestRequest"
      responses:
        "202":
          description: Accepted
        "409":
          description: Already running
components:
  securitySchemes:
    bearerAuth:
      type: http
      scheme: bearer
  schemas:
    DashboardSummary:
      type: object
      required: [period, vacancies_active, median_salary]
      # ... fields as above
```

Полный OpenAPI: [`api/openapi.yaml`](../../api/openapi.yaml) (канонический путь; держать в синхроне с этим документом).

**Публичная HTML-документация (GitHub Pages):**

- Redoc (landing): https://chuchoss.github.io/it-labor-pulse/
- Swagger UI: https://chuchoss.github.io/it-labor-pulse/swagger.html

Статика: [`docs/api-site/`](../../api-site/); workflow копирует yaml на build — см. [ADR 008](./adr/008-github-pages-openapi.md), [10-cicd.md](./10-cicd.md).

### Валюта MVP

Публичный API Phase 1 — **RUB-only**: query-параметра `currency` нет, а `salary_currency` / `currency` в ответах всегда равны `RUB`. Исходная валюта и FX сохраняются внутри нормализованной модели для аудита. Будущая multi-currency выдача потребует additive-полей с FX-date/методологией и отдельной версии контракта, а не неявного расширения MVP.

---

## Internal gRPC

Канонический package layout (stubs уже в репо):

```
libs/proto/lma/common/v1/common.proto
libs/proto/lma/ingest/v1/ingest.proto
libs/proto/lma/query/v1/query.proto
libs/proto/lma/ai/v1/ai.proto          # Target stub
```

Импорты внутри proto: `import "lma/common/v1/common.proto";`  
Кодоген (intended): `make proto` → Go пакеты под `libs/proto/gen/...`.

### `common/v1`

```protobuf
syntax = "proto3";
package lma.common.v1;

message Period {
  string from = 1; // YYYY-MM-DD
  string to = 2;
}

message PageRequest {
  int32 page = 1;
  int32 page_size = 2;
}

message PageResponse {
  int32 page = 1;
  int32 page_size = 2;
  int64 total = 3;
}

enum Source {
  SOURCE_UNSPECIFIED = 0;
  SOURCE_HH = 1;
  SOURCE_SUPERJOB = 2;
  SOURCE_REMOTIVE = 3;
  SOURCE_ADZUNA = 4;
}
```

### Ingest service

```protobuf
syntax = "proto3";
package lma.ingest.v1;

import "lma/common/v1/common.proto";

service IngestService {
  rpc StartRun(StartRunRequest) returns (StartRunResponse);
  rpc GetRun(GetRunRequest) returns (IngestRun);
  rpc ListRuns(ListRunsRequest) returns (ListRunsResponse);
}

message StartRunRequest {
  lma.common.v1.Source source = 1;
  string mode = 2; // full|incremental
  map<string, string> params = 3;
  string requested_by = 4; // admin|scheduler
}

message StartRunResponse {
  string run_id = 1;
  string status = 2;
}

message GetRunRequest { string run_id = 1; }

message IngestRun {
  string run_id = 1;
  lma.common.v1.Source source = 2;
  string mode = 3;
  string status = 4;
  string started_at = 5;
  string finished_at = 6;
  IngestStats stats = 7;
  string error_message = 8;
}

message IngestStats {
  int64 fetched = 1;
  int64 published = 2;
  int64 errors = 3;
}

message ListRunsRequest {
  lma.common.v1.PageRequest page = 1;
  lma.common.v1.Source source = 2;
}

message ListRunsResponse {
  repeated IngestRun runs = 1;
  lma.common.v1.PageResponse page = 2;
}
```

### Query service

```protobuf
syntax = "proto3";
package lma.query.v1;

import "lma/common/v1/common.proto";

service QueryService {
  rpc GetDashboardSummary(GetDashboardSummaryRequest) returns (DashboardSummary);
  rpc ListRoles(ListRolesRequest) returns (ListRolesResponse);
  rpc GetRole(GetRoleRequest) returns (RoleStat);
  rpc ListRegions(ListRegionsRequest) returns (ListRegionsResponse);
  rpc GetRegion(GetRegionRequest) returns (RegionStat);
  rpc GetSalaryTrends(GetSalaryTrendsRequest) returns (SalaryTrendsResponse);
  rpc GetDemandTrends(GetDemandTrendsRequest) returns (DemandTrendsResponse);
  rpc GetTopSkills(GetTopSkillsRequest) returns (TopSkillsResponse);
  rpc ListVacancies(ListVacanciesRequest) returns (ListVacanciesResponse);
}

message GetDashboardSummaryRequest {
  lma.common.v1.Period period = 1;
  string role_id = 2;
  string region_id = 3;
}

message DashboardSummary {
  lma.common.v1.Period period = 1;
  int64 vacancies_active = 2;
  int64 vacancies_new = 3;
  double median_salary = 4;
  string salary_currency = 5;
  int64 salary_sample_size = 6;
  string generated_at = 7;
  bool cache_hit = 8;
  repeated RoleCount top_roles = 9;
  repeated RegionCount top_regions = 10;
}

message RoleCount {
  string role_id = 1;
  string title = 2;
  int64 count = 3;
}

message RegionCount {
  string region_id = 1;
  string title = 2;
  int64 count = 3;
}

message ListRolesRequest {
  lma.common.v1.Period period = 1;
  string region_id = 2;
  lma.common.v1.PageRequest page = 3;
  string sort = 4;
  lma.common.v1.Source source = 5;
}

message GetRoleRequest {
  string role_id = 1;
  lma.common.v1.Period period = 2;
  string region_id = 3;
}

message RoleStat {
  string role_id = 1;
  string title = 2;
  int64 vacancies_count = 3;
  double median_salary = 4;
  double p25_salary = 5;
  double p75_salary = 6;
  string currency = 7;
}

message ListRolesResponse {
  repeated RoleStat roles = 1;
  lma.common.v1.PageResponse page = 2;
}

message ListRegionsRequest {
  lma.common.v1.Period period = 1;
  string role_id = 2;
  lma.common.v1.PageRequest page = 3;
  string sort = 4;
}

message GetRegionRequest {
  string region_id = 1;
  lma.common.v1.Period period = 2;
  string role_id = 3;
}

message RegionStat {
  string region_id = 1;
  string title = 2;
  int64 vacancies_count = 3;
  double median_salary = 4;
  double p25_salary = 5;
  double p75_salary = 6;
  string currency = 7;
}

message ListRegionsResponse {
  repeated RegionStat regions = 1;
  lma.common.v1.PageResponse page = 2;
}

message GetSalaryTrendsRequest {
  lma.common.v1.Period period = 1;
  string role_id = 2;
  string region_id = 3;
  string grain = 4; // day|week|month
}

message SalaryPoint {
  string period_start = 1;
  double median = 2;
  double p25 = 3;
  double p75 = 4;
  int64 sample_size = 5;
}

message SalaryTrendsResponse {
  string grain = 1;
  string currency = 2;
  repeated SalaryPoint points = 3;
}

message GetDemandTrendsRequest {
  lma.common.v1.Period period = 1;
  string role_id = 2;
  string region_id = 3;
  string grain = 4;
}

message DemandPoint {
  string period_start = 1;
  int64 active_count = 2;
  int64 new_count = 3;
}

message DemandTrendsResponse {
  string grain = 1;
  repeated DemandPoint points = 2;
}

message GetTopSkillsRequest {
  lma.common.v1.Period period = 1;
  string role_id = 2;
  string region_id = 3;
  int32 limit = 4;
}

message SkillStat {
  string skill_id = 1;
  string name = 2;
  int64 count = 3;
  double share = 4;
}

message TopSkillsResponse {
  repeated SkillStat skills = 1;
}

message ListVacanciesRequest {
  string q = 1;
  string role_id = 2;
  string region_id = 3;
  bool only_active = 4;
  lma.common.v1.PageRequest page = 5;
  lma.common.v1.Source source = 6;
}

message Vacancy {
  string id = 1;
  string source = 2;
  string external_id = 3;
  string title = 4;
  string role_id = 5;
  string region_id = 6;
  double salary_from = 7;
  double salary_to = 8;
  string salary_currency = 9;
  bool salary_gross = 10;
  string published_at = 11;
  bool is_active = 12;
  repeated string skills = 13;
}

message ListVacanciesResponse {
  repeated Vacancy vacancies = 1;
  lma.common.v1.PageResponse page = 2;
}
```

### AI service (Target)

```protobuf
syntax = "proto3";
package lma.ai.v1;

service AIService {
  rpc StartAnalysis(StartAnalysisRequest) returns (StartAnalysisResponse);
  rpc GetJob(GetJobRequest) returns (AIJob);
  rpc GetInsight(GetInsightRequest) returns (AIInsight);
}

message StartAnalysisRequest {
  string type = 1; // role_trend|vacancy_cluster|skills_shift
  string role_id = 2;
  string region_id = 3;
  string from = 4;
  string to = 5;
  string prompt_version = 6;
}

message StartAnalysisResponse {
  string job_id = 1;
  string status = 2;
}

message GetJobRequest { string job_id = 1; }

message AIJob {
  string job_id = 1;
  string type = 2;
  string status = 3;
  string error_message = 4;
  string created_at = 5;
  string finished_at = 6;
}

message GetInsightRequest { string id = 1; }

message AIInsight {
  string id = 1;
  string job_id = 2;
  string type = 3;
  string summary = 4;
  repeated string bullets = 5;
  string prompt_version = 6;
  string model = 7;
  bool needs_human_review = 8;
  string created_at = 9;
}
```

## Auth stub (MVP → Target)

| Фаза | Поведение |
|------|-----------|
| MVP | Все `/api/v1/*` открыты в dev; admin endpoints можно закрыть basic env flag `ADMIN_TOKEN` |
| Target | JWT (user) + API keys (automation); RBAC: `viewer`, `admin` |
| gRPC | Cluster-internal; optional service auth (SPIFFE/mTLS) later |

## Idempotency keys (Target)

Для `POST` admin: заголовок `Idempotency-Key` → повтор вернёт тот же `run_id`/`job_id`.
