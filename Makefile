# LMA local DX — see docs/architecture/12-local-dev.md
# Windows without Make: use the docker compose / curl equivalents in that doc.

COMPOSE_FILE := deploy/compose/docker-compose.yml
ENV_FILE     := .env
COMPOSE      := docker compose --env-file $(ENV_FILE) -f $(COMPOSE_FILE)
# Optional local data plane; cloud PG/Redis need no Compose services.
PROFILES_INFRA := --profile local-redis --profile local-pg

.PHONY: help up-cloud up-local-redis up-redis up-mvp up-local-pg up-local up-full up-obs down logs ps wait-ready psql redis-cli migrate-up migrate-down bust-cache run-bff run-web run-ingest-scheduler run-hydrate-scheduler run-discovery-scheduler discovery-once run-analytics-scheduler analytics-daily analytics-weekly analytics-backfill ingest-hh ingest-hh-it-plan ingest-hh-it ingest-hh-fixture test test-go test-web

help:
	@echo "LMA local targets:"
	@echo "  make up-cloud       - cloud PG + cloud Redis: nothing to start (check .env URLs)"
	@echo "  make up-local-redis - start local Redis container (profile local-redis)"
	@echo "  make up-redis       - alias for up-local-redis"
	@echo "  make up-local-pg    - start local Postgres container (profile local-pg)"
	@echo "  make up-local       - start local Redis + local Postgres"
	@echo "  make up-mvp         - reserved for Phase 0–1 apps; for now see up-cloud / up-local-*"
	@echo "  make up-obs         - Loki + Tempo + Alloy + Prometheus + Grafana (profile observability)"
	@echo "  make run-bff        - run public BFF on :8080 (loads .env)"
	@echo "  make run-web        - run Vite SPA on :3000 (proxy to BFF :8080)"
	@echo "  make ingest-hh      - one-shot HH ingest → normalize → PG (needs DATABASE_URL + HH_USER_AGENT)"
	@echo "  make ingest-hh-it-plan - aggregate-only plan for all official HH IT roles in Russia"
	@echo "  make ingest-hh-it   - bounded/resumable all-IT crawl (explicit; may take multiple runs)"
	@echo "  make run-discovery-scheduler - daily search-only snapshots (01:00 UTC)"
	@echo "  make discovery-once - run/resume one bounded daily discovery"
	@echo "  make run-hydrate-scheduler - background detail hydration"
	@echo "  make analytics-daily - snapshot latest eligible complete all-IT cycle"
	@echo "  make analytics-weekly - roll up current ISO week from daily snapshots"
	@echo "  make analytics-backfill - process eligible complete cycles and weeks"
	@echo "  make run-analytics-scheduler - poll durable cycle markers and weekly rollups"
	@echo "  make ingest-hh-fixture - same path using testdata/hh (no live HH; needs DATABASE_URL)"
	@echo "  make test           - Go + frontend unit tests"
	@echo "  make down           - stop compose stack (local-redis + local-pg)"
	@echo "  make wait-ready     - wait until local-redis is healthy (skip if cloud Redis)"
	@echo "  make logs           - follow compose logs"
	@echo "  make ps             - compose ps"
	@echo "  make psql           - psql into local postgres (needs up-local-pg)"
	@echo "  make redis-cli      - redis-cli into local redis (needs up-local-redis)"
	@echo "  make migrate-up     - apply PG migrations via DATABASE_URL"
	@echo "  make migrate-down   - roll back one PG migration (dev only)"
	@echo "  make bust-cache     - INCR meta:cache_version in local Redis"

# Ensure .env exists (copy from example once).
$(ENV_FILE):
	@if [ ! -f $(ENV_FILE) ]; then cp .env.example $(ENV_FILE); echo "Created $(ENV_FILE) from .env.example — edit secrets locally."; fi

# Cloud-everything: managed DATABASE_URL + REDIS_URL — Compose starts no infra.
up-cloud: $(ENV_FILE)
	@echo "Cloud path: no Compose services required."
	@echo "Ensure in .env (do not commit):"
	@echo "  DATABASE_URL=postgres://...@.../...?sslmode=require"
	@echo "  REDIS_URL=rediss://...@...:...  (or redis:// if provider has no TLS)"
	@echo "Optional discrete REDIS_ADDR / REDIS_PASSWORD for clients that do not take a URL."
	@echo "Local containers if needed: make up-local-redis / up-local-pg / up-local"
	@echo "Docs: docs/architecture/12-local-dev.md#облачный-redis"

# Optional local Redis (does not start Postgres).
up-local-redis: $(ENV_FILE)
	$(COMPOSE) --profile local-redis up -d --wait

# Backward-compatible aliases (local Redis container).
up-redis: up-local-redis
up-mvp: up-local-redis

# Optional local Postgres (does not start Redis).
up-local-pg: $(ENV_FILE)
	$(COMPOSE) --profile local-pg up -d --wait

# Full local infra: Redis + Postgres containers.
up-local: $(ENV_FILE)
	$(COMPOSE) $(PROFILES_INFRA) up -d --wait

up-full: $(ENV_FILE)
	@echo "Profile full is not wired yet (Phase 2+). Use make up-cloud, up-local-redis, or up-local."
	@exit 1

# Optional observability: Loki + Tempo + Alloy + Prometheus + Grafana (http://localhost:3001).
# Docs: docs/architecture/23-observability-tracing.md
up-obs: $(ENV_FILE)
	$(COMPOSE) --profile observability up -d
	@echo "Grafana: http://localhost:${GRAFANA_PORT:-3001}  OTLP HTTP: localhost:${OTEL_HOST_PORT_HTTP:-4318}"

down:
	$(COMPOSE) $(PROFILES_INFRA) down

logs:
	$(COMPOSE) $(PROFILES_INFRA) logs -f

ps:
	$(COMPOSE) $(PROFILES_INFRA) ps

# Wait for local Redis when using profile local-redis.
# Cloud Redis: make up-cloud (no container to wait for).
# For local PG too: make wait-ready PROFILES="--profile local-redis --profile local-pg"
wait-ready: $(ENV_FILE)
	@echo "Waiting for local Redis (compose --wait)..."
	$(COMPOSE) --profile local-redis up -d --wait
	@echo "OK: redis is healthy (profile local-redis)."
	@echo "Cloud Redis: set REDIS_URL in .env and skip this target (make up-cloud)."
	@echo "Cloud PG: ensure DATABASE_URL in .env; migrate later with make migrate-up."
	@echo "Local PG: make up-local-pg (or make up-local) if you need the container."
	@echo "Note: BFF /api/v1/health will be checked here once apps are wired in Compose."

psql: $(ENV_FILE)
	$(COMPOSE) --profile local-pg exec postgres \
		psql -U $${POSTGRES_USER:-lma} -d $${POSTGRES_DB:-lma}

redis-cli:
	$(COMPOSE) --profile local-redis exec redis redis-cli

# golang-migrate via official image; needs at least one *.up.sql under migrations/postgres.
# Cloud: DATABASE_URL as-is + sslmode/connect_timeout if missing (Supabase session pooler :5432).
# Local compose PG: if DATABASE_URL/host points at localhost, rewrite to service "postgres" on lma_net.
migrate-up: $(ENV_FILE)
	@set -a; . ./$(ENV_FILE); set +a; \
	files=$$(find migrations/postgres -name '*.up.sql' 2>/dev/null | wc -l | tr -d ' '); \
	if [ "$$files" = "0" ]; then \
		echo "No migrations yet under migrations/postgres/ — skipping."; \
		exit 0; \
	fi; \
	DB_URL="$${DATABASE_URL:-}"; \
	NETWORK_ARGS=""; \
	if [ -z "$$DB_URL" ] || echo "$$DB_URL" | grep -Eq '@(localhost|127\.0\.0\.1)(:|/|\?)'; then \
		NETWORK_ARGS="--network lma_net"; \
		DB_URL="postgres://$${POSTGRES_USER}:$${POSTGRES_PASSWORD}@postgres:5432/$${POSTGRES_DB}?sslmode=disable"; \
		echo "migrate-up: local compose postgres via lma_net"; \
	else \
		echo "migrate-up: using DATABASE_URL (cloud/remote)"; \
		echo "$$DB_URL" | grep -Eq 'sslmode=' || { \
			case "$$DB_URL" in *\?*) DB_URL="$${DB_URL}&sslmode=require";; *) DB_URL="$${DB_URL}?sslmode=require";; esac; \
		}; \
		echo "$$DB_URL" | grep -Eq 'connect_timeout=' || { \
			case "$$DB_URL" in *\?*) DB_URL="$${DB_URL}&connect_timeout=60";; *) DB_URL="$${DB_URL}?connect_timeout=60";; esac; \
		}; \
	fi; \
	docker run --rm $$NETWORK_ARGS \
		-v "$(CURDIR)/migrations/postgres:/migrations:ro" \
		migrate/migrate:v4.18.1 \
		-path=/migrations \
		-database "$$DB_URL" \
		up

migrate-down: $(ENV_FILE)
	@set -a; . ./$(ENV_FILE); set +a; \
	DB_URL="$${DATABASE_URL:-}"; \
	NETWORK_ARGS=""; \
	if [ -z "$$DB_URL" ] || echo "$$DB_URL" | grep -Eq '@(localhost|127\.0\.0\.1)(:|/|\?)'; then \
		NETWORK_ARGS="--network lma_net"; \
		DB_URL="postgres://$${POSTGRES_USER}:$${POSTGRES_PASSWORD}@postgres:5432/$${POSTGRES_DB}?sslmode=disable"; \
	else \
		echo "$$DB_URL" | grep -Eq 'sslmode=' || { \
			case "$$DB_URL" in *\?*) DB_URL="$${DB_URL}&sslmode=require";; *) DB_URL="$${DB_URL}?sslmode=require";; esac; \
		}; \
		echo "$$DB_URL" | grep -Eq 'connect_timeout=' || { \
			case "$$DB_URL" in *\?*) DB_URL="$${DB_URL}&connect_timeout=60";; *) DB_URL="$${DB_URL}?connect_timeout=60";; esac; \
		}; \
	fi; \
	docker run --rm $$NETWORK_ARGS \
		-v "$(CURDIR)/migrations/postgres:/migrations:ro" \
		migrate/migrate:v4.18.1 \
		-path=/migrations \
		-database "$$DB_URL" \
		down 1

bust-cache:
	$(COMPOSE) --profile local-redis exec redis redis-cli INCR meta:cache_version

# Phase 0: public BFF on :8080 — ADR 010 (revised)
run-bff: $(ENV_FILE)
	@set -a; . ./$(ENV_FILE); set +a; \
	go run ./apps/bff/cmd/bff

run-web:
	npm --prefix apps/web run dev

# Phase 1: one-shot HH ingest (official API only). Requires migrate-up first.
ingest-hh: $(ENV_FILE)
	@set -a; . ./$(ENV_FILE); set +a; \
	go run ./apps/ingest/cmd/ingest

ingest-hh-it-plan: $(ENV_FILE)
	@set -a; . ./$(ENV_FILE); set +a; \
	go run ./apps/ingest/cmd/ingest -scope it -dry-run

ingest-hh-it: $(ENV_FILE)
	@set -a; . ./$(ENV_FILE); set +a; \
	go run ./apps/ingest/cmd/ingest -scope it

run-ingest-scheduler: $(ENV_FILE)
	@set -a; . ./$(ENV_FILE); set +a; \
	INGEST_SCOPE=it go run ./apps/ingest/cmd/scheduler

run-hydrate-scheduler: run-ingest-scheduler

run-discovery-scheduler: $(ENV_FILE)
	@set -a; . ./$(ENV_FILE); set +a; \
	go run ./apps/ingest/cmd/discovery

discovery-once: $(ENV_FILE)
	@set -a; . ./$(ENV_FILE); set +a; \
	go run ./apps/ingest/cmd/discovery -once

analytics-daily: $(ENV_FILE)
	@set -a; . ./$(ENV_FILE); set +a; \
	go run ./apps/analytics/cmd/worker -mode daily

analytics-weekly: $(ENV_FILE)
	@set -a; . ./$(ENV_FILE); set +a; \
	go run ./apps/analytics/cmd/worker -mode weekly

analytics-backfill: $(ENV_FILE)
	@set -a; . ./$(ENV_FILE); set +a; \
	go run ./apps/analytics/cmd/worker -mode backfill

run-analytics-scheduler: $(ENV_FILE)
	@set -a; . ./$(ENV_FILE); set +a; \
	go run ./apps/analytics/cmd/worker -mode scheduler

# Offline smoke: fixtures from testdata/hh (no live HH call).
ingest-hh-fixture: $(ENV_FILE)
	@set -a; . ./$(ENV_FILE); set +a; \
	go run ./apps/ingest/cmd/ingest -fixture

test: test-go test-web

test-go:
	go test ./...

test-web:
	npm --prefix apps/web test
