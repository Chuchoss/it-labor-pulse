# ADR 012. Отдельные scopes и taxonomy для dashboard rankings

## Context

Dashboard должен ранжировать языки программирования и управленческие IT-роли.
Существующий публичный список вакансий намеренно ограничен development/leads,
analytics и QA. Навык из HH не обязательно является языком, а одна вакансия
может иметь несколько официальных ролей и несколько alias одного языка.

## Decision

1. `vacancy_listing` и `management_analytics` — независимые many-to-many scopes
   в `role_scopes` / `vacancy_role_scopes`. Публичный `/vacancies` использует
   только `vacancy_listing`.
2. Management scope строится только по professional role ID из актуального
   официального каталога HH категории IT: `10`, `36`, `73`, `104`, `107`,
   `125`, `148`, `150`, `156`, `157`, `164`.
3. `skills.skill_kind` и `skill_aliases` задают data-driven taxonomy. Строгий
   ranking включает только `programming_language`; SQL (`query_language`),
   HTML/CSS (`markup`), Bash (`shell`) и 1C (`platform_language`) не входят.
4. Ranking — текущий срез активных вакансий, не исторический snapshot.
   Salary metric — медиана offered salary в RUB/net; группы с выборкой `<5`
   исключаются.

## Consequences

- (+) Расширение analytics collection не расширяет пользовательский listing.
- (+) Alias одного языка дедуплицируются до связи `(vacancy_id, skill_id)`.
- (+) Scope и taxonomy проверяются SQL constraints и тестами, а не title filters.
- (−) Текущий PG-запрос подходит Phase 1; при больших объёмах потребуется
  перенос ranking facts в ClickHouse без изменения публичной семантики.
- (−) 1C и shell/query/markup видны в общем top-skills, но не в строгом
  programming-language ranking.
