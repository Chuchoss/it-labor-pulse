package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Chuchoss/it-labor-pulse/libs/go-common/assistant"
	"github.com/Chuchoss/it-labor-pulse/libs/go-common/logging"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

type confirmer interface {
	ConfirmTelegramLink(context.Context, string, int64) error
}

func main() {
	_ = godotenv.Load()
	log := logging.New(logging.Options{Service: "telegram-linker", Env: envOr("APP_ENV", "local"), Level: envOr("LOG_LEVEL", "info")})
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if os.Getenv("ASSISTANT_TELEGRAM_ENABLED") != "true" || os.Getenv("TELEGRAM_BOT_TOKEN") == "" || os.Getenv("DATABASE_URL") == "" {
		log.Info("telegram_linker_disabled", "reason", "explicit_config_required")
		return
	}
	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Error("telegram_linker_database_failed")
		return
	}
	defer pool.Close()
	client, err := assistant.NewTelegram(os.Getenv("TELEGRAM_BOT_TOKEN"), nil)
	if err != nil {
		log.Error("telegram_linker_config_failed")
		return
	}
	store := assistant.NewPostgresRepository(pool)
	var offset int
	for {
		updates, err := client.GetUpdates(ctx, offset)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Error("telegram_linker_poll_failed", "kind", "provider")
			time.Sleep(5 * time.Second)
			continue
		}
		for _, update := range updates {
			offset = update.UpdateID + 1
			if update.Message == nil || len(update.Message.Text) < 7 || update.Message.Text[:7] != "/start " {
				continue
			}
			if err := store.ConfirmTelegramLink(ctx, update.Message.Text[7:], update.Message.Chat.ID); err != nil {
				log.Warn("telegram_link_failed", "kind", "invalid_or_expired_nonce")
			}
		}
	}
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
