package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/Chuchoss/it-labor-pulse/apps/fx/cbr"
	fxstore "github.com/Chuchoss/it-labor-pulse/apps/fx/store"
	"github.com/Chuchoss/it-labor-pulse/libs/go-common/logging"
)

func main() {
	var dateRaw, fromRaw, toRaw string
	var scheduler bool
	flag.StringVar(&dateRaw, "date", "", "sync one requested date (YYYY-MM-DD)")
	flag.StringVar(&fromRaw, "from", "", "backfill start date (YYYY-MM-DD)")
	flag.StringVar(&toRaw, "to", "", "backfill end date (YYYY-MM-DD)")
	flag.BoolVar(&scheduler, "scheduler", false, "run daily scheduler")
	flag.Parse()
	_ = godotenv.Load()

	log := logging.New(logging.Options{
		Service: "fx-sync", Env: os.Getenv("APP_ENV"), Level: os.Getenv("LOG_LEVEL"),
	})
	slog.SetDefault(log)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	db, err := fxstore.Open(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Error("fx_database_open_failed", "error_category", "database")
		os.Exit(1)
	}
	defer db.Close()

	run := func(ctx context.Context) error {
		dates, err := requestedDates(ctx, db, dateRaw, fromRaw, toRaw, time.Now().UTC())
		if err != nil {
			return err
		}
		lock, acquired, err := db.TryLock(ctx)
		if err != nil || !acquired {
			if err == nil {
				log.Info("fx_sync_skipped", "reason", "lock")
			}
			return err
		}
		defer lock.Release(context.WithoutCancel(ctx))
		runID, err := db.StartRun(ctx, dates[0], dates[len(dates)-1])
		if err != nil {
			return err
		}
		fetchedDates := 0
		var upserted int64
		client := cbr.Client{}
		for _, date := range dates {
			var rates []cbr.Rate
			err = retry(ctx, 3, func(attempt context.Context) error {
				var fetchErr error
				rates, fetchErr = client.Fetch(attempt, date)
				return fetchErr
			})
			if err != nil {
				_ = db.FinishRun(context.WithoutCancel(ctx), runID, "failed", fetchedDates, upserted, "provider")
				return err
			}
			count, storeErr := db.Upsert(ctx, rates, time.Now().UTC())
			if storeErr != nil {
				_ = db.FinishRun(context.WithoutCancel(ctx), runID, "failed", fetchedDates, upserted, "database")
				return storeErr
			}
			fetchedDates++
			upserted += count
			if !wait(ctx, 250*time.Millisecond) {
				return ctx.Err()
			}
		}
		reconciled, err := db.Reconcile(ctx)
		if err != nil {
			_ = db.FinishRun(context.WithoutCancel(ctx), runID, "failed", fetchedDates, upserted, "database")
			return err
		}
		if err := db.FinishRun(ctx, runID, "success", fetchedDates, upserted, ""); err != nil {
			return err
		}
		log.Info("fx_sync_finished",
			"provider", cbr.Provider,
			"requested_dates", len(dates),
			"fetched_dates", fetchedDates,
			"upserted_rates", upserted,
			"vacancies_reconciled", reconciled.VacanciesUpdated,
			"vacancies_missing_rate", reconciled.VacanciesMissing,
			"observations_reconciled", reconciled.ObservationsUpdated,
			"observations_missing_rate", reconciled.ObservationsMissing,
			"source_links_missing", reconciled.SourceLinksMissing,
		)
		return nil
	}

	if !scheduler {
		if err := run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			log.Error("fx_sync_failed", "error_category", "dependency", "error", err.Error())
			os.Exit(1)
		}
		return
	}
	hour := envInt("FX_SYNC_UTC_HOUR", 6)
	for {
		if err := run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			log.Error("fx_sync_failed", "error_category", "dependency", "error", err.Error())
		}
		next := nextDaily(time.Now().UTC(), hour)
		log.Info("fx_sync_scheduled", "next_run_at", next.Format(time.RFC3339))
		if !wait(ctx, time.Until(next)) {
			return
		}
	}
}

func requestedDates(
	ctx context.Context,
	db *fxstore.Postgres,
	dateRaw, fromRaw, toRaw string,
	now time.Time,
) ([]time.Time, error) {
	if dateRaw != "" {
		date, err := time.Parse(time.DateOnly, dateRaw)
		return []time.Time{date}, err
	}
	if fromRaw != "" || toRaw != "" {
		from, err := time.Parse(time.DateOnly, fromRaw)
		if err != nil {
			return nil, fmt.Errorf("invalid -from")
		}
		to, err := time.Parse(time.DateOnly, toRaw)
		if err != nil || to.Before(from) || to.Sub(from) > 366*24*time.Hour {
			return nil, fmt.Errorf("invalid or excessive -to")
		}
		var result []time.Time
		for date := from; !date.After(to); date = date.AddDate(0, 0, 1) {
			result = append(result, date)
		}
		return result, nil
	}
	needed, err := db.NeededDates(ctx, now)
	if err != nil {
		return nil, err
	}
	needed = append(needed, now.AddDate(0, 0, -7), now)
	unique := map[string]time.Time{}
	for _, date := range needed {
		unique[date.UTC().Format(time.DateOnly)] = date.UTC()
	}
	result := make([]time.Time, 0, len(unique))
	for _, date := range unique {
		result = append(result, date)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Before(result[j]) })
	return result, nil
}

func retry(ctx context.Context, attempts int, operation func(context.Context) error) error {
	var err error
	for attempt := 0; attempt < attempts; attempt++ {
		attemptCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
		err = operation(attemptCtx)
		cancel()
		if err == nil {
			return nil
		}
		delay := time.Duration(1<<attempt)*time.Second + time.Duration(rand.IntN(500))*time.Millisecond
		if !wait(ctx, delay) {
			return ctx.Err()
		}
	}
	return err
}

func nextDaily(now time.Time, hour int) time.Time {
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, 0, 0, 0, time.UTC)
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

func wait(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func envInt(name string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(name))
	if err != nil || value < 0 || value > 23 {
		return fallback
	}
	return value
}
