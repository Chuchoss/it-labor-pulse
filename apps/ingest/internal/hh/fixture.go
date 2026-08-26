package hh

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// FixtureSource serves search/detail from local testdata/hh (no network).
type FixtureSource struct {
	Dir string
	// DetailByID optional override map; if nil, uses vacancy_detail.json for 900001
	// and salary_absent-style files when present, else synthesizes from search item.
	DetailByID map[string]string
}

// NewFixtureSource points at testdata/hh directory.
func NewFixtureSource(dir string) *FixtureSource {
	return &FixtureSource{
		Dir: dir,
		DetailByID: map[string]string{
			"900001": "vacancy_detail.json",
			"900002": "vacancy_detail_spb.json",
			"900010": "salary_absent.json",
		},
	}
}

// SearchVacancies returns vacancy_search_page.json (ignores query page for MVP fixtures).
func (f *FixtureSource) SearchVacancies(_ context.Context, q SearchQuery) (SearchPage, error) {
	raw, err := os.ReadFile(filepath.Join(f.Dir, "vacancy_search_page.json"))
	if err != nil {
		return SearchPage{}, fmt.Errorf("hh fixture search: %w", err)
	}
	page, err := ParseSearchPage(raw)
	if err != nil {
		return SearchPage{}, err
	}
	// Simulate pagination: only page 0 has items.
	if q.Page > 0 {
		page.Page = q.Page
		page.Items = nil
		return page, nil
	}
	page.Page = 0
	return page, nil
}

// GetVacancyRaw loads a detail fixture by id.
func (f *FixtureSource) GetVacancyRaw(_ context.Context, id string) ([]byte, error) {
	name := ""
	if f.DetailByID != nil {
		name = f.DetailByID[id]
	}
	if name == "" {
		// Fall back: search-list only vacancy — build minimal JSON from search page.
		return f.detailFromSearch(id)
	}
	raw, err := os.ReadFile(filepath.Join(f.Dir, name))
	if err != nil {
		return nil, fmt.Errorf("hh fixture detail %s: %w", id, err)
	}
	return raw, nil
}

func (f *FixtureSource) detailFromSearch(id string) ([]byte, error) {
	raw, err := os.ReadFile(filepath.Join(f.Dir, "vacancy_search_page.json"))
	if err != nil {
		return nil, err
	}
	page, err := ParseSearchPage(raw)
	if err != nil {
		return nil, err
	}
	for _, it := range page.Items {
		if it.ID == id {
			// Minimal detail body sufficient for DraftFromDetail.
			return fmt.Appendf(nil,
				`{"id":%q,"name":%q,"area":{"id":"0","name":"Unknown"},"salary":null,"employer":{"id":"0","name":"Unknown"},"published_at":"2026-08-10T10:00:00+0300","archived":false,"key_skills":[],"professional_roles":[],"description":""}`,
				it.ID, it.Name,
			), nil
		}
	}
	return nil, fmt.Errorf("hh fixture: vacancy %s not found", id)
}
