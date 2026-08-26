package pipeline

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/Chuchoss/it-labor-pulse/apps/ingest/internal/hh"
)

const ITCategoryID = "11"

// RoleCatalogSource is the official HH taxonomy and vacancy-search surface.
type RoleCatalogSource interface {
	ProfessionalRoles(context.Context, string) ([]hh.ProfessionalRole, error)
	SearchVacancies(context.Context, hh.SearchQuery) (hh.SearchPage, error)
}

// SearchPartition is one disjoint, fetchable HH search leaf.
type SearchPartition struct {
	RoleID string
	Area   string
	From   time.Time
	To     time.Time
	Found  int
	Depth  int
}

// ITPlan is an aggregate-only plan; it contains no vacancy content.
type ITPlan struct {
	Roles            []hh.ProfessionalRole
	Partitions       []SearchPartition
	EstimatedResults int
	ProbeRequests    int
}

// ITPlanOptions bounds recursive planning.
type ITPlanOptions struct {
	Area          string
	CategoryID    string
	Earliest      time.Time
	Now           time.Time
	MaxDepth      int
	MaxPartitions int
	MaxRequests   int
}

// PlanIT partitions every official IT role until each search leaf is below HH's cap.
func PlanIT(ctx context.Context, src RoleCatalogSource, opts ITPlanOptions) (ITPlan, error) {
	if src == nil {
		return ITPlan{}, fmt.Errorf("it planner: source is required")
	}
	if opts.Area == "" {
		opts.Area = "113"
	}
	if opts.CategoryID == "" {
		opts.CategoryID = ITCategoryID
	}
	if opts.Earliest.IsZero() {
		opts.Earliest = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	if opts.Now.IsZero() {
		opts.Now = time.Now().UTC()
	}
	opts.Earliest = opts.Earliest.UTC().Truncate(time.Second)
	opts.Now = opts.Now.UTC().Truncate(time.Second)
	if !opts.Earliest.Before(opts.Now) {
		return ITPlan{}, fmt.Errorf("it planner: earliest must be before now")
	}
	if opts.MaxDepth <= 0 {
		opts.MaxDepth = 32
	}
	if opts.MaxPartitions <= 0 {
		opts.MaxPartitions = 512
	}
	if opts.MaxRequests <= 0 {
		opts.MaxRequests = 1024
	}

	roles, err := src.ProfessionalRoles(ctx, opts.CategoryID)
	if err != nil {
		return ITPlan{}, err
	}
	sort.Slice(roles, func(i, j int) bool { return roles[i].ID < roles[j].ID })
	// Fetching /professional_roles is one official API request.
	plan := ITPlan{Roles: roles, ProbeRequests: 1}
	for _, role := range roles {
		root := SearchPartition{RoleID: role.ID, Area: opts.Area}
		if err := planPartition(ctx, src, opts, &plan, root); err != nil {
			return ITPlan{}, err
		}
	}
	sort.Slice(plan.Partitions, func(i, j int) bool {
		a, b := plan.Partitions[i], plan.Partitions[j]
		if a.RoleID != b.RoleID {
			return a.RoleID < b.RoleID
		}
		return a.From.Before(b.From)
	})
	return plan, nil
}

func planPartition(
	ctx context.Context,
	src RoleCatalogSource,
	opts ITPlanOptions,
	plan *ITPlan,
	part SearchPartition,
) error {
	if plan.ProbeRequests >= opts.MaxRequests {
		return fmt.Errorf("it planner: request safety ceiling %d reached", opts.MaxRequests)
	}
	page, err := src.SearchVacancies(ctx, hh.SearchQuery{
		Area:             part.Area,
		ProfessionalRole: part.RoleID,
		DateFrom:         part.From,
		DateTo:           part.To,
		Page:             0,
		PerPage:          1,
	})
	plan.ProbeRequests++
	if err != nil {
		return err
	}
	part.Found = page.Found
	if page.Found <= hh.MaxSearchResults {
		if len(plan.Partitions) >= opts.MaxPartitions {
			return fmt.Errorf("it planner: partition safety ceiling %d reached", opts.MaxPartitions)
		}
		plan.Partitions = append(plan.Partitions, part)
		plan.EstimatedResults += page.Found
		return nil
	}
	if part.Depth >= opts.MaxDepth {
		return fmt.Errorf("it planner: role %s exceeds cap at max depth %d", part.RoleID, opts.MaxDepth)
	}
	from, to := part.From, part.To
	if from.IsZero() {
		from, to = opts.Earliest, opts.Now
	}
	if !from.Before(to) {
		return fmt.Errorf("it planner: role %s exceeds cap in unsplittable interval", part.RoleID)
	}
	mid := from.Add(to.Sub(from) / 2).UTC().Truncate(time.Second)
	rightFrom := mid.Add(time.Second)
	if rightFrom.After(to) {
		return fmt.Errorf("it planner: role %s exceeds cap in one-second interval", part.RoleID)
	}
	left := SearchPartition{
		RoleID: part.RoleID, Area: part.Area, From: from, To: mid, Depth: part.Depth + 1,
	}
	right := SearchPartition{
		RoleID: part.RoleID, Area: part.Area, From: rightFrom, To: to, Depth: part.Depth + 1,
	}
	if err := planPartition(ctx, src, opts, plan, left); err != nil {
		return err
	}
	return planPartition(ctx, src, opts, plan, right)
}
