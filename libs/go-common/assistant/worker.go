package assistant

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"sync"
	"time"
)

// WorkerStore is the durable boundary used by the one-shot worker. The
// PostgreSQL implementation may map these operations to work items, cursors,
// advisory locks and the tables from migrations 012-013.
type WorkerStore interface {
	TryLock(context.Context) (release func() error, acquired bool, err error)
	Users(context.Context) ([]WorkerUser, error)
	Candidates(context.Context, string, time.Time, int) ([]WorkerCandidate, error)
	SaveMatch(context.Context, WorkerMatch) (created bool, err error)
	SaveDelivery(context.Context, WorkerDelivery) (created bool, err error)
	AdvanceCursor(context.Context, string, time.Time, string) error
}

type SnapshotWorkerStore interface {
	SnapshotCandidates(context.Context, AssistantRun, int) ([]WorkerCandidate, error)
	UpdateAssistantRunProgress(context.Context, string, WorkerStats, *WorkerCandidate) error
}

type WorkerUser struct {
	ID         string
	Preference PreferenceRecord
}

type WorkerCandidate struct {
	ID, Source, ExternalID, Title, SourceURL string
	Vacancy                                  Vacancy
	Description                              string
	DescriptionTruncated                     bool
	AIRetry                                  bool
	Revision                                 int
	SalaryText                               string
	ObservedAt                               time.Time
	CreatedAt                                time.Time
}

type WorkerMatch struct {
	UserID, VacancyID, Source, ExternalID  string
	PreferenceVersion                      int
	Result                                 Result
	Method, Provider, Model, PromptVersion string
	InputSnapshotHash                      []byte
	VacancyRevision                        int
}

type AIStore interface {
	SaveAIResult(context.Context, WorkerMatch, MatchOutput) error
	AIResultExists(context.Context, string, int, string, int) (bool, error)
	SaveAIFailure(context.Context, WorkerMatch, string) error
}

type AutomationStore interface {
	AutomationSettings(context.Context, string) (AutomationSettings, error)
}

type TelegramEligibilityStore interface {
	TelegramEligible(context.Context, string) (bool, error)
}

type WorkItemStore interface {
	CompleteWorkItem(context.Context, string, string, int) error
	RetryWorkItem(context.Context, string, string, int, string) error
	DeferWorkItem(context.Context, string, string, int, time.Time) error
}

type WorkerDelivery struct {
	UserID, VacancyID string
	PreferenceVersion int
}

type aiJob struct {
	candidate WorkerCandidate
	user      WorkerUser
	match     WorkerMatch
	request   Request
	shared    string
	settings  AutomationSettings
}

type WorkerOptions struct {
	Source           string
	Cutoff           time.Time
	BatchSize        int
	Now              time.Time
	Log              *slog.Logger
	AIProvider       AIProvider
	AIThreshold      float64
	TelegramEnabled  bool
	MaxSnapshotPages int
}

type WorkerStats struct {
	Users, Processed, Eligible, Matched, Notified, Skipped, AICalls int
	AIEligible, AISucceeded, AIMatches, AIReviews, AIRejects        int
	AIFailures, AISkipped                                           int
	AIHTTPAttempts, AIRetries, AIBatches                            int
	AIPromptTokens, AICompletionTokens, AICachedTokens              int
	AIRateLimit, AITimeouts, AIInvalidResponses, AIAuth             int
	AIQuota, AIServer, AINetwork, AIContextLimit, AIContentFilter   int
	AIInvalidRequest                                                int
	RunID, AIStatus, AISkipReason                                   string
}

func (s WorkerStats) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Int("users", s.Users), slog.Int("processed", s.Processed), slog.Int("eligible", s.Eligible),
		slog.Int("matched", s.Matched), slog.Int("notified", s.Notified),
		slog.Int("skipped", s.Skipped), slog.Int("ai_calls", s.AICalls),
		slog.Int("ai_eligible", s.AIEligible), slog.Int("ai_succeeded", s.AISucceeded),
		slog.Int("ai_matches", s.AIMatches), slog.Int("ai_failures", s.AIFailures),
		slog.Int("ai_skipped", s.AISkipped), slog.String("ai_status", s.AIStatus),
		slog.Int("ai_http_attempts", s.AIHTTPAttempts), slog.Int("ai_retries", s.AIRetries),
		slog.Int("ai_batches", s.AIBatches), slog.Int("ai_prompt_tokens", s.AIPromptTokens),
		slog.Int("ai_completion_tokens", s.AICompletionTokens), slog.Int("ai_cached_tokens", s.AICachedTokens),
		slog.Int("ai_rate_limit", s.AIRateLimit), slog.Int("ai_timeouts", s.AITimeouts),
		slog.Int("ai_invalid_responses", s.AIInvalidResponses),
		slog.String("ai_skip_reason", s.AISkipReason),
	)
}

// RunOnce completes one queued manual snapshot run in bounded keyset batches.
// With no manual run it processes one bounded incremental outbox batch.
func RunOnce(ctx context.Context, store WorkerStore, opts WorkerOptions) (stats WorkerStats, err error) {
	if opts.BatchSize < 1 || opts.BatchSize > 100 {
		opts.BatchSize = 25
	}
	if opts.Source == "" {
		opts.Source = "hh"
	}
	if opts.Now.IsZero() {
		opts.Now = time.Now().UTC()
	}
	release, acquired, err := store.TryLock(ctx)
	if err != nil || !acquired {
		return WorkerStats{}, err
	}
	defer func() { _ = release() }()
	var users []WorkerUser
	var claimedRunID string
	completionCtx := context.WithoutCancel(ctx)
	defer func() {
		if claimedRunID == "" {
			return
		}
		runStore, ok := store.(AssistantRunStore)
		if !ok {
			return
		}
		state, category := "succeeded", ""
		if err != nil {
			state, category = "failed", "worker_failed"
		}
		finalizeAIStats(&stats)
		_ = runStore.CompleteAssistantRun(completionCtx, claimedRunID, state, stats, category)
	}()
	if runStore, ok := store.(AssistantRunStore); ok {
		run, claimed, claimErr := runStore.ClaimAssistantRun(ctx)
		if claimErr != nil {
			return WorkerStats{}, claimErr
		}
		if claimed {
			claimedRunID = run.ID
			var stopHeartbeat func()
			ctx, stopHeartbeat = startRunHeartbeat(ctx, store, run.ID, opts.AIProvider, opts.Log)
			defer stopHeartbeat()
			if scoped, ok := store.(ScopedWorkerStore); ok {
				users, err = scoped.UsersForAssistantRun(ctx, run)
				if err != nil {
					return WorkerStats{}, err
				}
			}
			stats = WorkerStats{
				Users: len(users), RunID: claimedRunID, Processed: run.Processed,
				Eligible: run.Eligible, Matched: run.Matched, AICalls: run.AICalls, Skipped: run.Skipped,
				AIEligible: run.AIEligible, AISucceeded: run.AISucceeded,
				AIMatches: run.AIMatches, AIReviews: run.AIReviews, AIRejects: run.AIRejects,
				AIFailures: run.AIFailures, AISkipped: run.AISkipped,
				AIHTTPAttempts: run.AIHTTPAttempts, AIRetries: run.AIRetries,
				AIBatches: run.AIBatches, AIPromptTokens: run.AIPromptTokens,
				AICompletionTokens: run.AICompletionTokens, AICachedTokens: run.AICachedTokens,
				AIRateLimit: run.AIRateLimit, AITimeouts: run.AITimeouts,
				AIInvalidResponses: run.AIInvalidResponses, AIAuth: run.AIAuth, AIQuota: run.AIQuota,
				AIServer: run.AIServer, AINetwork: run.AINetwork, AIContextLimit: run.AIContextLimit,
				AIContentFilter: run.AIContentFilter, AIInvalidRequest: run.AIInvalidRequest,
				AIStatus: run.AIStatus, AISkipReason: run.AISkipReason,
			}
			if len(users) == 0 {
				return stats, nil
			}
			snapshot, ok := store.(SnapshotWorkerStore)
			if !ok {
				return stats, errors.New("assistant snapshot store is not configured")
			}
			pages := 0
			for {
				candidates, candidateErr := snapshot.SnapshotCandidates(ctx, run, opts.BatchSize)
				if candidateErr != nil {
					return stats, candidateErr
				}
				if len(candidates) == 0 {
					break
				}
				if err = processCandidatesSafely(ctx, store, opts, users, candidates, &stats, true); err != nil {
					return stats, err
				}
				var last *WorkerCandidate
				for i := range candidates {
					if !candidates[i].AIRetry {
						last = &candidates[i]
					}
				}
				if err = snapshot.UpdateAssistantRunProgress(ctx, run.ID, stats, last); err != nil {
					return stats, err
				}
				pages++
				if last != nil {
					run.CursorCreatedAt = &last.CreatedAt
					run.CursorVacancyID = last.ID
				}
				if opts.MaxSnapshotPages > 0 && pages >= opts.MaxSnapshotPages {
					control, ok := store.(AssistantRunControlStore)
					if !ok {
						return stats, errors.New("assistant run control store is not configured")
					}
					if err := control.PauseAssistantRun(ctx, run.ID); err != nil {
						return stats, err
					}
					claimedRunID = ""
					return stats, nil
				}
			}
			return stats, nil
		}
	}
	users, err = store.Users(ctx)
	if err != nil {
		return WorkerStats{}, err
	}
	stats = WorkerStats{Users: len(users)}
	if len(users) == 0 {
		return stats, nil
	}
	candidates, err := store.Candidates(ctx, opts.Source, opts.Cutoff, opts.BatchSize)
	if err != nil {
		return stats, err
	}
	if err = processCandidatesSafely(ctx, store, opts, users, candidates, &stats, false); err != nil {
		return stats, err
	}
	if len(candidates) > 0 {
		last := candidates[len(candidates)-1]
		observedAt := last.ObservedAt
		if observedAt.IsZero() {
			observedAt = opts.Now
		}
		if err := store.AdvanceCursor(ctx, opts.Source, observedAt, last.ExternalID); err != nil {
			return stats, err
		}
	}
	if opts.Log != nil {
		opts.Log.Info("assistant_worker_complete", "stats", stats)
	}
	return stats, nil
}

func startRunHeartbeat(
	ctx context.Context,
	store WorkerStore,
	runID string,
	provider AIProvider,
	log *slog.Logger,
) (context.Context, func()) {
	heartbeat, ok := store.(AssistantRunHeartbeatStore)
	if !ok {
		return ctx, func() {}
	}
	heartbeatCtx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	var activityMu sync.RWMutex
	activity := ProviderActivity{Phase: "processing"}
	if observable, ok := provider.(ObservableAIProvider); ok {
		observable.SetActivityObserver(func(next ProviderActivity) {
			activityMu.Lock()
			activity = next
			activityMu.Unlock()
		})
		deferObserver := observable
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-heartbeatCtx.Done()
			deferObserver.SetActivityObserver(nil)
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			activityMu.RLock()
			current := activity
			activityMu.RUnlock()
			beatCtx, beatCancel := context.WithTimeout(context.WithoutCancel(heartbeatCtx), 5*time.Second)
			err := heartbeat.HeartbeatAssistantRun(
				beatCtx, runID, current.Phase, current.RetryCategory, current.RetryUntil,
				current.ActiveBatches, current.Concurrency,
			)
			beatCancel()
			if err != nil && log != nil {
				log.Warn("assistant_worker_heartbeat_failed", "run_id", runID, "category", "database")
			}
			if err != nil {
				cancel()
				return
			}
			select {
			case <-heartbeatCtx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return heartbeatCtx, func() {
		cancel()
		wg.Wait()
	}
}

func processCandidatesSafely(
	ctx context.Context,
	store WorkerStore,
	opts WorkerOptions,
	users []WorkerUser,
	candidates []WorkerCandidate,
	stats *WorkerStats,
	manual bool,
) (err error) {
	defer func() {
		if recover() != nil {
			if opts.Log != nil {
				opts.Log.Error("assistant_worker_batch_panic", "run_id", stats.RunID, "category", "panic")
			}
			err = errors.New("assistant worker batch panic")
		}
	}()
	return processCandidates(ctx, store, opts, users, candidates, stats, manual)
}

func processCandidates(
	ctx context.Context,
	store WorkerStore,
	opts WorkerOptions,
	users []WorkerUser,
	candidates []WorkerCandidate,
	stats *WorkerStats,
	manual bool,
) error {
	jobs := make([]aiJob, 0, len(candidates))
	candidateFailed := make(map[string]bool, len(candidates))
	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !candidate.AIRetry {
			stats.Processed++
		}
		for _, user := range users {
			settings := AutomationSettings{}
			if settingsStore, ok := store.(AutomationStore); ok {
				var settingsErr error
				settings, settingsErr = settingsStore.AutomationSettings(ctx, user.ID)
				if settingsErr != nil {
					return settingsErr
				}
			}
			if !manual && settings.ActivationAt != nil && candidate.ObservedAt.Before(*settings.ActivationAt) {
				stats.Skipped++
				continue
			}
			if !candidate.AIRetry {
				stats.Eligible++
			}
			result := Match(candidate.Vacancy, toPreferences(user.Preference), opts.Now)
			if candidate.AIRetry {
				// Deterministic state and counters were persisted on the first pass.
			} else if result.Decision == DecisionMatch {
				created, err := store.SaveMatch(ctx, WorkerMatch{
					UserID: user.ID, VacancyID: candidate.ID, Source: candidate.Source,
					ExternalID: candidate.ExternalID, PreferenceVersion: user.Preference.Version,
					VacancyRevision: candidate.Revision, Result: result,
				})
				if err != nil {
					return err
				}
				stats.Matched++
				if created && !settings.AIEnabled && opts.TelegramEnabled && settings.TelegramEnabled {
					eligible := true
					if eligibility, ok := store.(TelegramEligibilityStore); ok {
						eligible, err = eligibility.TelegramEligible(ctx, user.ID)
						if err != nil {
							return err
						}
					}
					if eligible {
						created, err := store.SaveDelivery(ctx, WorkerDelivery{
							UserID: user.ID, VacancyID: candidate.ID,
							PreferenceVersion: user.Preference.Version,
						})
						if err != nil {
							return err
						}
						if created {
							stats.Notified++
						}
					}
				}
			} else {
				stats.Skipped++
			}

			durable, ok := store.(AIStore)
			if !settings.AIEnabled {
				stats.AISkipped++
				setAISkipReason(stats, "user_opt_out")
				continue
			}
			if !candidate.AIRetry {
				stats.AIEligible++
			}
			if opts.AIProvider == nil || !ok {
				stats.AISkipped++
				setAISkipReason(stats, "provider_unavailable")
				continue
			}
			exists, err := durable.AIResultExists(ctx, user.ID, user.Preference.Version, candidate.ID, candidate.Revision)
			if err != nil {
				return err
			}
			if exists {
				stats.AISkipped++
				setAISkipReason(stats, "already_analyzed")
				continue
			}
			evidence := map[string]bool{"vacancy:title": true, "vacancy:description": true, "preferences": true}
			facts := map[string]string{
				"salary":                   candidate.SalaryText,
				"deterministic_decision":   string(result.Decision),
				"deterministic_score":      strconv.FormatFloat(result.Score, 'f', 2, 64),
				"description_truncated":    strconv.FormatBool(candidate.DescriptionTruncated),
				"requested_specialization": stringValue(user.Preference.HardCriteria["specialization"]),
				"include_leadership":       strconv.FormatBool(boolValue(user.Preference.HardCriteria["include_leadership"])),
			}
			input := MinimizedInput(candidate.Title, candidate.Description, facts, evidence)
			shared := "PREFERENCES_JSON:\n" + preferenceSnapshot(user.Preference)
			match := WorkerMatch{
				UserID: user.ID, VacancyID: candidate.ID, PreferenceVersion: user.Preference.Version,
				VacancyRevision: candidate.Revision, Result: result, Method: "ai", Provider: "deepseek",
				PromptVersion: "batch-v4-hard-semantics", InputSnapshotHash: sha256Bytes(shared + "\n" + input),
			}
			jobs = append(jobs, aiJob{
				candidate: candidate, user: user, match: match, shared: shared, settings: settings,
				request: Request{InputSnapshot: input, Evidence: evidence},
			})
		}
	}
	if len(jobs) > 0 {
		if err := processAIJobs(ctx, store, opts, jobs, stats, candidateFailed); err != nil {
			return err
		}
	}
	if !manual {
		if workItems, ok := store.(WorkItemStore); ok {
			for _, candidate := range candidates {
				var itemErr error
				if candidateFailed[candidate.ID] {
					itemErr = workItems.RetryWorkItem(ctx, candidate.Source, candidate.ExternalID,
						candidate.Revision, "ai_provider_failed")
				} else {
					itemErr = workItems.CompleteWorkItem(ctx, candidate.Source, candidate.ExternalID, candidate.Revision)
				}
				if itemErr != nil {
					return itemErr
				}
			}
		}
	}
	return nil
}

func processAIJobs(
	ctx context.Context,
	store WorkerStore,
	opts WorkerOptions,
	jobs []aiJob,
	stats *WorkerStats,
	candidateFailed map[string]bool,
) error {
	recordStats := func(call ProviderCallStats) {
		stats.AIHTTPAttempts += call.HTTPAttempts
		stats.AIRetries += call.Retries
		stats.AIBatches += call.Batches
		stats.AIPromptTokens += call.PromptTokens
		stats.AICompletionTokens += call.CompletionTokens
		stats.AICachedTokens += call.CachedTokens
	}
	finish := func(job aiJob, output MatchOutput, category string) error {
		if !job.candidate.AIRetry {
			stats.AICalls++
		} else {
			stats.AIRetries++
		}
		if category != "" {
			if !job.candidate.AIRetry {
				stats.AIFailures++
			}
			candidateFailed[job.candidate.ID] = true
			countProviderFailure(stats, category)
			if opts.Log != nil {
				opts.Log.Warn("assistant_ai_inference_failed", "run_id", stats.RunID, "category", category)
			}
			return store.(AIStore).SaveAIFailure(ctx, job.match, category)
		}
		if err := store.(AIStore).SaveAIResult(ctx, job.match, output); err != nil {
			return err
		}
		if job.candidate.AIRetry && stats.AIFailures > 0 {
			stats.AIFailures--
		}
		stats.AISucceeded++
		if output.Decision == string(DecisionMatch) {
			stats.AIMatches++
			if opts.TelegramEnabled && job.settings.TelegramEnabled {
				eligible := true
				if eligibility, ok := store.(TelegramEligibilityStore); ok {
					var err error
					eligible, err = eligibility.TelegramEligible(ctx, job.user.ID)
					if err != nil {
						return err
					}
				}
				if eligible {
					created, err := store.SaveDelivery(ctx, WorkerDelivery{
						UserID: job.user.ID, VacancyID: job.candidate.ID,
						PreferenceVersion: job.user.Preference.Version,
					})
					if err != nil {
						return err
					}
					if created {
						stats.Notified++
					}
				}
			}
		} else if output.Decision == string(DecisionReview) {
			stats.AIReviews++
		} else if output.Decision == string(DecisionReject) {
			stats.AIRejects++
		}
		return nil
	}

	if batchProvider, ok := opts.AIProvider.(BatchAIProvider); ok {
		groups := map[string][]aiJob{}
		order := make([]string, 0)
		for _, job := range jobs {
			if _, exists := groups[job.shared]; !exists {
				order = append(order, job.shared)
			}
			groups[job.shared] = append(groups[job.shared], job)
		}
		for _, shared := range order {
			group := groups[shared]
			items := make([]BatchItem, 0, len(group))
			for _, job := range group {
				items = append(items, BatchItem{
					ID: job.candidate.ID, InputSnapshot: job.request.InputSnapshot, Evidence: job.request.Evidence,
				})
			}
			result, err := batchProvider.CompleteBatchDetailed(ctx, BatchRequest{
				SharedPreferences: shared, Items: items,
			})
			recordStats(result.Stats)
			if err != nil {
				return err
			}
			for _, job := range group {
				output, ok := result.Outputs[job.candidate.ID]
				category := result.Errors[job.candidate.ID]
				if !ok && category == "" {
					category = ProviderErrorInvalidResponse
				}
				if err := finish(job, output, category); err != nil {
					return err
				}
			}
		}
		return nil
	}

	for _, job := range jobs {
		request := job.request
		request.InputSnapshot = job.shared + "\n" + request.InputSnapshot
		var output MatchOutput
		call := ProviderCallStats{HTTPAttempts: 1, Batches: 1}
		var err error
		if detailed, ok := opts.AIProvider.(DetailedAIProvider); ok {
			output, call, err = detailed.CompleteDetailed(ctx, request)
			call.Batches = 1
		} else {
			output, err = opts.AIProvider.Complete(ctx, request)
		}
		recordStats(call)
		category := ""
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			category = call.Category
			if category == "" {
				category = providerErrorCategory(err)
			}
		}
		if err := finish(job, output, category); err != nil {
			return err
		}
	}
	return nil
}

func countProviderFailure(stats *WorkerStats, category string) {
	switch category {
	case ProviderErrorRateLimit:
		stats.AIRateLimit++
	case ProviderErrorTimeout:
		stats.AITimeouts++
	case ProviderErrorInvalidResponse:
		stats.AIInvalidResponses++
	case ProviderErrorAuth:
		stats.AIAuth++
	case ProviderErrorQuota:
		stats.AIQuota++
	case ProviderErrorServer:
		stats.AIServer++
	case ProviderErrorNetwork:
		stats.AINetwork++
	case ProviderErrorContextLimit:
		stats.AIContextLimit++
	case ProviderErrorContentFilter:
		stats.AIContentFilter++
	case ProviderErrorInvalidRequest:
		stats.AIInvalidRequest++
	}
}

func setAISkipReason(stats *WorkerStats, reason string) {
	if stats.AISkipReason == "" {
		stats.AISkipReason = reason
	}
}

func finalizeAIStats(stats *WorkerStats) {
	switch {
	case stats.AICalls > 0 && stats.AIFailures == stats.AICalls:
		stats.AIStatus, stats.AISkipReason = "failed", ""
	case stats.AICalls > 0 && (stats.AIFailures > 0 || stats.AISkipped > 0):
		stats.AIStatus, stats.AISkipReason = "partial", ""
	case stats.AICalls > 0:
		stats.AIStatus, stats.AISkipReason = "completed", ""
	case stats.AIStatus == "skipped" && stats.AISkipReason != "":
		// Queue-time reason is authoritative for disabled/opt-out runs.
	case stats.AIEligible == 0:
		stats.AIStatus, stats.AISkipReason = "skipped", "no_eligible"
	default:
		stats.AIStatus = "skipped"
		if stats.AISkipReason == "" {
			stats.AISkipReason = "unknown"
		}
	}
}

func preferenceSnapshot(p PreferenceRecord) string {
	value := map[string]any{"hard_criteria": p.HardCriteria}
	data, _ := json.Marshal(value)
	return string(data)
}

func sha256Bytes(value string) []byte {
	sum := sha256.Sum256([]byte(value))
	return sum[:]
}

func toPreferences(p PreferenceRecord) Preferences {
	return Preferences{
		ApprovedRoles:     stringSlice(p.HardCriteria["approved_roles"]),
		Specialization:    Specialization(stringValue(p.HardCriteria["specialization"])),
		IncludeLeadership: boolValue(p.HardCriteria["include_leadership"]),
		Regions:           stringSlice(p.HardCriteria["regions"]),
		RequiredSkills:    stringSlice(p.HardCriteria["required_skills"]),
		ExcludedSkills:    stringSlice(p.HardCriteria["excluded_skills"]),
		RemoteOnly:        boolValue(p.HardCriteria["remote_only"]),
		MinSalaryRUB:      floatPointer(p.HardCriteria["min_salary_rub"]),
	}
}

func stringValue(value any) string {
	result, _ := value.(string)
	return result
}

func stringSlice(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		if text, ok := item.(string); ok {
			result = append(result, text)
		}
	}
	return result
}

func boolValue(value any) bool {
	result, _ := value.(bool)
	return result
}

func floatPointer(value any) *float64 {
	number, ok := value.(float64)
	if !ok {
		return nil
	}
	return &number
}
