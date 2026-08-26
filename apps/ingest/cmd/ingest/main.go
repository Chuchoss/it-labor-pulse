package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/Chuchoss/it-labor-pulse/apps/ingest/internal/checkpoint"

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
	scope := flag.String("scope", "", "ingest scope: query or it (default INGEST_SCOPE)")
	dryRun := flag.Bool("dry-run", false, "plan only; do not fetch vacancy details or write vacancies")
	area := flag.String("area", "", "HH area id (default INGEST_DEFAULT_AREA)")
	text := flag.String("text", "", "HH search text (default INGEST_DEFAULT_TEXT)")
	maxPages := flag.Int("max-pages", -1, "max pages; 0 means all available (default INGEST_MAX_PAGES)")
	perPage := flag.Int("per-page", 0, "HH page size, 1..100 (default INGEST_PER_PAGE)")
	flag.Parse()

	cfg := config.Load()
	if *scope != "" {
		cfg.Scope = *scope
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
	var client *hh.Client
	if cfg.FixtureDir != "" {
		log.Info("ingest_fixture_mode", "dir", cfg.FixtureDir)
		src = hh.NewFixtureSource(cfg.FixtureDir)
	} else {
		maxRequests := 0
		if cfg.Scope == "it" {
			maxRequests = cfg.ITMaxReqs
		}
		client, err = hh.NewClient(hh.ClientOptions{
			BaseURL:     cfg.HHBaseURL,
			UserAgent:   cfg.HHUserAgent,
			AppToken:    cfg.HHAppToken,
			PageDelay:   cfg.PageDelay,
			MaxRequests: maxRequests,
		})
		if err != nil {
			log.Error("hh_client_failed", "err", err.Error())
			os.Exit(1)
		}
		src = client
	}

	if cfg.Scope == "it" {
		if client == nil {
			log.Error("it_scope_requires_live_hh")
			os.Exit(1)
		}
		runCtx, cancelRun := context.WithTimeout(ctx, cfg.RunTimeout)
		defer cancelRun()
		if err := runIT(runCtx, cfg, client, st, log, *dryRun); err != nil {
			log.Error("it_ingest_failed", "err", err.Error())
			os.Exit(1)
		}
		return
	}

	p := pipeline.Params{
		Area:        cfg.DefaultArea,
		Text:        cfg.DefaultText,
		Mode:        "incremental",
		MaxPages:    cfg.MaxPages,
		PerPage:     cfg.PerPage,
		RequestedBy: "cli",
	}
	if *area != "" {
		p.Area = *area
	}
	if *text != "" {
		p.Text = *text
	}
	if *maxPages >= 0 {
		p.MaxPages = *maxPages
	}
	if *perPage > 0 {
		p.PerPage = *perPage
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

func runIT(
	ctx context.Context,
	cfg config.Config,
	client *hh.Client,
	st store.Store,
	log *slog.Logger,
	dryRun bool,
) error {
	now := time.Now().UTC().Truncate(time.Second)
	plan, err := pipeline.PlanIT(ctx, client, pipeline.ITPlanOptions{
		Area:          cfg.ITArea,
		Now:           now,
		MaxDepth:      cfg.ITMaxDepth,
		MaxPartitions: cfg.ITMaxParts,
		MaxRequests:   cfg.ITMaxReqs,
	})
	if err != nil {
		return err
	}
	log.Info("it_plan_ready",
		"roles", len(plan.Roles),
		"partitions", len(plan.Partitions),
		"estimated_results", plan.EstimatedResults,
		"probe_requests", plan.ProbeRequests,
		"request_ceiling", cfg.ITMaxReqs,
	)
	if dryRun {
		return nil
	}

	sourceRoles := make([]store.SourceRole, 0, len(plan.Roles))
	for _, role := range plan.Roles {
		sourceRoles = append(sourceRoles, store.SourceRole{
			ExternalID: role.ID, Title: role.Name, Family: "it",
		})
	}
	roleIDs, err := st.SyncRoles(ctx, hh.SourceCode, sourceRoles)
	if err != nil {
		return err
	}
	opts := normalize.DefaultOptions()
	opts.Roles = normalize.MapRoleMatcher{
		RoleByExternalID: map[string]map[string]string{hh.SourceCode: roleIDs},
	}

	planScope := checkpoint.ScopeHash(
		hh.SourceCode, "it-plan", cfg.ITArea, "", cfg.PerPage,
		now.Format(time.DateOnly),
	)
	cursor, _, err := st.GetCheckpoint(ctx, hh.SourceCode, planScope)
	if err != nil {
		return err
	}
	start, err := checkpoint.ParseCursorPage(cursor)
	if err != nil || start > len(plan.Partitions) {
		return fmt.Errorf("it plan checkpoint: invalid partition cursor")
	}
	remainingRequests := cfg.ITMaxReqs - plan.ProbeRequests
	total := store.Stats{}
	completedParts := 0
	for index := start; index < len(plan.Partitions); index++ {
		part := plan.Partitions[index]
		pages := (part.Found + cfg.PerPage - 1) / cfg.PerPage
		if pages == 0 {
			pages = 1
		}
		affordablePages := remainingRequests / (cfg.PerPage + 1)
		if part.Found == 0 {
			affordablePages = 1
		}
		if affordablePages < 1 {
			break
		}
		maxPages := min(pages, affordablePages)
		runner := &pipeline.Runner{Source: client, Store: st, Log: log, Opts: opts}
		result, runErr := runner.Run(ctx, pipeline.Params{
			Area:             part.Area,
			ProfessionalRole: part.RoleID,
			DateFrom:         part.From,
			DateTo:           part.To,
			Mode:             "incremental",
			MaxPages:         maxPages,
			PerPage:          cfg.PerPage,
			RequestedBy:      "cli-it",
		})
		total.Fetched += result.Stats.Fetched
		total.Upserted += result.Stats.Upserted
		total.Unchanged += result.Stats.Unchanged
		total.Errors += result.Stats.Errors
		total.Pages += result.Stats.Pages
		remainingRequests -= result.Stats.Fetched + result.Stats.Pages
		if runErr != nil {
			return runErr
		}
		if !result.Completed {
			break
		}
		completedParts++
		next := index + 1
		if next == len(plan.Partitions) {
			next = 0
		}
		if _, _, err := st.SavePage(ctx, hh.SourceCode, planScope, strconv.Itoa(next), nil); err != nil {
			return err
		}
	}
	log.Info("it_execution_finished",
		"completed_partitions", completedParts,
		"remaining_partitions", len(plan.Partitions)-start-completedParts,
		"pages", total.Pages,
		"fetched", total.Fetched,
		"upserted", total.Upserted,
		"unchanged", total.Unchanged,
		"errors", total.Errors,
	)
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
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
