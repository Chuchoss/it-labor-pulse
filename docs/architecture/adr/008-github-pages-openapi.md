# ADR 008: GitHub Pages для публикации OpenAPI

## Context

Нужна публичная HTML-документация REST контракта (`api/openapi.yaml`) без отдельного хостинга и без дублирования спеки как второго source of truth. Репозиторий может ещё не быть на GitHub; workflow должен быть готов к первому push.

## Decision

1. Статический сайт в [`docs/api-site/`](../../../docs/api-site/): **Redoc** (`index.html`) как landing, **Swagger UI** (`swagger.html`) для Try it.
2. Канон спеки остаётся [`api/openapi.yaml`](../../../api/openapi.yaml). CI копирует файл в `docs/api-site/openapi.yaml` только на build Pages; копию не коммитить.
3. Публикация — **GitHub Pages** через workflow [`.github/workflows/docs-pages.yml`](../../../.github/workflows/docs-pages.yml) (`upload-pages-artifact` + `deploy-pages`), triggers: push `main` (+ path filter) и `workflow_dispatch`.
4. Redoc / Swagger UI грузятся с **jsDelivr CDN** (нужен сеть; offline-preview без CDN не цель).

## Consequences

- (+) Простой read-only сайт + Try it без своего backend  
- (+) Один OpenAPI source of truth  
- (−) Нужно один раз включить Pages: Source = GitHub Actions  
- (−) CDN dependency; локальный просмотр HTML без сети неполноценен  
