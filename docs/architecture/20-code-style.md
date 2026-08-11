# 20. Code Style (единый гайд)

Единый стиль для IT Labor Market Analytics (LMA). Инструменты: [`.editorconfig`](../../.editorconfig), [`.golangci.yml`](../../.golangci.yml).  
Связано: [19-agent-tooling.md](./19-agent-tooling.md), [13-testing.md](./13-testing.md), [22-documentation-style.md](./22-documentation-style.md) (docs / OpenAPI / proto), ADR [002](./adr/002-migrate-tool.md).

Документация для людей — **русский**; идентификаторы в коде и API — **английский**. Подробные правила оформления docs и контрактов — в [22](./22-documentation-style.md).

---

## Общие принципы

1. Читаемость > микрооптимизации; не раздувай абстракции раньше Phase.
2. Следуй существующему layout docs (`cmd`/`internal`, `web`, `api`, `libs/proto`).
3. Не коммить секреты; имена env — из `.env.example`.
4. Один PR/изменение — одна тема; conventional commits (ниже).

---

## Go

| Тема | Правило |
|------|---------|
| Format | `gofmt`; импорты — `goimports` |
| Lint | `golangci-lint run ./...` (конфиг в корне) |
| Layout | `cmd/<svc>`, `internal/...`, общее в `libs/go-common` |
| Errors | wrap с `%w`; не `panic` в libs |
| Context | первый аргумент I/O и RPC |
| Logs | `log/slog` JSON, поля из [18](./18-logging-and-incidents.md) |
| DB schema | только **golang-migrate** SQL, не ad-hoc DDL в prod path |
| Tests | table-driven; HH без сети (`testdata/hh/`) |

Имена: `MixedCaps` экспортируемое; пакеты короткие, без underscores (`normalize`, не `salary_utils`).

---

## TypeScript / React (`web`)

| Тема | Правило |
|------|---------|
| Language | TypeScript **strict** |
| API | только BFF `/api/v1`; контракт `api/openapi.yaml` |
| Secrets | не в клиенте; только public `VITE_*` |
| Screens | IA из [16-frontend.md](./16-frontend.md) |
| Components | функциональные; states loading/empty/error/partial |
| Format | когда появится пакет: Prettier + ESLint (indent 2 из EditorConfig) |

Имена: компоненты `PascalCase`, хуки `useCamelCase`, файлы экранов рядом с route.

Пока нет `package.json` — не добавляй случайный набор линтеров «на будущее» без запроса; этот doc — канон.

---

## SQL migrations

- PG: `migrations/postgres/` (или путь из docs), tool **golang-migrate** (ADR 002).
- Имена: `NNNNNN_description.up.sql` / `.down.sql`.
- Идемпотентность down где разумно; без destructive data loss без явного ок.
- ClickHouse — отдельный каталог runner (не смешивать с PG binary обязательно).
- Параметризованные запросы в коде; схема — reviewable SQL diffs.

---

## Protobuf / OpenAPI

| Артефакт | Путь | Naming |
|----------|------|--------|
| REST | `api/openapi.yaml` | `/api/v1/...`, `snake_case` JSON fields (как в контракте) |
| gRPC | `libs/proto/lma/` | `package lma.<domain>.v1;`, `PascalCase` messages/services |

- Source of truth — файлы контракта; код генерённый не править руками без нужды.
- Не дрейфовать путями (`libs/proto`, не корневой `proto/`).
- Полный стиль спеки, `operationId`, `x-lifecycle`, чеклист смены API — [22-documentation-style.md](./22-documentation-style.md).

---

## Conventional commits

```text
<type>(<scope>): <short English summary>

[optional body]
```

| type | Когда |
|------|--------|
| `feat` | новая функциональность |
| `fix` | багфикс |
| `docs` | только документация |
| `chore` | tooling, CI, зависимости |
| `refactor` | без смены поведения |
| `test` | тесты |
| `perf` | производительность |

Scopes (примеры): `bff`, `ingest`, `hh`, `normalize`, `query`, `web`, `compose`, `arch`.

Примеры:

```text
docs(arch): add agent tooling and code style guides
feat(ingest): add HH search pagination with 429 backoff
fix(normalize): swap salary from/to when inverted
```

---

## EditorConfig (кратко)

- Default: UTF-8, LF, spaces 2, final newline.
- Go: tabs.
- Makefile: tabs.
- Markdown: не резать trailing spaces жестко (совместимость с некоторыми MD).

Полный файл: [`.editorconfig`](../../.editorconfig).
