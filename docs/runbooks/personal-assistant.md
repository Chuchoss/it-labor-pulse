# Локальный assistant: DeepSeek и Telegram

Контур assistant отделён от публичного `/api/v1/vacancies`: deterministic
assessment считает hard-критерии, а включённый AI независимо проверяет каждую
новую eligible-вакансию по очищенному описанию. При выключенных flags AI не вызывается, а Telegram не
отправляет сообщений.

## Жизненный цикл критериев и статус анализа

Каждое сохранение добавляет новую immutable-версию `vacancy_preferences`;
архивирование только заполняет `archived_at`, поэтому старые критерии и
evidence совпадений остаются доступны для аудита. Архивная последняя версия
не выбирается для будущей обработки: интерфейс предлагает сохранить новую
версию. Удаление строк и скрытое удаление данных не используются.

Детерминированный matcher принимает только структурированные hard-критерии
`approved_roles`, `regions`, `required_skills`, `excluded_skills`,
`remote_only`, `min_salary_rub`; неизвестные поля возвращают `400`.
Свободная заметка и неподдержанные soft-критерии не участвуют в matcher.

### Совместимость legacy role

Каноническое значение `approved_roles` — official HH professional role ID.
Нормализация ограничена фиксированным списком (регистр, `_` и `-` не важны):

- `backend`, `backend developer`, `frontend`, `frontend developer`,
  `fullstack`, `full stack`, `fullstack developer`, `developer`, `programmer`,
  `software developer` → `96` («Программист, разработчик»);
- `team lead`, `teamlead`, `lead developer` → `104`
  («Руководитель группы разработки»);
- `qa`, `qa engineer`, `tester`, `quality assurance` → `124`
  («Тестировщик»);
- `system analyst`, `systems analyst` → `148`; `business analyst` → `150`;
  `bi analyst`, `data analyst` → `156`; `product analyst` → `164`.

Другие значения не угадываются и возвращают `400`. Чтение известного legacy
значения нормализует только модель ответа/worker; исходная версия в PostgreSQL
остаётся неизменной. Следующее сохранение создаёт новую версию без `role`, с
`approved_roles`. Free-text заметка может содержать «backend», но не участвует
в structured role gate.

`GET /api/v1/assistant/status` читает последний run из PostgreSQL. Состояния
`never_run`, `queued`, `running`, `succeeded`, `failed`, `disabled` и счётчики
детерминированного matcher/AI не являются in-memory состоянием. Кнопка запуска
фиксирует снимок всех текущих активных неудалённых вакансий активных источников.
Worker обрабатывает снимок пакетами до 25 строк по keyset cursor; `total` известен
при создании, progress сохраняется после каждого пакета. Созданные после cutoff
вакансии остаются для следующего ручного запуска и outbox. Внешний DeepSeek
требует отдельного server-side opt-in (`ASSISTANT_AI_ENABLED` и explicit
live-test gate); при выключенном AI полный deterministic scan завершается без
внешних вызовов. `succeeded` относится к локальной проверке снимка и не
утверждает, что AI вызывался: отдельные `ai_status` и `ai_skip_reason`
показывают provider lifecycle. При `ai_calls=0` UI выводит «не выполнялся» и
не показывает нулевые AI-совпадения как результат провайдера.

HH ingest получает search list, затем официальную detail-карточку
`GET /vacancies/{id}`. HTML-описание превращается в plain text: script/style и
разметка удаляются, entities декодируются, whitespace нормализуется, лимит —
12 000 Unicode-символов с `description_truncated=true`. Assistant передаёт
провайдеру не более 8 000 символов вместе со структурированными фактами и
текущей версией предпочтений. Текст вакансии отделён как недоверенные данные;
prompt запрещает выполнять инструкции из описания.

## Конфигурация

Скопируйте `.env.example` в `.env`. Никогда не передавайте ключи в React или
чат. Для обычной локальной проверки оставьте `ASSISTANT_ENABLED=false`,
`ASSISTANT_AI_ENABLED=false`, `ASSISTANT_TELEGRAM_ENABLED=false`.

Для локального экрана `/assistant` явно включите BFF-маршруты в своём `.env`
(файл не коммитится):

```text
ASSISTANT_ENABLED=true
ASSISTANT_DEV_AUTH_ENABLED=true
ASSISTANT_DEV_SUBJECT=local-dev-user
```

После изменения флагов перезапустите только BFF на `:8080`; Vite остаётся на
`:3000` и проксирует `/api` на этот BFF. Без `ASSISTANT_ENABLED=true` маршруты
assistant намеренно не регистрируются и возвращают 404. В local/dev subject
берётся из `X-Dev-User` (его отправляет React) или из
`ASSISTANT_DEV_SUBJECT`; в production dev-auth не включается.

DeepSeek использует официальный OpenAI-compatible endpoint
`https://api.deepseek.com/chat/completions`, `Authorization: Bearer`,
`response_format: {"type":"json_object"}` и модель `deepseek-v4-flash` по
умолчанию. См. [официальный quick start](https://api-docs.deepseek.com/) и
[Chat Completions API](https://api-docs.deepseek.com/api/create-chat-completion).
Проверяйте актуальные модели и цены перед включением billing.

Telegram использует только официальный Bot API: [sendMessage и
getUpdates](https://core.telegram.org/bots/api/). Webhook и long polling
взаимоисключающие. Команда `go run ./apps/assistant/cmd/telegram-linker`
использует безопасный long polling только при `ASSISTANT_TELEGRAM_ENABLED=true`,
`DATABASE_URL` и `TELEGRAM_BOT_TOKEN`; `/start <nonce>` одноразовый и сохраняется
только в виде hash. Произвольный `chat_id` не принимается.

## Delivery loop

`go run ./apps/assistant/cmd/worker` обрабатывает bounded delivery batch после
matcher. Для защиты от overlap используется отдельный PostgreSQL advisory lock,
`FOR UPDATE SKIP LOCKED`, lease и до пяти попыток. 429 уважает `Retry-After`,
временные ошибки используют backoff, остальные ошибки и исчерпанные попытки
попадают в dead-letter (`dead_letter_at`). Статусы `pending/sent/failed`,
`provider_message_id`, counters и безопасная последняя ошибка видны в UI.
Telegram delivery — at-least-once: timeout после принятия Bot API считается
неопределённым исходом и может привести к повтору; exactly-once не обещается.
Вакансия отправляется только при глобальном флаге, пользовательском флаге,
подтверждённой неотозванной связи, opt-in и deterministic/AI `match`.
`activation_at` исключает исторический backlog только из автоматического
outbox-пути; ручной snapshot намеренно анализирует существующие вакансии.
Пересечение snapshot/outbox безопасно дедуплицируется unique result key.

AI-идемпотентность включает пользователя, immutable preference, вакансию и её
analysis revision. Автоматический режим не backfill-ит вакансии до
`activation_at`, но повторно ставит содержательно изменившуюся ревизию.
Лимитов количества AI-запросов на запуск и на пользователя в час нет. Каждый
элемент снимка или допустимый новый outbox item может создать платный запрос;
batch size/rate limiting ограничивают только скорость. Provider
429/5xx/network/timeout повторяются до трёх HTTP-попыток с экспоненциальной
задержкой и jitter; `Retry-After` имеет приоритет. Некорректный JSON/схема
получает один repair retry, auth/quota/invalid-request не повторяются. Timeout
одного запроса по умолчанию 90 секунд. Счётчики отдельно показывают отправленные
AI-вакансии, все HTTP-попытки, retries и финальные ошибки по безопасным
категориям без provider body.

Чтобы немедленно остановить новые расходы, завершите единственный worker.
После аварийной остановки зафиксируйте resumable-состояние:

```powershell
go run ./apps/assistant/cmd/worker -pause-run <run-id>
# после исправления:
go run ./apps/assistant/cmd/worker -resume-run <run-id>
```

`paused` не завершает и не помечает run ошибочным; cursor и успешные решения
сохраняются. Повторный запуск не вызывает AI для уже завершённых решений.
Для устойчивого отключения снимите `-allow-external`, установите
`ASSISTANT_AI_ENABLED=false` (и перезапустите worker) либо выключите
пользовательский AI opt-in. Не запускайте replay dead-letter или ручной анализ
при расследовании. Исторический `budget_exhausted` остаётся читаемым статусом
старых запусков, но больше не создаётся.

## Безопасный smoke

```bash
go test ./libs/go-common/assistant
go test ./...
npm --prefix apps/web run typecheck
npm --prefix apps/web test
npm --prefix apps/web run build
```

Тесты используют `httptest` и fake-клиенты: внешние DeepSeek/Telegram запросы
не выполняются.

Worker запускается в ограниченном режиме одной пачкой:

```powershell
go run ./apps/assistant/cmd/worker -once
```

Без настроенного persistent assistant store команда завершается с агрегатами
`users=0`, не создаёт локальную identity и не вызывает внешние API. В рабочем
контуре worker должен быть подключён к PostgreSQL repository, запускаться одним
экземпляром (advisory lock) и обрабатывать ручной snapshot либо свежий outbox.

## Включение по явному opt-in

1. Создайте бота через официального `@BotFather`, сохраните token в локальном
   `.env` как `TELEGRAM_BOT_TOKEN`.
2. Получите DeepSeek key в кабинете DeepSeek и сохраните его локально как
   `DEEPSEEK_API_KEY`; не вставляйте значение в issue, UI или логи.
3. Проверьте тариф и доступный баланс, установите `DEEPSEEK_MODEL`; верхней
   границы числа запросов нет, поэтому включайте AI только после fake-тестов.
4. Для реального DeepSeek одновременно установите
   `ASSISTANT_AI_ENABLED=true`, `ASSISTANT_AI_LIVE_TEST=true`, включите
   автоматический AI-анализ в UI и перезапустите worker командой
   `go run ./apps/assistant/cmd/worker -allow-external`. Без любого из этих
   opt-in внешний вызов не выполняется. Для Telegram сначала
   запустите linker, запросите nonce, откройте bot deep-link и отправьте
   `/start <nonce>`; затем отдельно включите opt-in и Telegram automation в UI.
   Автоматически ничего не включается.
5. Для отзыва отключите opt-in/ревокируйте connection и при необходимости
   перевыпустите token у BotFather.

В production потребуется полноценная authentication/tenant isolation. Кнопка
«Тестовое уведомление» требует отдельного подтверждения и делает ровно один
реальный вызов Bot API. Resume tailoring, auto-apply, news и Perspectives Phase
5 в этот контур не входят.
