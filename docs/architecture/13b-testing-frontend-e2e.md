# 13b. Frontend и E2E testing

React (`apps/web`) и сквозные сценарии Playwright. Политика / CI — [13-testing.md](./13-testing.md). IA экранов — [16-frontend.md](./16-frontend.md). API — [03-api.md](./03-api.md).

Market `/market`: component tests через MSW обязаны проверять годы из coverage
API, collecting state без запроса series, published/active labels, disabled
weekly grain без complete week и отсутствие выдуманной истории.

---

## Содержание

1. [Инструменты](#1-инструменты)
2. [Unit: utils и mappers](#2-unit-utils-и-mappers)
3. [Component tests](#3-component-tests)
4. [MSW](#4-msw)
5. [E2E Playwright](#5-e2e-playwright)
6. [Seed data](#6-seed-data)
7. [Когда запускать](#7-когда-запускать)
8. [Phase 5 Perspectives](#8-phase-5-perspectives)

---

## 1. Инструменты

| Слой | Выбор |
|------|-------|
| Unit / component | **Vitest** + **Testing Library** (`@testing-library/react`) |
| API mock | **MSW** (Mock Service Worker) |
| E2E | **Playwright** в `tests/e2e/` |
| Типы API | из OpenAPI (codegen optional) — mappers тестировать отдельно |

Не смешивать Playwright-спеки с Vitest в `apps/web/src`.

---

## 2. Unit: utils и mappers

Приоритет — чистые функции без DOM.

| Область | Примеры кейсов |
|---------|----------------|
| Salary formatting | null → «—»; 250000 → локаль RU; sample_size < 5 → «мало данных» |
| Period / query params | `from`/`to` → search params; invalid range |
| API client mappers | BFF JSON → view-model; `salary_currency` всегда RUB в MVP |
| Error mapping | `error.code` → user-facing message + `request_id` |
| Cache label | `cache: HIT` → stale/opt UI flag |
| Trends vs Perspectives | разные routes/helpers; не путать названия в i18n keys |

```bash
cd apps/web && npm test
# vitest run
```

---

## 3. Component tests

Только **критичные** виджеты (не каждый презентационный блок).

| Виджет | Что проверить |
|--------|---------------|
| KPI summary cards | loading skeleton → data; empty sample_size=0 |
| Global filters (role/period) | смена роли дергает callback / URL params |
| Error state | retry button; показывает `request_id` |
| Roles table | pagination envelope render |
| Vacancy salary disclaimer | текст offered/net (copy) |
| Vacancy infinite scroll | next-page params, sentinel/fallback, dedup, stop, filter reset, retry |
| Dashboard rankings | независимые toggles count/salary, формат ₽ и sample, без `%` в salary, show-more/end/retry, reset по period |

Responsive smoke: три ranking cards на desktop и один столбец на narrow viewport;
controls доступны по role/name, контент не требует hover. Анимация полос не
обязательна; если добавлена, учитывает `prefers-reduced-motion`.

States из [16](./16-frontend.md): Loading / Empty / Error / Partial — хотя бы по одному тесту на critical screen container.

**Не обязательно в MVP:** визуальные screenshot tests, полные chart canvas asserts (достаточно «series length > 0» / mock chart).

---

## 4. MSW

- Хендлеры под `/api/v1/*` с телами, совместимыми с OpenAPI.
- Фикстуры JSON можно держать в `apps/web/src/test/fixtures/` или читать урезанные копии; **не** тащить сырые HH payloads в UI-тесты.
- Happy + 502 + 400 для summary — минимум.

Пример идеи (не код в проде до реализации):

```ts
// GET /api/v1/dashboard/summary → 200 median_salary, salary_sample_size
// GET /api/v1/roles → pagination envelope
```

MSW — default для Vitest; Playwright E2E бьёт в **реальный BFF** compose (или отдельный fixture-mode стек), не в MSW.

---

## 5. E2E Playwright

### Расположение

```text
tests/e2e/
  README.md
  playwright.config.ts
  specs/
    dashboard.spec.ts
    filter-role.spec.ts
    admin-ingest-fixture.spec.ts
  support/
    seed.ts
    auth.ts          # ADMIN_TOKEN из env test
```

### Critical journeys (MVP)

| # | Journey | Шаги | Assert |
|---|---------|------|--------|
| 1 | Open dashboard | открыть `/` | KPI блоки видимы (не placeholders error) |
| 2 | Filter role | выбрать роль → применить | summary/roles перезапрос; URL или UI отражает role |
| 3 | Trends vacancy | `/trends` | график/серия или empty state осмысленный |
| 4 | Admin ingest fixture | `/admin` или API + inject | run `success`/`partial` без live HH; данные появляются после refresh |

Опционально Phase 1: Vacancies list drill-down.

### Стабильность

- Явные `getByRole` / test ids на KPI root — sparingly.
- Ждать network idle / concrete selector, не fixed `sleep`.
- Retry в CI: 1 (Playwright config); flaky → quarantine, не silent skip.
- **Не** gate на каждый PR — см. [13 §5](./13-testing.md#5-слоистые-gates).

### Окружение

```bash
# ориентир
make up-cloud   # или up-local + migrate + seed
make seed-e2e
cd tests/e2e && npx playwright test
```

Base URL: `http://localhost:3000` (web), API через тот же origin proxy или `http://localhost:8080`.

---

## 6. Seed data

| Источник | Назначение |
|----------|------------|
| `testdata/seeds/*.sql` | роли, регионы, 20–50 synthetic vacancies с salary mid_rub |
| Fixture ingest mode | прогон HH JSON → PG как в integration |
| Не | прод-дамп, реальные работодатели, токены |

Требования к seed:

1. Стабильные UUID/slug (`role_go_dev`, …) для assert в E2E.
2. Хотя бы одна роль с `sample_size ≥ 5` и одна «пустая» для empty state.
3. Outlier-строка **не** должна ломать median (проверка косвенно в KPI).
4. Идемпотентный seed (`INSERT … ON CONFLICT`).

---

## 7. Когда запускать

| Suite | PR | production | Nightly |
|-------|----|------|---------|
| Vitest unit/component | ✅ required | ✅ | ✅ |
| Playwright full journeys | ❌ | опц. 1 smoke | ✅ required job |
| Post-deploy | — | curl smoke; опц. 1 e2e | — |

Бюджет nightly E2E: < 25 мин, включая подъём compose.

---

## 8. Phase 5 Perspectives

| Тест | Слой |
|------|------|
| Disclaimer «эвристика, не прогноз» на `/perspectives` | component + E2E |
| Список направлений + score_version | MSW + contract backend |
| Empty signals | empty state CTA |
| Не регрессировать vacancy `/trends` | отдельные specs; разные test ids/routes |

E2E Perspectives — только когда API + seed `trend_scores_daily` существуют; до Phase 5 не закладывать в required nightly.
