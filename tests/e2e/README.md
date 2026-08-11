# E2E (Playwright)

Сквозные сценарии LMA против Compose / local stack.

Политика, journeys и когда запускать: [docs/architecture/13b-testing-frontend-e2e.md](../../docs/architecture/13b-testing-frontend-e2e.md), обзор — [13-testing.md](../../docs/architecture/13-testing.md).

## Статус

Placeholder: спеки появятся при реализации Phase 1 UI + BFF. Не коммитить секреты; ingest только в **fixture mode**.

## Ожидаемый layout

```text
tests/e2e/
  playwright.config.ts
  specs/
  support/
```

## Запуск (когда появится)

```bash
make seed-e2e   # ориентир
npx playwright test
```
