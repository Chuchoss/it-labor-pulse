---
name: lma-architecture
description: >-
  Ведёт изменения архитектурной документации LMA / IT Labor Pulse: куда писать,
  как обновлять индекс и ADR, как не противоречить фазам и контрактам. Use when
  editing docs/architecture, ADR, OpenAPI/proto docs, runbooks, or when the user
  asks about architecture, phases, services, or system design.
---

# LMA Architecture

## Когда читать этот skill

Любое изменение `docs/architecture/**`, ADR, контрактных stubs с влиянием на docs, runbooks.

## Стиль (обязательно)

Перед правкой docs / OpenAPI / proto следуй **[22-documentation-style.md](../../../docs/architecture/22-documentation-style.md)** и rule `.cursor/rules/docs-openapi-style.mdc`:

- язык (RU docs / EN identifiers), нумерация `NN-*.md`, индекс README;
- OpenAPI 3.1, `/api/v1`, error schema, `x-lifecycle`, `operationId`;
- proto `lma.*.v1` без дрейфа от публичного REST;
- при смене API — yaml/proto + `03-api.md` (+ `16` / `13*` при необходимости);
- диаграммы mermaid; секреты не дублировать (→ 17).

## Чеклист изменения

```
- [ ] Соблюдён стиль из 22-documentation-style.md
- [ ] Найден канонический doc (не дублировать новый файл без нужды)
- [ ] Фазы / MVP vs Target согласованы с 00-overview.md
- [ ] Контрактные пути: api/openapi.yaml, libs/proto/lma/
- [ ] Кросс-ссылки на соседние docs обновлены
- [ ] docs/architecture/README.md индекс обновлён (новый файл)
- [ ] Спорное решение → ADR + строка в adr/README.md
- [ ] Секреты/логи не противоречат 17 и 18
- [ ] Смена API → openapi/proto + 03-api (+ 16 / 13* если нужно)
```

## Куда что писать

| Тема | Файл |
|------|------|
| Vision, фазы | `docs/architecture/00-overview.md` |
| Сервисы / порты | `02-services.md` |
| REST/gRPC | `03-api.md` + OpenAPI/proto |
| Данные PG/CH | `05-data-model.md` |
| Kafka | `07-messaging.md` |
| Адаптеры источников | `08-integrations-and-extensibility.md` |
| Local DX | `12-local-dev.md` |
| Тесты | `13-testing.md` (+ `13a` backend, `13b` frontend/E2E) |
| Normalize | `15-normalization-rules.md` |
| UI IA | `16-frontend.md` |
| Secrets | `17-secrets-management.md` |
| Logs/incidents | `18-logging-and-incidents.md` |
| Agent tooling | `19-agent-tooling.md` |
| Code style | `20-code-style.md` |
| Внешние провайдеры | `21-external-services.md` |
| Стиль docs / OpenAPI / proto | `22-documentation-style.md` |
| Решения | `docs/architecture/adr/` |
| Ops | `docs/runbooks/` |

## Правила

1. Документация — **русский**; стиль артефактов — из `22-documentation-style.md`.
2. Не изобретай стек/фазы: HH first; LinkedIn/Avito scraping запрещены.
3. Не реализуй полное приложение «заодно» с правкой docs.
4. Карта файлов и шаблон ADR: [reference.md](reference.md).
