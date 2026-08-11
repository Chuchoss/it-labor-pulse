# HH API fixtures

Анонимизированные минимальные JSON для тестов адаптера HeadHunter (без сети).

Структура приближена к публичному API HH (`/vacancies` search и vacancy detail), но:

- названия компаний и тексты — **вымышленные**;
- id — синтетические;
- нет телефонов, email, ФИО контактов.

## Файлы

|Файл|Аналог ответа HH|Назначение|
|---|---|---|
|[`vacancy_search_page.json`](./vacancy_search_page.json)|`GET /vacancies?...` (page)|пагинация, список items|
|[`vacancy_detail.json`](./vacancy_detail.json)|`GET /vacancies/{id}`|salary, employer, skills, area, description|
|[`salary_absent.json`](./salary_absent.json)|vacancy|отсутствующая salary|
|[`salary_invalid_outlier.json`](./salary_invalid_outlier.json)|vacancy|invalid `from=0` и outlier `to=5_000_000`|
|[`salary_fx_miss.json`](./salary_fx_miss.json)|vacancy|USD без доступного FX rate|
|[`salary_rur_to_rub.json`](./salary_rur_to_rub.json)|vacancy|legacy HH `RUR` → ISO `RUB`|

## Golden expectations

Фикстуры проверяют две границы: adapter отдаёт `SourceNeutralDraftV1` без shared normalization, normalizer применяет правила из [15-normalization-rules.md](../../docs/architecture/15-normalization-rules.md).

|Fixture|Expected draft / normalization result|
|---|---|
|`salary_absent.json`|draft содержит `external_id=900010`, salary fields `null`; вакансия учитывается в demand, но не в salary sample|
|`salary_invalid_outlier.json`|adapter передаёт source values; normalizer делает invalid `from=0` null и исключает оставшийся outlier из salary aggregates; demand сохраняется|
|`salary_fx_miss.json`|draft сохраняет raw `USD`; при отсутствии rate за `collected_at` и fallback normalizer оставляет `salary_mid_rub=null`, увеличивает FX-miss metric и исключает из salary aggregate|
|`salary_rur_to_rub.json`|draft содержит raw `RUR`; normalizer сохраняет `salary_currency=RUB`, `salary_mid=150000`, `salary_mid_rub=150000`|

Эти значения — contract/golden expectations для будущих unit-тестов; fixtures не являются данными HH и не требуют сети.

## Использование

```go
raw, _ := os.ReadFile("testdata/hh/vacancy_detail.json")
draft, err := hhadapter.ParseDetail(raw)
```

См. [docs/architecture/13-testing.md](../../docs/architecture/13-testing.md), [13a-testing-backend.md](../../docs/architecture/13a-testing-backend.md) и [15-normalization-rules.md](../../docs/architecture/15-normalization-rules.md).

## Обновление

При изменении полей, на которые опирается адаптер:

1. Обновить фикстуру минимально (не копировать огромные prod payload).
2. Обновить golden/assert в тестах.
3. Не добавлять PII.
