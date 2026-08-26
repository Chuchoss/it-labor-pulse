# Локальный assistant: DeepSeek и Telegram

Контур assistant отделён от публичного `/api/v1/vacancies`: сначала применяются
структурированные hard-gates и deterministic score, затем (только для
кандидатов) optional AI. При выключенных flags AI не вызывается, а Telegram не
отправляет сообщений.

## Конфигурация

Скопируйте `.env.example` в `.env`. Никогда не передавайте ключи в React или
чат. Для обычной локальной проверки оставьте `ASSISTANT_ENABLED=false`,
`ASSISTANT_AI_ENABLED=false`, `ASSISTANT_TELEGRAM_ENABLED=false`.

DeepSeek использует официальный OpenAI-compatible endpoint
`https://api.deepseek.com/chat/completions`, `Authorization: Bearer`,
`response_format: {"type":"json_object"}` и модель `deepseek-v4-flash` по
умолчанию. См. [официальный quick start](https://api-docs.deepseek.com/) и
[Chat Completions API](https://api-docs.deepseek.com/api/create-chat-completion).
Проверяйте актуальные модели и цены перед включением billing.

Telegram использует только официальный Bot API: [sendMessage и
getUpdates](https://core.telegram.org/bots/api/). Webhook и long polling
взаимоисключающие; в текущем локальном foundation реальный polling не запускается.

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
   запросите nonce, откройте bot deep-link и подтвердите opt-in; произвольный
   `chat_id` из браузера не принимается.
5. Для отзыва отключите opt-in/ревокируйте connection и при необходимости
   перевыпустите token у BotFather.

В production потребуется полноценная authentication/tenant isolation и
background polling/webhook worker. Resume tailoring, auto-apply, news и
Perspectives Phase 5 в этот контур не входят.
