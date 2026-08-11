# Local DX — reference

## Порты (ориентир)

| Service | HTTP | gRPC |
|---------|------|------|
| BFF | 8080 | — |
| Query | 8081 | 9091 |
| Ingest | 8082 | 9092 |
| Web | 3000 | — |
| Postgres | 5432 | — |
| Redis | 6379 | — |

## Предусловия

- Docker Compose v2 (optional `local-redis` / `local-pg`)
- Cloud/managed Postgres рекомендуется (`DATABASE_URL`)
- Cloud/managed Redis рекомендуется (`REDIS_URL`, часто `rediss://`)
- Go 1.22+ (когда есть сервисы)
- Node 20+ (web)
- Make опционален на Windows

## Make (infra)

| Target | Meaning |
|--------|---------|
| `up-cloud` | cloud PG + Redis — nothing to start |
| `up-local-redis` / `up-redis` | Redis container (`local-redis`) |
| `up-local-pg` | Postgres container |
| `up-local` | Redis + Postgres containers |
| `up-mvp` | alias → `up-local-redis` until apps land |

## Не коммитить

`.env`, `*.pem`, `*.key`, `kubeconfig`, `credentials.json` — см. `.gitignore` и hook `block-secret-commit`.
