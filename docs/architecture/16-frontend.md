# 16. Frontend IA (MVP + Target Perspectives)

React SPA (`web`) — дашборд аналитики IT-рынка труда. Публичный API только через **BFF** (`/api/v1` на `:8080`), контракт: [03-api.md](./03-api.md), [`api/openapi.yaml`](../../api/openapi.yaml), [ADR 010](./adr/010-api-gateway.md).

---

## MVP screens (4–6)

| # | Экран | Route (ориентир) | Job to be done | Фаза |
|---|-------|------------------|----------------|------|
| 1 | **Dashboard** | `/` | Сводка рынка за период | MVP |
| 2 | **Roles** | `/roles` | Спрос и зарплаты по ролям | MVP |
| 3 | **Role detail** | `/roles/:roleId` | Одна роль: тренд + skills | MVP |
| 4 | **Regions** | `/regions` | Срез по регионам | MVP |
| 5 | **Trends** | `/trends` | Salary + demand time series **только по вакансиям** | MVP |
| 6 | **Vacancies** | `/vacancies` | Drill-down в OLTP-список | MVP |
| 7 | **«Тенденции» (Perspectives)** | `/perspectives` | Какие IT-направления выглядят перспективнее (composite heuristic) | **Phase 5 Target** |

Опционально позже: Admin ingest (`/admin/ingest`) — можно начать с curl/`make ingest-hh`.

**Не путать:** MVP **Trends** (`/trends`) ≠ Target **«Тенденции»** (`/perspectives`). Первый — salary/demand из job pipeline; второй — multi-source Perspectives ([ADR 007](./adr/007-multi-source-trend-signals.md)).

Фильтры глобальные (header/sidebar): `from`, `to`, опционально `role_id`, `region_id`, `source`. На Perspectives дополнительно: `role_family` / direction.

---

## States (на каждом экране данных)

| State | UI | Когда |
|-------|----|-------|
| **Loading** | Skeleton / spinner секции, не весь blank page | Первый fetch / смена фильтров |
| **Empty** | Короткий текст + CTA «измените период» / «запустите ingest» | 200 с пустым `data` / sample_size=0 |
| **Error** | Сообщение + retry; показать `request_id` если есть | 4xx/5xx / network |
| **Partial** | График ок, блок skills — error boundary | Один из параллельных запросов упал |
| **Stale** (opt) | Метка «из кэша» если `cache: HIT` | После summary |

Не блокировать весь layout на одном медленном запросе: summary и trends грузить независимо.

---

## Экраны → endpoints

### 1. Dashboard `/`

| Блок | Endpoint |
|------|----------|
| KPI (active, new, median) | `GET /api/v1/dashboard/summary` |
| Top roles (mini) | из `summary.top_roles` или `GET /api/v1/roles?page_size=5` |
| Top regions (mini) | из `summary.top_regions` |

### 2. Roles `/roles`

| Блок | Endpoint |
|------|----------|
| Таблица/список | `GET /api/v1/roles` (`sort`, pagination, filters) |

### 3. Role detail `/roles/:roleId`

| Блок | Endpoint |
|------|----------|
| Заголовок + KPI | `GET /api/v1/roles/{role_id}` |
| Salary trend | `GET /api/v1/trends/salaries?role_id=...` |
| Demand trend | `GET /api/v1/trends/demand?role_id=...` |
| Top skills | `GET /api/v1/skills/top?role_id=...` |
| Sample vacancies | `GET /api/v1/vacancies?role_id=...&page_size=10` |

### 4. Regions `/regions`

| Блок | Endpoint |
|------|----------|
| Список | `GET /api/v1/regions` |
| Detail (можно тот же page + drawer) | `GET /api/v1/regions/{region_id}` |

### 5. Trends `/trends`

| Блок | Endpoint |
|------|----------|
| Salary series | `GET /api/v1/trends/salaries` (`grain`) |
| Demand series | `GET /api/v1/trends/demand` |

Две серии рядом; не смешивать с survey benchmarks без отдельного toggle (см. [15](./15-normalization-rules.md)).

### 6. Vacancies `/vacancies`

| Блок | Endpoint |
|------|----------|
| Список + поиск | `GET /api/v1/vacancies` (`q`, filters, page) |

Фильтры экрана: поиск, несколько регионов, несколько канонических ролей,
зарплата от/до, несколько навыков (ANY), source и active. Справочники
загружаются ограниченно из `/regions`, `/roles`, `/skills/top?limit=100`; сбой
одного справочника не скрывает уже полученный список. Состояние хранится в URL
(`role_id`, `region_id`, `skill_id` — CSV), текстовый поиск debounce 400 ms.
Смена/сброс фильтра сбрасывает infinite pages; UI показывает число активных
фильтров и действие «Сбросить все».

Список использует infinite scroll по `page` / `page_size`, оставляя доступную
кнопку «Загрузить ещё» как fallback. Страницы объединяются с dedup по vacancy
ID и загружаются только до `total`; смена фильтра начинает выдачу заново.
Батчи небольшие и карточки/строки остаются лёгкими; виртуализация нужна только
после измеренного DOM/performance bottleneck. Нет тяжёлой аналитики на этом
экране — только drill-down.

**Phase 1 live update:** отдельный лёгкий HTTP polling запрашивает первую
страницу (`page_size=100`) в каноническом порядке `published_at DESC, id ASC`
с теми же фильтрами. Начальный ответ задаёт baseline; следующие ответы
добавляют только ранее не встречавшиеся ID, обновляют `total` и не
перезапрашивают все infinite pages. Новые строки на 8 секунд получают зелёную
подсветку и `aria-live`-объявление; анимация отключается при
`prefers-reduced-motion`. Ошибка polling не скрывает текущий список.

Интервал задаётся публичной переменной `VITE_VACANCIES_POLL_INTERVAL_MS`
(default `30000`, минимум `10000`, `0` отключает polling). Скрытая/offline
вкладка не опрашивается; после focus/reconnect выполняется повторная проверка.
Polling только обнаруживает данные после записи ingest в PostgreSQL и **не**
запускает HH ingest: его расписание — отдельная задача.

### 7. «Тенденции» `/perspectives` (Phase 5 Target)

| Блок | Endpoint |
|------|----------|
| Рейтинг направлений | `GET /api/v1/trends/perspectives` |
| Ряд / разложение score | `GET /api/v1/trends/perspectives/{direction_key}` |

**UI:**

- Фильтры: период (`from`/`to`), `role_family` (опц.), сортировка по `composite_score`
- Charts: composite series + stacked/legend по ногам demand / learning / media
- Показ `coverage` (какие источники участвовали) и `score_version`
- **Disclaimer (обязателен, рядом с графиком и в footer секции):**

> «Тенденции» — составная эвристика по открытым сигналам (вакансии, обучение, медиа), а не прогноз и не карьерный совет. Методика и веса могут меняться (`score_version`).

Nav: пункт «Тенденции» не добавлять в MVP shell до Phase 5 (или feature flag `FEATURE_PERSPECTIVES=false`).

---

## Attribution & salary disclaimer

Показывать в footer **и** рядом с salary-графиками (коротко):

> Данные вакансий: HeadHunter и другие указанные источники. Медианные зарплаты — оценка по полям salary в вакансиях (offered), приведены к net упрощённо; это не опросы и не офер кандидату. Соблюдайте ToS источников.

Ссылка на README / полный дисклеймер.  
Не хардкодить чужие логотипы без разрешения; текстовая attribution достаточна для учебного проекта.

---

## UX notes (практично)

- Период по умолчанию: последние 30 дней.
- `sample_size < 5` → warning «мало данных», median можно скрыть/засерить.
- Валюта default RUB.
- Admin actions не в основном nav MVP.
- Mobile: один столбец; графики — упрощённые spark/line.

---

## Техзаметки

- Base URL: `VITE_API_BASE_URL` → **BFF** (default `/api/v1`; local Vite proxy → `http://localhost:8080`, см. `.env.example`).
- Vacancy polling: `VITE_VACANCIES_POLL_INTERVAL_MS` (default 30 секунд; `0` отключает).
- `request_id` из error body — в UI error state.
- OpenAPI → опциональная генерация типов (`openapi-typescript`) в Phase 1.
- Auth MVP отсутствует; admin token только если появится admin UI.
