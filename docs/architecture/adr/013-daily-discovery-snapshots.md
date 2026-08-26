# ADR 013: Дневной discovery отдельно от detail hydration

## Context

Полный detail crawl всех IT-вакансий занимает около 11 тысяч запросов. При
локальном безопасном темпе один detail-запрос на 30 минут его оценка — 58,5
часа, поэтому ежедневные запуски перекрываются и не доказывают дневное
покрытие.

Официальный HH `GET /vacancies` уже возвращает `id`, `published_at`, `area`,
`professional_roles` и offered `salary`. `key_skills` и полное описание
доступны только в detail.

## Decision

- Ввести отдельный ежедневный `discovery`: все bounded role/date partitions и
  search pages до фиксированного UTC cutoff.
- День снимка — предыдущий UTC-день; cutoff — его правая граница `00:00 UTC`.
  Default старт — `01:00 UTC` (`04:00 Europe/Moscow`).
- `ingest_cycle_observations` хранит только минимальные агрегатные входы.
  Natural key — `(cycle_id, source, external_id)`, поэтому пересечения ролей и
  partitions не дают двойного счёта.
- Роль и регион берутся только из конкретного search item. Primary role
  выбирается детерминированным приоритетом taxonomy policy.
- Cycle становится `complete` только после commit всех запланированных search
  pages. Только такой marker запускает `vacancy_demand_v2`.
- `active_count` — число дедуплицированных observations; `published_count` —
  observations с `published_at` в UTC-дне snapshot.
- Salary использует те же поля и shared normalizer, что detail path. Snapshot
  сохраняет `salary_method=hh_search_shared_normalizer_v1` и coverage.
- Detail hydration остаётся отдельным bounded scheduler для skills,
  descriptions и vacancy listing. Она не блокирует demand snapshot.
- При ошибке checkpoint остаётся `running`, и процесс возобновляет старейший
  незаконченный день. Новый день параллельно не стартует. После восстановления
  выполняется текущий due day; полностью пропущенные дни не фабрикуются.
- Observations сохраняются минимум 35 дней. Очистка допустима только для cycle
  с успешным snapshot; snapshots и provenance сохраняются.

## Consequences

- (+) Около 117 search pages плюс taxonomy/probes завершаются значительно
  быстрее суток при default `350 ms` delay и hard ceiling `300` attempts.
- (+) Неполные и failed cycles видны в coverage, но не публикуются.
- (+) Skill analytics честно отражает только hydrated vacancies.
- (+) Scheduler locks разделены: discovery имеет приоритет и не конфликтует с
  hydration lock.
- (−) При выключенном ПК нет фонового выполнения; дни простоя остаются
  `missed`, если внешний always-on runner не настроен.
- (−) Изменение HH search schema или salary semantics требует новой method
  version и контрактных тестов.

Это Phase 1 vacancy demand, не Phase 5 multi-source «Тенденции».
