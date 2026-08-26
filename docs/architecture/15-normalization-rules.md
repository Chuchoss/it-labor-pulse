# 15. Normalization Rules

Канонические правила преобразования сырых вакансий (сначала HH) в модель из [05-data-model.md](./05-data-model.md).  
Используются normalizer'ом и unit-тестами ([13-testing.md](./13-testing.md)).

**Пакет (Phase 1):** чистая реализация shared rules — [`libs/go-common/normalize`](../../libs/go-common/normalize) (`Draft` / `CanonicalVacancy` / `Normalize` / injectable FX и alias maps). Без сети и без записи в PG; оркестрация HH → normalize → UPSERT — [`apps/ingest`](../../apps/ingest) (`make ingest-hh` / `make ingest-hh-fixture`). Тесты: `go test ./libs/go-common/normalize/...` · `go test ./apps/ingest/...` (+ фикстуры [`testdata/hh`](../../testdata/hh)).

**Продуктовый принцип:** на дашборде по умолчанию показываем **предлагаемые зарплаты из вакансий (offered)**, не survey-бенчмарки. Смешение — только с явной пометкой серии.

---

## Pipeline (кратко)

```text
source payload → adapter: SourceNeutralDraftV1 → normalizer: validate + shared map
→ salary normalize → role match → remote detect → content_hash → UPSERT PG → (Phase 2) CH snapshot
```

Adapter сохраняет source facts в versioned draft и не выполняет shared rules. Normalizer — единственное место для правил этого документа. Идемпотентность: unique `(source, external_id)`; повтор с тем же `content_hash` → noop.

---

## Offered vs survey (явно)

| Тип данных | Источник | Где храним | На графике |
|------------|----------|------------|------------|
| **Offered** | Поля salary вакансии | `vacancies.salary_*`, CH `salary_mid_rub` | Default series: «Зарплаты в вакансиях» |
| **Survey** | Внешние отчёты / CSV | `salary_benchmarks` (Target, см. [08](./08-integrations-and-extensibility.md)) | Отдельная серия: «Опросы / бенчмарки» |

Правила:

1. Медианы dashboard/trends считаются **только по offered**, пока UI явно не запросит benchmark overlay.
2. API не смешивает типы в одном поле `median_salary` без дискриминатора.
3. В UI — дисклеймер: «на основе зарплат, указанных в вакансиях» ([16-frontend.md](./16-frontend.md)).

---

## Salary: from / to / mid

| `salary_from` | `salary_to` | `salary_mid` (в исходной валюте, до FX) |
|---------------|-------------|----------------------------------------|
| set | set | `(from + to) / 2` |
| set | null | `from` |
| null | set | `to` |
| null | null | `null` — вакансия **исключается** из salary sample, но учитывается в demand/count |

Дополнительно:

- Отрицательные / нулевые значения → трактовать как invalid → `salary_* = null` + metric `normalize_salary_invalid`.
- Если `from > to` → swap с логом warn (или null — выбрать swap; **решение: swap**).

Поле PG: `salary_mid` — mid в **исходной** валюте после gross/net политики (см. ниже), до отчётной конвертации.  
Отчётный mid: `salary_mid_rub` (CH / computed) после FX.

---

## Gross / net policy

HH: `salary.gross` = true означает «до вычета налогов» (gross).

| `salary_gross` | Политика MVP |
|----------------|--------------|
| `true` | Приводим к **net** для сопоставимости: `mid_net = mid_gross * (1 - TAX_RATE)` |
| `false` / null | Считаем уже net; `TAX_RATE` не применяем. `null` → как net + metric `gross_unknown` |
| нет salary | всё null |

Параметры (env / config):

| Param | Default MVP | Комментарий |
|-------|-------------|-------------|
| `SALARY_TAX_RATE` | `0.13` | Упрощение для RU employee; **не** бухгалтерия |
| `SALARY_NORMALIZE_TO` | `net` | Фиксируем net как канон аналитики |

Документировать в UI: «зарплаты приведены к оценке net (упрощённо)».

Target: разные ставки / самозанятость — только после явной модели employment type.

---

## Currency → RUB (FX)

1. Нормализуем код валюты источника к ISO 4217 перед хранением: HH `RUR` → `RUB`; затем храним `salary_currency` как `RUB`, `USD`, `EUR`, ….
2. Считаем `salary_mid_rub = salary_mid * fx(currency→RUB, rate_date)`.
3. `rate_date` = UTC date от `published_at` (fallback `collected_at`, только если
   источник не дал публикацию). Берётся ближайшая предыдущая дата, но не старше
   7 календарных дней.
4. Production provider — официальный дневной XML Банка России
   [`XML_daily.asp`](https://www.cbr.ru/development/SXML/); PostgreSQL хранит
   `rub_per_unit = Value / Nominal`, дату ответа и provenance. Это не realtime.
5. Нет курса в bounded окне → raw salary/currency сохраняются, canonical RUB/net
   остаётся null и запись исключается из salary aggregates. Static fallback
   разрешён только явным deterministic fixture/unit-тестам.
6. Текущие вакансии/рейтинги конвертируются из canonical RUB по последнему
   доступному курсу. Исторические points используют дату point, а не сегодняшний
   курс. UI поддерживает RUB/USD/EUR/CNY и показывает дату курса.

Решение и ограничения: [ADR 014](./adr/014-official-fx-and-source-links.md).

---

## Outliers

Цель: не ломать медиану опечатками и «1 рубль».

| Условие (после net + FX в RUB) | Действие |
|--------------------------------|----------|
| `salary_mid_rub < 10_000` | `exclude_from_salary_agg = true` (или обнулить mid для agg) |
| `salary_mid_rub > 2_000_000` | то же |
| sample_size < 5 для разреза | API может вернуть median, но UI показывает «мало данных» |

Пороги — константы конфига (`SALARY_OUTLIER_MIN_RUB`, `SALARY_OUTLIER_MAX_RUB`).  
Demand counts **не** зависят от outlier-фильтра.

---

## Role matching

### Продуктовый scope HH (Phase 1)

До общего role matching HH-вакансия проходит строгую проверку
`professional_roles[]` по allowlist из [08](./08-integrations-and-extensibility.md).
Хотя бы один официальный ID должен быть разрешён; иначе запись не активируется и
не попадает в публичную выдачу. Пустой/неразрешённый mapping не уточняется по
title. Для multi-role выбирается разрешённая роль по стабильному приоритету.
Счётчик `excluded_out_of_scope` агрегируется без title/employer/описания.

Порядок:

1. HH `professional_roles[]` → lookup `role_aliases` (`source=hh`, pattern = external role id).
2. Иначе нормализованный `title` (lowercase, ё→е, trim punctuation) → pattern / contains rules в `role_aliases`.
3. Иначе `role_id = null`; для HH такая запись исключена из активного
   продуктового списка до классификации.

Правила:

- Один primary `role_id` на вакансию (первый matched по priority aliases).
- Не угадывать ML в MVP.
- Метрика: `normalize_role_unmapped_total`.

Slug ролей — канон в PG (`go-developer`), в API часто удобные id вида `role_go_dev` = тот же slug/id из БД (не плодить два словаря).

---

## Skills

1. HH `key_skills[].name` → normalize → `skill_aliases` → `skills.id`.
2. Неизвестный скилл: создать `skills` + alias **или** (строже) отложить в `unmapped_skills` — **решение MVP:** upsert skill by slug (автосоздание) с `is_active=true`, чтобы top-skills работал.
3. Дедуп скиллов в одной вакансии по `skill_id`.

### Taxonomy языков (v7)

- canonical aliases: `Go/Golang`, `JS/JavaScript/ECMAScript`,
  `TS/TypeScript`, `C Sharp/C#/CSharp`, `C++/CPP` и остальные seed aliases
  из migration v7;
- ranking «Языки программирования» включает только
  `skills.skill_kind=programming_language`;
- SQL/PL-SQL/T-SQL — `query_language`; HTML/CSS — `markup`; Bash/Shell —
  `shell`; 1C — `platform_language`. Они остаются навыками, но не смешиваются
  со строгим рейтингом языков;
- framework/database/tool не становятся языком по совпадению строки;
- alias lookup выполняется до upsert, поэтому одна vacancy × canonical
  language даёт одну строку `vacancy_skills`.

---

## Region

1. HH `area.id` → `region_external_ids(source, external_id)`.
2. Miss → создать/линковать через словарь areas (кэш Redis `dict:hh:areas`) + seed mapping для Москвы/СПб/remote.
3. Remote-only площадки могут иметь `region_id` = специальный `ru-remote` или null + `is_remote=true`.

---

## Remote detection

`is_remote` (поле Target в модели / флаг в JSONB attrs; MVP можно колонку позже):

| Сигнал | Вес |
|--------|-----|
| HH schedule / work_format id «удалённо» | сильный → true |
| `area` = «Удаленно» / remote | сильный |
| title/description содержит `\bremote\b`, «удаленн» | слабый (нужен ещё один сигнал) |

Иначе `is_remote=false`.  
Гибрид: `is_remote=true` + region офиса сохраняем оба.

---

## Employer

Upsert `employers` by `(source, external_id)`; имя — из raw (fake в фикстурах).  
Не хранить контакты HR из description.

---

## Activity / soft-delete

| Событие | Поля |
|---------|------|
| Вакансия пришла в ingest | `is_active=true`, `deleted_at=null`, обновить `collected_at` |
| HH-вакансия вне обоих утверждённых scopes | не upsert/reactivate |
| Management-only role подтверждена bounded ingest | `is_active=true`, только `management_analytics`; не входит в `/vacancies` |
| Нет в источнике N дней / full sync miss | `is_active=false` |
| Ops hide | `deleted_at=now()` |

Аналитика «текущий рынок»: `is_active AND deleted_at IS NULL`.

---

## Content hash

`content_hash = sha256` от канонического subset: title, salary_*, area, employer_id, skills set, published_at.  
Не включать `collected_at`.  
Skip upsert body если hash совпал (touch `collected_at` опционально).

---

## Чеклист реализации

- [x] Table-driven tests на mid / gross / FX / outliers (`libs/go-common/normalize`)  
- [x] Фикстура HH detail с salary и без (`testdata/hh`)  
- [x] Метрики unmapped role / invalid salary / fx miss (флаги `Metrics` в результате)  
- [ ] UI copy: offered, net estimate, attribution HH  
- [ ] Не смешивать survey в `median_salary`  
