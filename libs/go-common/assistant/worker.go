package assistant

import (
	"context"
	"crypto/sha256"
	"log/slog"
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

type WorkerUser struct {
	ID         string
	Preference PreferenceRecord
}

type WorkerCandidate struct {
	ID, Source, ExternalID, Title, SourceURL string
	Vacancy                                  Vacancy
	SalaryText                               string
	ObservedAt                               time.Time
}

type WorkerMatch struct {
	UserID, VacancyID, Source, ExternalID  string
	PreferenceVersion                      int
	Result                                 Result
	Method, Provider, Model, PromptVersion string
	InputSnapshotHash                      []byte
}

type AIStore interface {
	SaveAIResult(context.Context, WorkerMatch, MatchOutput) error
}

type AutomationStore interface {
	AutomationSettings(context.Context, string) (AutomationSettings, error)
}

type TelegramEligibilityStore interface {
	TelegramEligible(context.Context, string) (bool, error)
}

type WorkItemStore interface {
	CompleteWorkItem(context.Context, string, string) error
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
	AIBudget        int
	TelegramEnabled bool
}

type WorkerStats struct {
	Users, Processed, Matched, Notified, Skipped, AICalls int
}

func (s WorkerStats) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Int("users", s.Users), slog.Int("processed", s.Processed),
		slog.Int("matched", s.Matched), slog.Int("notified", s.Notified),
		slog.Int("skipped", s.Skipped), slog.Int("ai_calls", s.AICalls),
	)
}

// RunOnce processes one bounded cursor window. It never falls back to a
// historical full-table scan and exits cleanly when no users exist.
func RunOnce(ctx context.Context, store WorkerStore, opts WorkerOptions) (WorkerStats, error) {
	if opts.BatchSize < 1 || opts.BatchSize > 100 {
		opts.BatchSize = 25
	}
	if opts.Source == "" {
		opts.Source = "hh"
	}
	if opts.Now.IsZero() {
		opts.Now = time.Now().UTC()
	}
	if opts.AIBudget < 1 || opts.AIBudget > 100 {
		opts.AIBudget = 20
	}
	release, acquired, err := store.TryLock(ctx)
	if err != nil || !acquired {
		return WorkerStats{}, err
	}
	defer func() { _ = release() }()
	users, err := store.Users(ctx)
	if err != nil {
		return WorkerStats{}, err
	}
	stats := WorkerStats{Users: len(users)}
	aiCalls := 0
	if len(users) == 0 {
		return stats, nil
	}
	candidates, err := store.Candidates(ctx, opts.Source, opts.Cutoff, opts.BatchSize)
	if err != nil {
		return stats, err
	}
	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return stats, err
		}
		stats.Processed++
		for _, user := range users {
			settings := AutomationSettings{AIEnabled: true, MaxAICallsPerHour: 1}
			if settingsStore, ok := store.(AutomationStore); ok {
				var settingsErr error
				settings, settingsErr = settingsStore.AutomationSettings(ctx, user.ID)
				if settingsErr != nil {
					return stats, settingsErr
				}
			}
			if settings.ActivationAt != nil && candidate.ObservedAt.Before(*settings.ActivationAt) {
				stats.Skipped++
				continue
			}
			result := Match(candidate.Vacancy, toPreferences(user.Preference), opts.Now)
			if result.Decision != DecisionMatch {
				stats.Skipped++
				continue
			}
			created, err := store.SaveMatch(ctx, WorkerMatch{
				UserID: user.ID, VacancyID: candidate.ID, Source: candidate.Source,
				ExternalID: candidate.ExternalID, PreferenceVersion: user.Preference.Version,
				Result: result,
			})
			if err != nil {
				return stats, err
			}
			if !created {
				continue
			}
			stats.Matched++
			if opts.TelegramEnabled && settings.TelegramEnabled {
				eligible := true
				if eligibility, ok := store.(TelegramEligibilityStore); ok {
					eligible, err = eligibility.TelegramEligible(ctx, user.ID)
					if err != nil {
						return stats, err
					}
				}
				if eligible {
					created, err := store.SaveDelivery(ctx, WorkerDelivery{
						UserID: user.ID, VacancyID: candidate.ID,
						PreferenceVersion: user.Preference.Version,
					})
					if err != nil {
						return stats, err
					}
					if created {
						stats.Notified++
					}
				}
			}
			if settings.AIEnabled && opts.AIProvider != nil && aiCalls < opts.AIBudget &&
				(settings.MaxAICallsPerHour <= 0 || aiCalls < settings.MaxAICallsPerHour) &&
				(opts.AIThreshold <= 0 || result.Score >= opts.AIThreshold) {
				evidence := make(map[string]bool, len(result.Evidence))
				for _, id := range result.Evidence {
					evidence[id] = true
				}
				input := MinimizedInput(candidate.Title, candidate.SalaryText, nil, evidence)
				output, err := opts.AIProvider.Complete(ctx, Request{InputSnapshot: input, Evidence: evidence})
				if err != nil {
					return stats, err
				}
				if durable, ok := store.(AIStore); ok {
					aiResult := Result{Decision: Decision(output.Decision), Score: output.Score,
						Reasons: output.Matched, Unknowns: output.Unknowns, Conflicts: output.Conflicts, Evidence: output.Evidence}
					if err := durable.SaveAIResult(ctx, WorkerMatch{
						UserID: user.ID, VacancyID: candidate.ID, PreferenceVersion: user.Preference.Version,
						Result: aiResult, Method: "ai", Provider: "deepseek",
						InputSnapshotHash: sha256Bytes(input),
					}, output); err != nil {
						return stats, err
					}
				}
				aiCalls++
				stats.AICalls++
			}
		}
		if workItems, ok := store.(WorkItemStore); ok {
			if err := workItems.CompleteWorkItem(ctx, candidate.Source, candidate.ExternalID); err != nil {
				return stats, err
			}
		}
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
