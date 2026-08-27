# ADR 015: Personal vacancy assistant with optional AI and Telegram

## Context

Публичный Phase 1 market snapshot не должен превращаться в персональный feed без
identity, opt-in и контроля стоимости внешних API. При этом нужен путь к Phase 4
assistant для одной локальной учётной записи.

## Decision

Вводим отдельный assistant-контур: preferences → deterministic assessment →
опциональный AI provider → Telegram delivery. Внешние вызовы выключены по
умолчанию и разрешаются только feature flags плюс серверные secrets. DeepSeek
вызывается через узкий provider-neutral интерфейс; ответ принимается только после
строгой JSON/evidence validation. Telegram связывается одноразовым короткоживущим
nonce через `/start`, а не произвольным `chat_id`. Public vacancy listing и
Phase 5 Perspectives остаются отдельными.

Локальный BFF и worker используют PostgreSQL как durable store: версии
preferences append-only, request idempotency keys возвращают исходную версию,
а match/delivery/work state защищены unique keys. Worker обрабатывает bounded
fresh window с PostgreSQL advisory lock и cursor. `X-Dev-User` допустим только
в `local`/`dev` и является явной single-user development identity, а не
production auth. One-candidate DeepSeek validation требует одновременно
`ASSISTANT_AI_ENABLED=true`, `ASSISTANT_AI_LIVE_TEST=true` и CLI
`-allow-external`; Telegram остаётся выключенным до отдельного opt-in.

Роль HH `96` («Программист, разработчик») остаётся широким официальным scope и
не означает Frontend. Для неё preference хранит явную `specialization`
(`frontend|backend|fullstack|mobile|devops_platform|data_ml|other`) и
`include_leadership`, по умолчанию `false`. Детерминированная классификация
версируется (`specialization-v1`), предпочитает title, затем key skills и только
затем description; неоднозначность даёт `review`. Известный legacy role может
дать только подсказку специализации, но не подтверждает её без нового
immutable-сохранения.

Lifecycle preferences остаётся append-only: UI показывает содержимое всех
версий, а archive помечает версию `archived_at` без удаления evidence. При
архивировании последней активной версии новый запуск не должен обрабатывать
профиль до сохранения новой версии. `assistant_runs` хранит состояние и
агрегированные counters в PostgreSQL; UI может безопасно обновляться после
перезапуска BFF. Ручная кнопка фиксирует `snapshot_cutoff`, текущую
immutable-версию preferences и количество всех активных неудалённых вакансий
активных источников. Worker проходит этот конечный scope keyset-пакетами
`(created_at, id)` и сохраняет cursor/progress между пакетами. Вакансии,
созданные после cutoff, не входят в запуск.

Автоматический режим использует `assistant_work_items` как PostgreSQL outbox:
запись создаётся в той же транзакции, что и upsert вакансии, и уникальна по
`(source, external_id, vacancy_revision)`. Ревизия увеличивается только при
изменении нормализованных analysis-relevant полей, включая очищенное описание.
Worker атомарно claim-ит bounded batch через
`FOR UPDATE SKIP LOCKED`, держит lease и возвращает просроченные leases в
`pending`; завершённые элементы не выбираются после рестарта. AI и Telegram
управляются отдельными пользовательскими настройками
`assistant_automation_settings`, обе по умолчанию выключены. Включение AI
фиксирует `activation_at`, поэтому старые `first_observed_at` не backfill-ятся;
`published_at` остаётся только показателем свежести HH.

После opt-in автоматический AI применяется ко всем новым или содержательно
изменившимся вакансиям, поступившим после `activation_at`, даже если
deterministic assessment дал `reject`; такой результат хранится как AI
`match|reject|review`, а не превращается в совпадение. Отключение прекращает
новые вызовы, повторное включение действует только вперёд. Ручной snapshot
использует текущий opt-in и те же server-side flags, но намеренно охватывает
исторический снимок.

Количество AI-вакансий не ограничивается ни на запуск, ни на пользователя в
час. Worker объединяет ещё не обработанные вакансии одной версии preferences в
адаптивные пакеты: обычно максимум 15 (жёсткий предел 20) и не более настроенного
оценочного token budget. До трёх HTTP-пакетов выполняются параллельно.
Общий контекст и preferences передаются один раз на пакет; каждое решение
сохраняется отдельно. Это осознанный cost-risk. Оператор управляет расходами
остановкой единственного worker, снятием `-allow-external`, отключением
`ASSISTANT_AI_ENABLED`/`ASSISTANT_AI_LIVE_TEST` или пользовательского opt-in.
Контроллер уменьшает concurrency после 429, timeout, context-limit или
некорректного ответа и медленно восстанавливает её после успешных волн.
Ограничение batch size и provider rate limiting управляют пропускной
способностью, но не общим числом вакансий. Идемпотентность AI:
`(user, preference, vacancy, vacancy_revision)`; provider failures повторяются
до пяти раз и затем переходят в dead-letter. Ошибка/отсутствие AI не отменяет
сохранённый deterministic результат.

В пользовательской выдаче deterministic `match` имеет статус preliminary.
Завершённое AI-решение той же preference/vacancy revision имеет приоритет:
AI `reject|review` скрывает предварительное совпадение из положительного списка,
а `match` становится `confirmed`. Telegram при включённом AI ставится в очередь
только после AI `match`.

AI получает только `hard_criteria`; свободная заметка и дополнительные пожелания
не превращаются в обязательные условия. Неизвестные salary/remote/region/skills
дают `review`, если известные данные не опровергают hard-критерий. Для строгого
Frontend ясный Frontend IC может дать `match`, Backend/Fullstack и leadership
при `include_leadership=false` дают `reject`.

Batch-ответ — JSON-объект с ровно одним решением на каждый opaque
`vacancy_id`. Duplicate/unknown ID отклоняет пакет; missing item остаётся
retryable. Context-limit, truncation, malformed и partial output рекурсивно
делят только неразрешённые элементы до singleton fallback. Счётчики вакансий
отделены от HTTP attempts, batches, retries и provider-reported token usage.

Ручной snapshot и outbox не создают отдельные копии вакансий. Если одна вакансия
попала в оба пути, unique key результата `(user, preference, vacancy, method, …)`
делает повторную запись безопасной. Один активный run на пользователя
обеспечивается partial unique index, claim/recovery — lease,
`FOR UPDATE SKIP LOCKED` и advisory worker lock.

Telegram delivery остаётся at-least-once: уникальный ключ notification,
provider message id, bounded retries, lease, advisory lock и cooldown защищают
от повторных уведомлений, но exactly-once внешний Bot API не обещается.
Timeout после принятия провайдером имеет ambiguous outcome и может быть
повторен. При отсутствии подтверждённой неотозванной связи, отдельного opt-in
или пользовательского automation flag сообщение не ставится в отправку.
Long-poll linker подтверждает только одноразовый hashed nonce из `/start`;
произвольный chat_id запрещён. Тестовая отправка требует явного подтверждения
в UI и не включает automation.

## Consequences

(+) Нет AI/Telegram вызовов при обычном локальном старте; deterministic matching
работает без ключей; delivery идемпотентна на уровне БД.

(+) Ручной запуск охватывает существующие вакансии, а не только события,
появившиеся после включения assistant.

(-) Верхней границы стоимости одного запуска нет; UI показывает размер снимка
до подтверждения и предупреждает, что число платных запросов может быть большим.

(+) Prompt input минимизирован, PII redacted, очищенное описание ограничено;
vacancy text отделён маркерами как недоверенные данные, инструкции из него
запрещено выполнять.

(-) Production auth и multi-user tenancy остаются следующими задачами; dev
identity нельзя использовать публично. Long polling для локального linker
нужно заменить на production webhook/managed worker при развёртывании.

(-) `raw_payload` остаётся внутренним source evidence по общей retention
политике; assistant получает только очищенный plain text. Dead-letter элементы
не переотправляются автоматически.
