# 22. Documentation & API contract style

Единый стиль человекочитаемых docs, OpenAPI и protobuf для LMA.  
Связано: [03-api.md](./03-api.md), [20-code-style.md](./20-code-style.md), [17-secrets-management.md](./17-secrets-management.md), Cursor rule `.cursor/rules/docs-openapi-style.mdc`.

Не цель этого документа — переписать весь пакет docs; цель — единые правила **вперёд**.

---

## Source of truth

| Артефакт | Канонический путь |
|----------|-------------------|
| Архитектура, фазы, семантика | `docs/architecture/` |
| Public REST (BFF) | [`api/openapi.yaml`](../../api/openapi.yaml) |
| Internal gRPC | [`libs/proto/lma/`](../../libs/proto/lma/) |
| Имена env | [`.env.example`](../../.env.example) |
| Решения | [`adr/`](./adr/) |

Секреты и их хранение — только в [17-secrets-management.md](./17-secrets-management.md); не копируй значения и полные инвентари в другие docs.

---

## Язык

| Слой | Язык |
|------|------|
| Документы для людей (`docs/**`, ADR, runbooks) | **Русский** |
| Идентификаторы кода, API, proto, git messages | **Английский** |
| `summary` / `description` в OpenAPI | Русский OK для людей; имена полей/paths/`operationId` — английский |

Тон: кратко, таблицы и списки вместо эссе. Не дублируй длинные куски между файлами — summary + ссылка.

---

## Нумерация и индекс

- Файлы архитектуры: `NN-name.md` (допускаются суффиксы вроде `13a-…`, `13b-…`).
- Следующий свободный номер — по факту наличия файлов и таблицы в [README.md](./README.md).
- **Новый** файл в `docs/architecture/` → обязательно строка в таблице «Порядок чтения» в README.
- Существенные ops-процедуры → `docs/runbooks/` + ссылка из релевантного architecture doc.

---

## Структура и метки

- Сохраняй метки **MVP** / **Target** и фазы из [00-overview.md](./00-overview.md).
- Кросс-ссылки — относительные (`./12-local-dev.md`, `../runbooks/...`).
- Контракты ссылай на реальные пути (`api/openapi.yaml`, `libs/proto/lma/`), не на устаревшие (`proto/lma/` без `libs/`).
- Код приложения в architecture docs не «реализуй целиком» — описывай контракты, поведение, границы.

---

## Диаграммы

- Предпочтительно **mermaid** в Markdown.
- Альтернативы (ASCII, внешние картинки) — только если mermaid неудобен; тогда краткое описание рядом.

---

## OpenAPI (`api/openapi.yaml`)

| Правило | Деталь |
|---------|--------|
| Версия спеки | OpenAPI **3.1** |
| Paths | `/api/v1/...`; breaking public → `/api/v2` (см. [03-api.md](./03-api.md)) |
| Errors | Единая схема `error.code` / `message` / `details` / `request_id` |
| Target-only | Расширение `x-lifecycle: target` на operation (AI, Perspectives и т.п.) |
| `operationId` | `verbNoun` camelCase: `getHealth`, `listRoles`, `triggerIngest` |
| JSON fields | `snake_case` (как в текущем контракте) |
| Примеры | Без секретов, токенов, реальных PII; placeholder'ы (`example.com`, фиктивные id) |
| Семантика | Yaml = канон формы; [03-api.md](./03-api.md) = семантика и internal mapping |

Header `info.description` держи кратким: что это за контракт, MVP vs Target, указатель на `03-api.md`.

### Публикация OpenAPI (GitHub Pages)

| Правило | Деталь |
|---------|--------|
| Source of truth | Только [`api/openapi.yaml`](../../api/openapi.yaml) — не коммитить копию в site |
| Статика | [`docs/api-site/`](../api-site/): Redoc (`index.html`) + Swagger UI (`swagger.html`) |
| CI | [`.github/workflows/docs-pages.yml`](../../.github/workflows/docs-pages.yml) копирует yaml → `docs/api-site/openapi.yaml` и деплоит artifact |
| CDN | Redoc / Swagger UI с **jsDelivr**; для просмотра нужна сеть (offline не цель) |
| URL | https://chuchoss.github.io/it-labor-pulse/ (Redoc), …/swagger.html (Swagger UI) — см. [03-api.md](./03-api.md), [ADR 008](./adr/008-github-pages-openapi.md) |
| Включение | Repo Settings → Pages → Source = **GitHub Actions** (один раз после появления репо на GitHub) |

---

## Protobuf (`libs/proto/lma/`)

| Правило | Деталь |
|---------|--------|
| Package | `lma.<domain>.v1` (например `lma.query.v1`); breaking → новый `v2` + параллельный deploy |
| Naming | `PascalCase` messages/services; поля — по стилю proto файла репозитория |
| Граница | gRPC только internal (cluster); браузер видит только REST BFF |
| Синхрон с REST | Публичная поверхность и семантика не дрейфуют от OpenAPI / `03-api.md` |

---

## ADR

При смене спорного архитектурного решения:

1. Новый или обновлённый файл в `docs/architecture/adr/`.
2. Формат секций: **Context** → **Decision** → **Consequences** (`(+)` / `(−)`).
3. Строка в [`adr/README.md`](./adr/README.md).
4. Ссылка из затронутого architecture doc при необходимости.

Шаблон также в skill `lma-architecture` / `reference.md`.

---

## Чеклист при изменении API

```
- [ ] api/openapi.yaml (и/или libs/proto/lma/)
- [ ] docs/architecture/03-api.md
- [ ] docs/architecture/16-frontend.md — если затронуты экраны / IA
- [ ] docs/architecture/13*.md — если меняются контрактные тесты / DoD
- [ ] ADR — если меняется стратегия версий, auth, границы BFF и т.п.
- [ ] Handler/BFF DTO не расходятся с yaml/proto «молча»
```

---

## Changelog

- Обязательно: краткая заметка в описании PR (что и зачем в контракте/docs).
- Отдельный корневой `CHANGELOG.md` — опционально позже; не блокирует текущую работу.

---

## Чего не делать

- Не коммитить секреты, kubeconfig, токены в docs/examples.
- Не плодить параллельные «истины» (второй OpenAPI, корневой `proto/` без `libs/`; не коммитить `docs/api-site/openapi.yaml`).
- Не смешивать MVP и Target без явной метки.
- Не раздувать один doc копипастой из соседних — ссылайся.
