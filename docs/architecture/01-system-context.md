# 01. System Context (C4)

## Context diagram

```mermaid
C4Context
  title IT Labor Market Analytics — System Context

  Person(analyst, "Аналитик / пользователь", "Смотрит дашборд зарплат и спроса")
  Person(ops, "Ops / Admin", "Запускает re-ingest, смотрит jobs")

  System(lms, "Labor Market Analytics", "Сбор, нормализация, аналитика вакансий")

  System_Ext(hh, "HeadHunter API", "Вакансии, словари areas/roles")
  System_Ext(sj, "SuperJob API", "Вакансии (Target)")
  System_Ext(remotive, "Remotive API", "Remote jobs (Target)")
  System_Ext(adzuna, "Adzuna API", "Агрегатор вакансий (Target)")
  System_Ext(ai, "AI Provider", "OpenAI-compatible / local LLM (Target)")
  System_Ext(edu, "Edu platforms", "Курсы / learning signals (Phase 5)")
  System_Ext(media, "News / articles", "RSS / API mentions (Phase 5)")
  System_Ext(gh, "Container Registry / GitHub", "CI образы, деплой артефакты")

  Rel(analyst, lms, "HTTPS REST / SPA")
  Rel(ops, lms, "HTTPS admin endpoints")
  Rel(lms, hh, "HTTPS REST, User-Agent, rate limits")
  Rel(lms, sj, "HTTPS (Target)")
  Rel(lms, remotive, "HTTPS (Target)")
  Rel(lms, adzuna, "HTTPS (Target)")
  Rel(lms, edu, "HTTPS / API (Phase 5)")
  Rel(lms, media, "HTTPS / RSS (Phase 5)")
  Rel(lms, ai, "HTTPS inference (Target)")
  Rel(lms, gh, "Pull images (deploy)")
```

Если рендер C4 в viewer недоступен, эквивалент:

```mermaid
flowchart LR
  Analyst[Аналитик]
  Ops[Ops / Admin]
  LMS[Labor Market Analytics]
  HH[HeadHunter API]
  SJ[SuperJob / Remotive / Adzuna]
  EduNews[Edu / News / Articles]
  AI[AI Provider]
  Reg[Container Registry]

  Analyst -->|HTTPS SPA/API| LMS
  Ops -->|HTTPS admin| LMS
  LMS -->|poll vacancies| HH
  LMS -->|Target jobs| SJ
  LMS -->|Phase 5 signals| EduNews
  LMS -->|async prompts| AI
  LMS -->|pull images| Reg
```

## Container diagram

```mermaid
flowchart TB
  subgraph TrustPublic["Trust boundary: Internet"]
    Browser[Browser React SPA]
    HH[HH API]
    ExtSrc[Other job APIs]
    LLM[AI Provider]
  end

  subgraph TrustCluster["Trust boundary: Kubernetes cluster"]
    Ingress[Ingress Controller]
    GW[API Gateway<br/>:8080 HTTP]
    BFF[BFF<br/>:8081 HTTP]
    Query[Query Analytics<br/>:8083 HTTP / :9091 gRPC]
    Ingest[Ingest<br/>:8082 HTTP / :9092 gRPC]
    Norm[Normalizer Worker]
    Sched[Scheduler]
    AIW[AI Analyzer Worker]
    Kafka[(Kafka)]
    PG[(PostgreSQL)]
    CH[(ClickHouse)]
    Redis[(Redis)]
  end

  Browser --> Ingress --> GW --> BFF
  BFF --> Query
  BFF --> Ingest
  Sched --> Ingest
  Ingest --> HH
  Ingest --> ExtSrc
  Ingest --> Kafka
  Kafka --> Norm
  Kafka --> AIW
  Norm --> PG
  Norm --> CH
  Query --> PG
  Query --> CH
  Query --> Redis
  AIW --> LLM
  AIW --> PG
  AIW --> CH
```

## External systems

| Система | Назначение | Протокол | Фаза | Заметки |
|---------|------------|----------|------|---------|
| **HeadHunter** | Основной источник вакансий и словарей | HTTPS REST | MVP | Обязательный `User-Agent` с контактами; 429 backoff |
| **SuperJob** | Доп. источник RU | HTTPS REST | Target | Отдельный adapter + credentials |
| **Remotive** | Remote/global IT jobs | HTTPS REST | Target | Часто более простая схема полей |
| **Adzuna** | Агрегатор, multi-country | HTTPS REST | Target | Полезно для бенчмарков стран |
| **Survey benchmarks** | Ручные/файловые бенчмарки зарплат | CSV/API | Target | Не вакансии; отдельный import path |
| **AI Provider** | Insights по кластерам/трендам | HTTPS (OpenAI-compatible) | Target (Phase 4) | Абстракция клиента; cost/rate limits |
| **Edu platforms** | Learning-interest signals («Тенденции») | HTTPS API | Phase 5 Target | Кандидаты: Stepik/Coursera/Skillbox — API TBD; см. [21](./21-external-services.md) |
| **News / articles** | Media-attention signals | HTTPS / RSS | Phase 5 Target | Официальные feeds/API only; Habr и др. — кандидат |
| **Container Registry** | Docker images для k8s | HTTPS | Phase 3 | GHCR / Docker Hub |

## Trust boundaries

```mermaid
flowchart TB
  subgraph Internet
    User[User browser]
    ExtAPIs[Job boards + AI + edu/news]
  end

  subgraph DMZ["Edge"]
    Ingress[TLS termination / Ingress]
  end

  subgraph AppNet["App network (ClusterIP)"]
    Services[Gateway, BFF, Query, Ingest, Workers]
  end

  subgraph DataNet["Data plane"]
    Stores[PG, CH, Redis, Kafka]
  end

  User -->|443 only| Ingress
  Ingress --> Services
  Services -->|egress allowlist| ExtAPIs
  Services --> Stores
```

| Граница | Правило |
|---------|---------|
| Internet → Cluster | Только Ingress (SPA static + `/api/*` → gateway). gRPC **не** публиковать наружу |
| Gateway → BFF → internal | mTLS опционально (Target); в MVP — private network |
| Egress | Allowlist: HH, будущие job APIs, edu/news (Phase 5), AI provider, registry, DNS |
| Secrets | API tokens HH/AI только в K8s Secrets / local `.env` (gitignored) |
| PII | Не кэшировать и не слать в LLM контактные данные кандидатов/работодателей сверх необходимости аналитики |
| Admin | Trigger ingest — отдельный путь + будущий authz role `admin` |

## Data sensitivity (кратко)

| Данные | Чувствительность | Где |
|--------|------------------|-----|
| Текст вакансии, зарплата, навыки | Бизнес-публичные (уже на job board) | PG, CH |
| Employer name | Низкая/средняя | PG, CH |
| Контакты из описания | PII — **не** для AI, по возможности не хранить отдельно | strip before AI |
| HH/AI API keys | Secret | Secret store |
| Aggregated trends | Низкая | CH, Redis cache |

## Actors & use cases

| Actor | Use cases |
|-------|-----------|
| Аналитик | Открыть dashboard, фильтры роль/регион/период, salary trends, top skills; Phase 5 — экран «Тенденции» |
| Ops | Ручной re-ingest, просмотр статуса job, DLQ replay (Target) |
| Scheduler | Периодический trigger ingest |
| AI worker (system) | Забрать job → вызвать LLM → сохранить insight |
