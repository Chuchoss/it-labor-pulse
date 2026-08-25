package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/joho/godotenv"

	"github.com/Chuchoss/it-labor-pulse/apps/ingest/internal/config"
	"github.com/Chuchoss/it-labor-pulse/apps/ingest/internal/hh"
	"github.com/Chuchoss/it-labor-pulse/apps/ingest/internal/pipeline"
	"github.com/Chuchoss/it-labor-pulse/apps/ingest/internal/store"
	"github.com/Chuchoss/it-labor-pulse/libs/go-common/logging"
	"github.com/Chuchoss/it-labor-pulse/libs/go-common/normalize"
)

func main() {
	_ = godotenv.Load()

	fixture := flag.Bool("fixture", false, "use testdata/hh instead of live HH API")
	area := flag.String("area", "", "HH area id (default INGEST_DEFAULT_AREA)")
	text := flag.String("text", "", "HH search text (default INGEST_DEFAULT_TEXT)")
	maxPages := flag.Int("max-pages", 0, "max pages (default INGEST_MAX_PAGES)")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}
	if *fixture && cfg.FixtureDir == "" {
		cfg.FixtureDir = findDefaultFixtureDir()
	}
	if err := cfg.ValidateLive(); err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}

	log := logging.New(logging.Options{
		Service: "ingest",
		Env:     cfg.AppEnv,
		Level:   cfg.LogLevel,
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("database_open_failed", "err", err.Error())
		os.Exit(1)
	}
	defer func() { _ = st.Close() }()

	var src pipeline.Source
	if cfg.FixtureDir != "" {
		log.Info("ingest_fixture_mode", "dir", cfg.FixtureDir)
		src = hh.NewFixtureSource(cfg.FixtureDir)
	} else {
		client, err := hh.NewClient(hh.ClientOptions{
			BaseURL:   cfg.HHBaseURL,
			UserAgent: cfg.HHUserAgent,
			AppToken:  cfg.HHAppToken,
			PageDelay: cfg.PageDelay,
		})
		if err != nil {
			log.Error("hh_client_failed", "err", err.Error())
			os.Exit(1)
		}
		src = client
	}

	p := pipeline.Params{
		Area:        cfg.DefaultArea,
		Text:        cfg.DefaultText,
		Mode:        "incremental",
		MaxPages:    cfg.MaxPages,
		RequestedBy: "cli",
	}
	if *area != "" {
		p.Area = *area
	}
	if *text != "" {
		p.Text = *text
	}
	if *maxPages > 0 {
		p.MaxPages = *maxPages
	}

	runner := &pipeline.Runner{
		Source: src,
		Store:  st,
		Log:    log,
		Opts:   normalize.DefaultOptions(),
	}
	runCtx, cancelRun := context.WithTimeout(ctx, cfg.RunTimeout)
	defer cancelRun()
	res, err := runner.Run(runCtx, p)
	if err != nil {
		log.Error("ingest_failed", "ingest_run_id", res.RunID, "err", err.Error(), "status", res.Status)
		os.Exit(1)
	}
	log.Info("ingest_ok",
		"ingest_run_id", res.RunID,
		"status", res.Status,
		"fetched", res.Stats.Fetched,
		"upserted", res.Stats.Upserted,
		"unchanged", res.Stats.Unchanged,
		"errors", res.Stats.Errors,
	)
}

func findDefaultFixtureDir() string {
	candidates := []string{
		"testdata/hh",
		filepath.Join("..", "..", "..", "testdata", "hh"),
	}
	wd, err := os.Getwd()
	if err == nil {
		candidates = append([]string{filepath.Join(wd, "testdata", "hh")}, candidates...)
		// Walk up looking for go.mod + testdata/hh
		dir := wd
		for i := 0; i < 6; i++ {
			p := filepath.Join(dir, "testdata", "hh")
			if st, err := os.Stat(p); err == nil && st.IsDir() {
				return p
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && st.IsDir() {
			return c
		}
	}
	return "testdata/hh"
}
