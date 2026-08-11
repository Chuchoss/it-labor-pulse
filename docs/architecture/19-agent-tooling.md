# 19. Agent tooling (Cursor rules / skills / hooks)

Как агенты и люди работают с репозиторием IT Labor Market Analytics (LMA) через Cursor.

Связано: [20-code-style.md](./20-code-style.md), [17-secrets-management.md](./17-secrets-management.md), корневой [README](../../README.md).

---

## Зачем

Пет-проект, но с production-like дисциплиной: модели не должны выдумывать scope, коммитить секреты, скрапить LinkedIn/Avito или прыгать через фазы архитектуры.

---

## Rules (`.cursor/rules/`)

| Файл | Когда | Что жёстко фиксирует |
|------|-------|----------------------|
| `lma-project.mdc` | всегда | фазы, секреты, HH-first, adapters, язык docs/code, запрет scraping |
| `go.mdc` | `*.go`, `go.mod` | layout, errors/context, slog, migrate, no panic in libs |
| `frontend-react.mdc` | `web` / `frontend` / `*.ts(x)` | BFF only, экраны из 16, no client secrets, TS strict |
| `proto-api.mdc` | `api/**`, `libs/proto/**` | канонические пути контрактов, no drift |
| `docs.mdc` | `docs/**` | русский, индекс README, кросс-ссылки, ADR |
| `tests-testdata.mdc` | testdata / `*_test.go` | пирамида, fixtures без сети/PII |

Правила намеренно короткие: hard constraints, не бюрократия.

---

## Skills (`.cursor/skills/`)

| Skill | Когда применять |
|-------|-----------------|
| `lma-architecture` | правки архитектуры/ADR/индекса |
| `lma-ingest-hh` | HH adapter, ingest, rate limit, fixtures |
| `lma-dev` | локальный Phase 0–1, Compose, migrate, smoke |

Skills с progressive disclosure: `SKILL.md` + `reference.md`.

В git коммитим только проектные `lma-*`; сторонние skill dumps (`.agents/`, `.claude/`, `.cursor/skills/supabase*` и прочие vendor installs) — в `.gitignore`, на диск можно оставлять локально.

---

## Hooks (`.cursor/hooks.json`)

Минимальный полезный набор — **без** шумных hook на каждый keystroke / каждый edit.

| Event | Script | Поведение |
|-------|--------|-----------|
| `beforeShellExecution` (matcher `git commit\|add`) | `hooks/block-secret-commit.js` | deny, если в add/staged есть `.env`, ключи, kubeconfig, `secrets/` и т.п. |

Почему так:

- Блокировка секретов на commit/add даёт максимум ценности при минимуме раздражения.
- Format-on-every-edit и stop-scan всего дерева для соло-пет-проекта — overkill (есть EditorConfig + golangci при появлении кода).
- `failClosed: true` — при падении скрипта команда не проходит «тихо».

Требование: в PATH доступен `node` (проверено для Windows-разработки).

Проверка: Cursor → Hooks settings / Hooks output channel после `git add .env` (должен deny).

---

## Code style tooling

| Файл | Назначение |
|------|------------|
| `.editorconfig` | отступы/charset для всех редакторов |
| `.golangci.yml` | линтеры Go + gofmt/goimports |
| [20-code-style.md](./20-code-style.md) | Go + TS/React + SQL + proto + commits |

ESLint/Prettier для frontend — когда появится `web/package.json`, не раньше без запроса.

---

## Что агенту нельзя (напоминание)

- Реализовывать «всё приложение целиком» без запроса фазы.
- Коммитить секреты или править git config.
- Противоречить ADR / 17 secrets / 18 logging.
- Скрапить запрещённые площадки.
