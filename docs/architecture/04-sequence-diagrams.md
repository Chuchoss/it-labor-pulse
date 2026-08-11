# 04. Sequence Diagrams

## 1. Daily ingest: HH → Kafka → normalize → PG → ClickHouse

**Фаза:** Target core (Phase 2). В MVP Kafka может быть заменена in-process вызовом.

> Для Phase 2 выбор between transactional outbox и documented replay checkpoint — отдельный ADR. Диаграмма ниже показывает pipeline, но не утверждает атомарность PG + Kafka.

```mermaid
sequenceDiagram
  autonumber
  participant Cron as Scheduler / CronJob
  participant Ing as Ingest
  participant Redis as Redis
  participant HH as HeadHunter API
  participant Kafka as Kafka
  participant Norm as Normalizer
  participant PG as PostgreSQL
  participant CH as ClickHouse

  Cron->>Ing: StartRun(source=hh, mode=incremental)
  Ing->>Redis: SET NX lock:ingest:hh TTL=45m
  alt lock acquired
    Ing->>PG: INSERT ingest_runs(status=running)
    loop pages until empty / limit
      Ing->>HH: GET /vacancies (cursor/page)
      HH-->>Ing: items[] + pages
      Ing->>Kafka: Produce vacancies.raw.hh (batch)
    end
    Ing->>PG: UPDATE ingest_runs stats/status
    Ing->>Redis: DEL lock:ingest:hh
  else lock busy
    Ing-->>Cron: 409 Conflict
  end

  Kafka->>Norm: Consume vacancies.raw.hh
  Norm->>Norm: Validate + map role/skills/salary
  Norm->>PG: UPSERT vacancies, links (dedup)
  Norm->>CH: INSERT fact_vacancy_snapshot
  Norm->>Kafka: Produce vacancies.normalized (optional fan-out)
```

### Phase 1: commit checkpoint страницы

```mermaid
sequenceDiagram
  autonumber
  participant Ing as Ingest
  participant HH as HeadHunter API
  participant Adapter as HH adapter
  participant Norm as In-process normalizer
  participant PG as PostgreSQL

  Ing->>PG: Read ingest_checkpoint(source, scope)
  Ing->>HH: GET /vacancies(cursor)
  HH-->>Ing: page items + next_cursor
  Ing->>Adapter: ToDraftV1 for every item
  Adapter-->>Ing: SourceNeutralDraftV1[]
  Ing->>Norm: NormalizeAndStorePage(drafts)
  Norm->>PG: UPSERT all vacancy/dictionary changes
  alt all drafts stored
    Norm-->>Ing: success
    Ing->>PG: UPDATE ingest_checkpoint(next_cursor)
  else any draft/store fails
    Norm-->>Ing: error
    Note over Ing,PG: Do not update checkpoint; retry whole page
  end
```

## 2. Dashboard read path — Redis cache hit / miss

```mermaid
sequenceDiagram
  autonumber
  participant UI as React SPA
  participant BFF as BFF
  participant Q as Query
  participant Redis as Redis
  participant CH as ClickHouse
  participant PG as PostgreSQL

  UI->>BFF: GET /api/v1/dashboard/summary?from&to
  BFF->>Q: GetDashboardSummary(period)
  Q->>Redis: GET cache:dashboard:summary:{hash}

  alt HIT
    Redis-->>Q: JSON payload
    Q-->>BFF: DashboardSummary(cache_hit=true)
    BFF-->>UI: 200 + cache:HIT
  else MISS
    Redis-->>Q: nil
    Q->>CH: aggregate active/new/median
    opt dictionaries / names
      Q->>PG: SELECT roles/regions titles
    end
    Q->>Redis: SETEX cache key TTL=5m
    Q-->>BFF: DashboardSummary(cache_hit=false)
    BFF-->>UI: 200 + cache:MISS
  end
```

## 3. Manual re-ingest (admin)

```mermaid
sequenceDiagram
  autonumber
  participant Ops as Ops / Admin UI
  participant BFF as BFF
  participant Ing as Ingest
  participant Redis as Redis
  participant PG as PostgreSQL
  participant Kafka as Kafka

  Ops->>BFF: POST /api/v1/admin/ingest/runs {source,mode,params}
  Note over BFF: Target: check admin JWT/role
  BFF->>Ing: StartRun(...)
  Ing->>Redis: SET NX lock:ingest:{source}
  alt ok
    Ing->>PG: create run queued/running
    Ing-->>BFF: run_id, status=queued
    BFF-->>Ops: 202 Accepted
    Ing->>Kafka: (async) fetch+produce as in daily flow
  else locked
    Ing-->>BFF: FailedPrecondition
    BFF-->>Ops: 409 CONFLICT
  end

  Ops->>BFF: GET /api/v1/admin/ingest/runs/{run_id}
  BFF->>Ing: GetRun
  Ing->>PG: SELECT run
  Ing-->>BFF: status+stats
  BFF-->>Ops: 200
```

## 4. Future AI analysis flow (async job)

```mermaid
sequenceDiagram
  autonumber
  participant UI as React / Admin
  participant BFF as BFF
  participant AIApi as AI Service gRPC
  participant PG as PostgreSQL
  participant Kafka as Kafka
  participant Worker as AI Analyzer
  participant CH as ClickHouse
  participant LLM as AI Provider

  UI->>BFF: POST /api/v1/admin/ai/analyses
  BFF->>AIApi: StartAnalysis(...)
  AIApi->>PG: INSERT ai_jobs(status=queued)
  AIApi->>Kafka: Produce ai.jobs
  AIApi-->>BFF: job_id queued
  BFF-->>UI: 202

  Kafka->>Worker: Consume ai.jobs
  Worker->>PG: UPDATE status=running
  Worker->>CH: Load trend series / vacancy sample
  Worker->>PG: Load role metadata (no PII)
  Worker->>Worker: Build prompt (prompt_version)
  Worker->>LLM: ChatCompletion
  alt success
    LLM-->>Worker: insight JSON
    Worker->>PG: INSERT ai_insights; job=completed
  else provider error / 429
    LLM-->>Worker: error
    Worker->>PG: job=retrying/failed
    Worker->>Kafka: retry or DLQ ai.jobs.dlq
  end

  UI->>BFF: GET /api/v1/ai/insights/{id}
  BFF->>AIApi: GetInsight
  AIApi->>PG: SELECT
  AIApi-->>BFF: insight
  BFF-->>UI: 200
```

## 5. HH rate-limit / 429 backoff

```mermaid
sequenceDiagram
  autonumber
  participant Ing as Ingest HH client
  participant HH as HeadHunter API
  participant Metrics as Metrics

  loop fetch page
    Ing->>HH: GET /vacancies?...
    alt 200 OK
      HH-->>Ing: items
      Ing->>Ing: reset consecutive_429
    else 429 Too Many Requests
      HH-->>Ing: 429 + Retry-After
      Ing->>Metrics: hh_429_total++
      Ing->>Ing: sleep max(Retry-After, exp_backoff+jitter)
      Note over Ing: attempt++ ; if attempt>max → fail run partial
      Ing->>HH: retry same page
    else 5xx / timeout
      HH-->>Ing: error
      Ing->>Ing: exp backoff retry
    end
  end
```

## 6. Soft-delete / deactivate missing vacancies (incremental)

```mermaid
sequenceDiagram
  autonumber
  participant Ing as Ingest
  participant Norm as Normalizer
  participant PG as PostgreSQL
  participant CH as ClickHouse

  Note over Ing: Run видит набор external_id за окно/поисковый срез
  Ing->>Norm: raw events + run checkpoint (seen ids)
  Norm->>PG: UPSERT seen vacancies is_active=true
  Norm->>PG: Mark candidates not seen in slice<br/>is_active=false, deleted_at=now()
  Norm->>CH: Snapshot with is_active flag for that day
```

> Политика «не увидели = inactive» осторожная: только для scoped search (например area+role), не глобально по всей HH за один проход. Полный reconcile — отдельный weekly job (Target).

## 7. Cache invalidation after successful normalize batch (optional)

```mermaid
sequenceDiagram
  participant Norm as Normalizer
  participant Redis as Redis

  Norm->>Norm: commit PG/CH batch
  Norm->>Redis: DEL cache:dashboard:* / cache:trends:* (by prefix/version)
  Note over Redis: Предпочтительно bump cache_version<br/>вместо массового DEL
```
