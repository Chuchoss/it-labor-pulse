# Integration tests (Go)

Интеграционные проверки с PostgreSQL / Redis (testcontainers или Compose). Kafka/Redpanda — с Phase 2.

Политика и сценарии: [docs/architecture/13a-testing-backend.md](../../docs/architecture/13a-testing-backend.md), обзор — [13-testing.md](../../docs/architecture/13-testing.md).

## Статус

Placeholder. Предпочтительно также `//go:build integration` рядом с пакетами `repo`/`store`; этот каталог — для сквозных multi-package suites.

## Запуск (когда появится)

```bash
go test -tags=integration ./tests/integration/...
# или: make test-integration
```

Требуется Docker. Без живого HH: только `testdata/hh` + httptest.
