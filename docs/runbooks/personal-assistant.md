# Локальный assistant: DeepSeek и Telegram

Контур assistant отделён от публичного `/api/v1/vacancies`: сначала применяются
структурированные hard-gates и deterministic score, затем (только для
кандидатов) optional AI. При выключенных flags AI не вызывается, а Telegram не
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
ставит в очередь только bounded-окно (до 25 новых вакансий), не запускает
Telegram и не сканирует исторические вакансии. Внешний DeepSeek требует
отдельного server-side opt-in (`ASSISTANT_AI_ENABLED` и explicit live-test gate);
обычный запуск не создаёт расхода провайдера.

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
`activation_at` исключает исторический backlog; AI выключенный не блокирует
deterministic match.

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

## Включение по явному opt-in

1. Создайте бота через официального `@BotFather`, сохраните token в локальном
   `.env` как `TELEGRAM_BOT_TOKEN`.
2. Получите DeepSeek key в кабинете DeepSeek и сохраните его локально как
   `DEEPSEEK_API_KEY`; не вставляйте значение в issue, UI или логи.
3. Проверьте лимит расходов и установите `DEEPSEEK_MODEL`; включайте AI только
   после проверки fake-тестов.
4. Включите flags и перезапустите server-side worker. Для Telegram сначала
   запустите linker, запросите nonce, откройте bot deep-link и отправьте
   `/start <nonce>`; затем отдельно включите opt-in и Telegram automation в UI.
   Автоматически ничего не включается.
5. Для отзыва отключите opt-in/ревокируйте connection и при необходимости
   перевыпустите token у BotFather.

В production потребуется полноценная authentication/tenant isolation. Кнопка
«Тестовое уведомление» требует отдельного подтверждения и делает ровно один
реальный вызов Bot API. Resume tailoring, auto-apply, news и Perspectives Phase
5 в этот контур не входят.
