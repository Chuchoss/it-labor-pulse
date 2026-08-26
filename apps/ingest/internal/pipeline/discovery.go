package pipeline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/Chuchoss/it-labor-pulse/apps/ingest/internal/hh"
	"github.com/Chuchoss/it-labor-pulse/apps/ingest/internal/store"
	"github.com/Chuchoss/it-labor-pulse/libs/go-common/normalize"
)

const DiscoveryMethodVersion = "vacancy_demand_v2"

// DiscoveryOptions bounds a complete search-only daily observation cycle.
type DiscoveryOptions struct {
	Area          string
	PerPage       int
	MaxDepth      int
	MaxPartitions int
	MaxRequests   int
	Earliest      time.Time
	CycleDate     time.Time
	CutoffAt      time.Time
	ObservedAt    time.Time
}

// DiscoveryResult summarizes one complete or resumable daily cycle.
type DiscoveryResult struct {
	CycleID         string
	CycleDate       time.Time
	Complete        bool
	PlannedPages    int
	CommittedPages  int
	ObservationRows int
	ProbeRequests   int
}

// RunDailyDiscovery enumerates HH search pages and never calls vacancy detail.
func RunDailyDiscovery(
	ctx context.Context,
	src RoleCatalogSource,
	st store.DiscoveryStore,
	log *slog.Logger,
	opts DiscoveryOptions,
) (DiscoveryResult, error) {
	if src == nil || st == nil {
		return DiscoveryResult{}, fmt.Errorf("daily discovery: source and store are required")
	}
	if log == nil {
		log = slog.Default()
	}
	if opts.Area == "" {
		opts.Area = "113"
	}
	if opts.PerPage < 1 || opts.PerPage > hh.MaxPerPage {
		return DiscoveryResult{}, fmt.Errorf("daily discovery: per page must be between 1 and %d", hh.MaxPerPage)
	}
	if opts.ObservedAt.IsZero() {
		opts.ObservedAt = time.Now().UTC()
	}
	opts.ObservedAt = opts.ObservedAt.UTC().Truncate(time.Second)
	if opts.CutoffAt.IsZero() {
		opts.CutoffAt = dayUTC(opts.ObservedAt)
	}
	opts.CutoffAt = opts.CutoffAt.UTC().Truncate(time.Second)
	if opts.CycleDate.IsZero() {
		opts.CycleDate = opts.CutoffAt.AddDate(0, 0, -1)
	}
	opts.CycleDate = dayUTC(opts.CycleDate)
	if opts.Earliest.IsZero() {
		opts.Earliest = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	}

	plan, err := PlanIT(ctx, src, ITPlanOptions{
		Area: opts.Area, Earliest: opts.Earliest, Now: opts.CutoffAt,
		MaxDepth: opts.MaxDepth, MaxPartitions: opts.MaxPartitions,
		MaxRequests: opts.MaxRequests,
	})
	if err != nil {
		return DiscoveryResult{}, fmt.Errorf("daily discovery plan: %w", err)
	}
	partitions := make([]store.DiscoveryPartition, 0, len(plan.Partitions))
	expectedPages := 0
	completedPartitions := 0
	for _, planned := range plan.Partitions {
		pages := 0
		if planned.Found > 0 {
			pages = (planned.Found + opts.PerPage - 1) / opts.PerPage
		}
		part := store.DiscoveryPartition{
			Key: partitionKey(planned), ProfessionalRoleID: planned.RoleID,
			Area: planned.Area, DateFrom: planned.From, DateTo: nonZeroTo(planned.To, opts.CutoffAt),
			ExpectedPages: pages,
		}
		if pages == 0 {
			part.Status = "complete"
			completedPartitions++
		}
		partitions = append(partitions, part)
		expectedPages += pages
	}
	scopeHash := discoveryScopeHash(opts.Area, opts.PerPage)
	cycleID, err := st.StartDiscoveryCycle(ctx, store.Cycle{
		Source: hh.SourceCode, Scope: "daily_discovery", ScopeHash: scopeHash,
		CycleDate: opts.CycleDate, CycleEnd: opts.CutoffAt, CutoffAt: opts.CutoffAt,
		PartitionCount: len(partitions), CompletedPartitions: completedPartitions,
		ExpectedPages: expectedPages,
		MethodVersion: DiscoveryMethodVersion,
	}, partitions)
	if err != nil {
		return DiscoveryResult{}, fmt.Errorf("daily discovery start: %w", err)
	}
	result := DiscoveryResult{
		CycleID: cycleID, CycleDate: opts.CycleDate,
		PlannedPages: expectedPages, ProbeRequests: plan.ProbeRequests,
	}
	log.Info("discovery_cycle_started",
		"cycle_id", cycleID,
		"cycle_date", opts.CycleDate.Format(time.DateOnly),
		"cutoff_at", opts.CutoffAt.Format(time.RFC3339),
		"partitions", len(partitions),
		"planned_pages", expectedPages,
		"probe_requests", plan.ProbeRequests,
		"method_version", DiscoveryMethodVersion,
	)

	normalizeOpts := normalize.DefaultOptions()
	for {
		part, found, err := st.NextDiscoveryPartition(ctx, cycleID)
		if err != nil {
			return result, fmt.Errorf("daily discovery resume: %w", err)
		}
		if !found {
			break
		}
		page, err := src.SearchVacancies(ctx, hh.SearchQuery{
			Area: part.Area, ProfessionalRole: part.ProfessionalRoleID,
			DateFrom: nonZeroFrom(part.DateFrom, opts.Earliest), DateTo: part.DateTo,
			Page: part.NextPage, PerPage: opts.PerPage,
		})
		if err != nil {
			return result, fmt.Errorf("daily discovery search: %w", err)
		}
		reportedPages := min(page.Pages, (hh.MaxSearchResults+opts.PerPage-1)/opts.PerPage)
		if reportedPages != part.ExpectedPages {
			if part.NextPage != 0 {
				return result, fmt.Errorf(
					"daily discovery: partition page count changed after page zero",
				)
			}
			if err := st.SetDiscoveryExpectedPages(
				ctx, cycleID, part.Key, reportedPages,
			); err != nil {
				return result, fmt.Errorf("daily discovery reconcile pages: %w", err)
			}
			log.Info("discovery_partition_pages_reconciled",
				"cycle_id", cycleID,
				"partition_key", part.Key,
				"planned_pages", part.ExpectedPages,
				"reported_pages", reportedPages,
			)
			result.PlannedPages += reportedPages - part.ExpectedPages
			part.ExpectedPages = reportedPages
			if reportedPages == 0 {
				continue
			}
		}
		observations := make([]store.DiscoveryObservation, 0, len(page.Items))
		for _, item := range page.Items {
			observation, include, err := observationFromSearch(item, opts.ObservedAt, normalizeOpts)
			if err != nil {
				return result, err
			}
			if include {
				observations = append(observations, observation)
			}
		}
		if err := st.SaveDiscoveryPage(ctx, cycleID, part, observations); err != nil {
			return result, fmt.Errorf("daily discovery save page: %w", err)
		}
		result.CommittedPages++
		result.ObservationRows += len(observations)
		log.Info("discovery_page_committed",
			"cycle_id", cycleID,
			"partition_key", part.Key,
			"page", part.NextPage,
			"items", len(page.Items),
			"observations", len(observations),
		)
	}
	if err := st.CompleteDiscoveryCycle(ctx, cycleID); err != nil {
		return result, err
	}
	result.Complete = true
	log.Info("discovery_cycle_completed",
		"cycle_id", cycleID,
		"cycle_date", opts.CycleDate.Format(time.DateOnly),
		"committed_pages", result.CommittedPages,
		"method_version", DiscoveryMethodVersion,
	)
	return result, nil
}

func observationFromSearch(
	item hh.SearchItem,
	observedAt time.Time,
	opts normalize.Options,
) (store.DiscoveryObservation, bool, error) {
	draft, err := hh.DraftFromSearch(item, observedAt)
	if err != nil {
		return store.DiscoveryObservation{}, false, err
	}
	roles := hh.CollectedProfessionalRoles(draft.ProfessionalRoleIDs)
	primary, include := hh.AllowedProfessionalRole(draft.ProfessionalRoleIDs)
	if !include {
		return store.DiscoveryObservation{}, false, nil
	}
	draft.ProfessionalRoleIDs = []string{primary.ID}
	normalized, err := normalize.Normalize(draft, opts)
	if err != nil {
		return store.DiscoveryObservation{}, false, fmt.Errorf("daily discovery normalize: %w", err)
	}
	externalRoleIDs := make([]string, 0, len(roles))
	for _, role := range roles {
		externalRoleIDs = append(externalRoleIDs, role.ID)
	}
	sort.Strings(externalRoleIDs)
	var salaryMid *float64
	eligible := !normalized.Vacancy.ExcludeFromSalaryAgg &&
		normalized.Vacancy.SalaryMidRub != nil
	if eligible {
		value := *normalized.Vacancy.SalaryMidRub
		salaryMid = &value
	}
	return store.DiscoveryObservation{
		ExternalID: item.ID, PublishedAt: draft.PublishedAt,
		ExternalRegionID: draft.RegionExternalID, ExternalRegionName: draft.RegionName,
		PrimaryRoleExternalID: primary.ID, RoleGroup: primary.Group,
		ExternalRoleIDs: externalRoleIDs,
		SalaryFrom:      normalized.Vacancy.SalaryFrom, SalaryTo: normalized.Vacancy.SalaryTo,
		SalaryCurrency:  normalized.Vacancy.SalaryCurrency,
		SalaryGross:     normalized.Vacancy.SalaryGross,
		SalaryMidRubNet: salaryMid, SalaryEligible: eligible,
		ObservedAt: observedAt,
	}, true, nil
}

func partitionKey(part SearchPartition) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf(
		"%s|%s|%s|%s", part.RoleID, part.Area,
		part.From.UTC().Format(time.RFC3339), part.To.UTC().Format(time.RFC3339),
	)))
	return hex.EncodeToString(sum[:])
}

func discoveryScopeHash(area string, perPage int) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf(
		"hh|daily-discovery-v2|%s|%d", area, perPage,
	)))
	return hex.EncodeToString(sum[:])
}

func nonZeroTo(value, fallback time.Time) time.Time {
	if value.IsZero() {
		return fallback
	}
	return value
}

func nonZeroFrom(value, fallback time.Time) time.Time {
	if value.IsZero() {
		return fallback
	}
	return value
}

func dayUTC(value time.Time) time.Time {
	y, m, d := value.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
