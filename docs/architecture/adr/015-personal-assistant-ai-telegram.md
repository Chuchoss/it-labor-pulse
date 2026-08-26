# ADR 015: Personal vacancy assistant with optional AI and Telegram

## Context

Публичный Phase 1 market snapshot не должен превращаться в персональный feed без
identity, opt-in и контроля стоимости внешних API. При этом нужен путь к Phase 4
assistant для одной локальной учётной записи.

## Decision

Вводим отдельный assistant-контур: preferences → deterministic prefilter →
опциональный AI provider → Telegram delivery. Внешние вызовы выключены по
умолчанию и разрешаются только feature flags плюс серверные secrets. DeepSeek
вызывается через узкий provider-neutral интерфейс; ответ принимается только после
строгой JSON/evidence validation. Telegram связывается одноразовым короткоживущим
nonce через `/start`, а не произвольным `chat_id`. Public vacancy listing и
Phase 5 Perspectives остаются отдельными.

## Consequences

(+) Нет AI/Telegram вызовов при обычном локальном старте; deterministic matching
работает без ключей; delivery идемпотентна на уровне БД.

(+) Prompt input минимизирован, PII redacted, vacancy text маркирован DATA.

(-) Production auth, полноценный webhook/polling worker и multi-user tenancy
остаются следующими задачами; dev identity нельзя использовать публично.
