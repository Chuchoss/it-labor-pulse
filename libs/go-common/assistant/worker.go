package assistant

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
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

type WorkerOptions struct {
	Source          string
	Cutoff          time.Time
	BatchSize       int
	Now             time.Time
	Log             *slog.Logger
	AIProvider      AIProvider
	AIThreshold     float64
	TelegramEnabled bool
}

type WorkerStats struct {
	Users, Processed, Eligible, Matched, Notified, Skipped, AICalls int
	AIEligible, AISucceeded, AIMatches, AIFailures, AISkipped       int
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
				AIMatches: run.AIMatches, AIFailures: run.AIFailures, AISkipped: run.AISkipped,
				AIStatus: run.AIStatus, AISkipReason: run.AISkipReason,
			}
			if len(users) == 0 {
				return stats, nil
			}
			snapshot, ok := store.(SnapshotWorkerStore)
			if !ok {
				return stats, errors.New("assistant snapshot store is not configured")
			}
			for {
				candidates, candidateErr := snapshot.SnapshotCandidates(ctx, run, opts.BatchSize)
				if candidateErr != nil {
					return stats, candidateErr
				}
				if len(candidates) == 0 {
					break
				}
				if err = processCandidates(ctx, store, opts, users, candidates, &stats, true); err != nil {
					return stats, err
				}
				last := candidates[len(candidates)-1]
				if err = snapshot.UpdateAssistantRunProgress(ctx, run.ID, stats, &last); err != nil {
					return stats, err
				}
				run.CursorCreatedAt = &last.CreatedAt
				run.CursorVacancyID = last.ID
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
	if err = processCandidates(ctx, store, opts, users, candidates, &stats, false); err != nil {
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

func processCandidates(
	ctx context.Context,
	store WorkerStore,
	opts WorkerOptions,
	users []WorkerUser,
	candidates []WorkerCandidate,
	stats *WorkerStats,
	manual bool,
) error {
	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return err
		}
		stats.Processed++
		candidateFailed := false
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
			stats.Eligible++
			result := Match(candidate.Vacancy, toPreferences(user.Preference), opts.Now)
			if result.Decision == DecisionMatch {
				created, err := store.SaveMatch(ctx, WorkerMatch{
					UserID: user.ID, VacancyID: candidate.ID, Source: candidate.Source,
					ExternalID: candidate.ExternalID, PreferenceVersion: user.Preference.Version,
					VacancyRevision: candidate.Revision, Result: result,
				})
				if err != nil {
					return err
				}
				stats.Matched++
				if created && opts.TelegramEnabled && settings.TelegramEnabled {
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
			stats.AIEligible++
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
				"salary":                 candidate.SalaryText,
				"deterministic_decision": string(result.Decision),
				"deterministic_score":    strconv.FormatFloat(result.Score, 'f', 2, 64),
				"preferences":            preferenceSnapshot(user.Preference),
				"description_truncated":  strconv.FormatBool(candidate.DescriptionTruncated),
			}
			input := MinimizedInput(candidate.Title, candidate.Description, facts, evidence)
			match := WorkerMatch{
				UserID: user.ID, VacancyID: candidate.ID, PreferenceVersion: user.Preference.Version,
				VacancyRevision: candidate.Revision, Result: result, Method: "ai", Provider: "deepseek",
				PromptVersion: "description-v1", InputSnapshotHash: sha256Bytes(input),
			}
			output, providerErr := opts.AIProvider.Complete(ctx, Request{InputSnapshot: input, Evidence: evidence})
			stats.AICalls++
			if providerErr != nil {
				stats.AIFailures++
				candidateFailed = true
				if err := durable.SaveAIFailure(ctx, match, "provider_failed"); err != nil {
					return err
				}
				continue
			}
			if err := durable.SaveAIResult(ctx, match, output); err != nil {
				return err
			}
			stats.AISucceeded++
			if output.Decision == string(DecisionMatch) {
				stats.AIMatches++
			}
		}
		if !manual {
			workItems, ok := store.(WorkItemStore)
			if ok {
				var itemErr error
				if candidateFailed {
					itemErr = workItems.RetryWorkItem(
						ctx, candidate.Source, candidate.ExternalID, candidate.Revision, "ai_provider_failed",
					)
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
	value := map[string]any{"note": p.Note, "hard_criteria": p.HardCriteria}
	data, _ := json.Marshal(value)
	return string(data)
}

func sha256Bytes(value string) []byte {
	sum := sha256.Sum256([]byte(value))
	return sum[:]
}

func toPreferences(p PreferenceRecord) Preferences {
	return Preferences{
		ApprovedRoles:  stringSlice(p.HardCriteria["approved_roles"]),
		Regions:        stringSlice(p.HardCriteria["regions"]),
		RequiredSkills: stringSlice(p.HardCriteria["required_skills"]),
		ExcludedSkills: stringSlice(p.HardCriteria["excluded_skills"]),
		RemoteOnly:     boolValue(p.HardCriteria["remote_only"]),
		MinSalaryRUB:   floatPointer(p.HardCriteria["min_salary_rub"]),
	}
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
