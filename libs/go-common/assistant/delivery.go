package assistant

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

type Delivery struct {
	ID, UserID, VacancyID, Title, Salary, SourceURL string
	ChatID                                          int64
	Score                                           float64
	Confidence                                      string
	Reasons                                         []string
	Attempts                                        int
}

type DeliveryStore interface {
	TryDeliveryLock(context.Context) (func() error, bool, error)
	ClaimDeliveries(context.Context, string, int, time.Duration) ([]Delivery, error)
	MarkDeliverySent(context.Context, string, int) error
	MarkDeliveryFailed(context.Context, string, string, time.Time, bool) error
}

type DeliveryOptions struct {
	BatchSize, MaxAttempts int
	Lease                  time.Duration
	Now                    time.Time
	Log                    *slog.Logger
}

type DeliveryStats struct {
	Claimed, Sent, Retried, DeadLettered, Skipped int
}

func RunDeliveryOnce(ctx context.Context, store DeliveryStore, client DeliveryTelegramClient, opts DeliveryOptions) (DeliveryStats, error) {
	if client == nil {
		return DeliveryStats{}, errors.New("telegram delivery client is required")
	}
	if opts.BatchSize < 1 || opts.BatchSize > 100 {
		opts.BatchSize = 25
	}
	if opts.MaxAttempts < 1 || opts.MaxAttempts > 20 {
		opts.MaxAttempts = 5
	}
	if opts.Lease <= 0 {
		opts.Lease = 2 * time.Minute
	}
	if opts.Now.IsZero() {
		opts.Now = time.Now().UTC()
	}
	release, acquired, err := store.TryDeliveryLock(ctx)
	if err != nil || !acquired {
		return DeliveryStats{}, err
	}
	defer func() { _ = release() }()
	items, err := store.ClaimDeliveries(ctx, "telegram-delivery", opts.BatchSize, opts.Lease)
	if err != nil {
		return DeliveryStats{}, err
	}
	stats := DeliveryStats{Claimed: len(items)}
	for _, item := range items {
		if err := ctx.Err(); err != nil {
			return stats, err
		}
		message := TelegramHTML(item.Title, item.Salary, item.SourceURL, item.Score, item.Confidence, item.Reasons)
		result, sendErr := client.SendMessageResult(ctx, item.ChatID, message)
		if sendErr == nil {
			if err := store.MarkDeliverySent(ctx, item.ID, result.MessageID); err != nil {
				return stats, err
			}
			stats.Sent++
			continue
		}
		var telegramErr *TelegramError
		retryAt := opts.Now.Add(backoff(item.Attempts))
		if errors.As(sendErr, &telegramErr) && telegramErr.RetryAfter > 0 {
			retryAt = opts.Now.Add(telegramErr.RetryAfter)
		}
		dead := item.Attempts+1 >= opts.MaxAttempts || (telegramErr != nil && telegramErr.StatusCode >= 400 && telegramErr.StatusCode < 500 && telegramErr.StatusCode != 429)
		if err := store.MarkDeliveryFailed(ctx, item.ID, sanitizeDeliveryError(sendErr), retryAt, dead); err != nil {
			return stats, err
		}
		if dead {
			stats.DeadLettered++
		} else {
			stats.Retried++
		}
	}
	if opts.Log != nil {
		opts.Log.Info("telegram_delivery_complete", "stats", stats)
	}
	return stats, nil
}

func backoff(attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	if attempt > 6 {
		attempt = 6
	}
	return time.Duration(1<<attempt) * time.Second
}

func sanitizeDeliveryError(err error) string {
	if err == nil {
		return ""
	}
	var telegramErr *TelegramError
	if errors.As(err, &telegramErr) {
		return fmt.Sprintf("telegram status %d", telegramErr.StatusCode)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "telegram request timeout (outcome ambiguous)"
	}
	if errors.Is(err, context.Canceled) {
		return "telegram request canceled"
	}
	return "telegram request failed"
}
