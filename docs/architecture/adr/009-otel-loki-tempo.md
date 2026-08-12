# ADR 009: OpenTelemetry + Loki + Tempo

## Context

Нужна наблюдаемость учебного LMA (Go BFF/ingest/query/workers): искать логи одного запроса/job по корреляции и при необходимости видеть spans. Полный ELK/APM избыточен для соло; формат логов уже зафиксирован как JSON stdout ([18](../18-logging-and-incidents.md)).

## Decision

1. **Корреляция:** W3C `traceparent` → поле лога `trace_id`; отдельно `request_id` (`X-Request-Id`) для API; `ingest_run_id` для async ingest.
2. **Instrumentation:** OpenTelemetry SDK (Go): `otelhttp`, `otelgrpc`, otelslog bridge; Kafka — inject/extract trace context в headers.
3. **Backends:** логи → **Loki** (Alloy/Promtail); трейсы → **Tempo** (или Grafana Cloud Traces); метрики → Prometheus ([11](../11-observability-security.md)); UI → **Grafana** со связкой Loki↔Tempo.
4. **Rollout:** Phase 0–1 — поля в stdout без обязательного коллектора; Phase 2–3 — Compose profile `observability` / `obs` или Grafana Cloud free tier; self-host на Yandex/VPS — later.
5. **Не default:** ELK/EFK, коммерческий APM, отдельный Jaeger как канон (Tempo предпочтителен в Grafana-стеке).

Детали: [23-observability-tracing.md](../23-observability-tracing.md).

## Consequences

- (+) Один вендорский UX (Grafana), дешёвый local profile, понятный upgrade в Cloud  
- (+) Формат логов не меняется при подключении Loki  
- (−) Нужна дисциплина полей (`trace_id` ≠ ULID `request_id`)  
- (−) Self-host Loki/Tempo требует диск/retention discipline  
