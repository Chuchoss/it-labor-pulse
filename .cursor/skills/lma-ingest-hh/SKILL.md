---
name: lma-ingest-hh
description: >-
  Ограничения и практика HH ingest для LMA: User-Agent, rate limits, adapter
  pattern, фикстуры testdata/hh, ToS и attribution. Use when implementing or
  changing HH collector/adapter, ingest runs, rate limiting, HH fixtures, or
  when the user mentions HeadHunter, hh.ru, ingest, or vacancy scraping.
---

# LMA Ingest HH

## Hard constraints

1. Только **официальный HH API** (`HH_BASE_URL`, default `https://api.hh.ru`).
2. **Запрещено** scraping LinkedIn, Avito и прочих площадок без API/ToS.
3. Новый источник = **adapter**, без переписывания normalize/query core.
4. `HH_APP_TOKEN` / secrets — только server-side; не в React, не в git.

## User-Agent (обязательно)

- Env: `HH_USER_AGENT` — идентифицирующая строка с контактом.
- Fail-fast при пустом UA на старте ingest.
- Пример: `LMAStudyProject/0.1 (+https://github.com/<user>/study_project; you@example.com)`
- Клиент реально должен слать заголовок `User-Agent`.

## Rate limits & reliability

- На **429/5xx**: exponential backoff; низкий parallelism; page delay.
- Distributed lock на source (Redis) — с Phase 2; до этого не долбить API конкурентно.
- Schedule: daily/incremental, не real-time flood.
- Idempotency downstream: `(source, external_id)`.

## Тесты без сети

- Фикстуры: `testdata/hh/` (`vacancy_search_page.json`, `vacancy_detail.json`).
- Unit/CI адаптера: читать файлы / httptest — **без** живого HH.
- Фикстуры анонимные (см. `testdata/hh/README.md`); не коммитить PII.
- Смена схемы HH → обновить фикстуру + assert.
- Стратегия и DoD: `docs/architecture/13-testing.md` / `13a-testing-backend.md`.

## Normalize handoff

- Mapping в канон — по `docs/architecture/15-normalization-rules.md`.
- Offered salaries only в default analytics; attribution HH в UI/README.

## Docs to read

- `docs/architecture/08-integrations-and-extensibility.md`
- `docs/architecture/11-observability-security.md` (ToS/UA)
- `docs/architecture/12-local-dev.md` (env, troubleshooting 403)
- `docs/architecture/13-testing.md`
- Детали полей/лимитов: [reference.md](reference.md)
