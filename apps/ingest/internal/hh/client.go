package hh

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

const (
	// MaxSearchResults is HH's documented maximum depth for one vacancy search query.
	MaxSearchResults = 2000
	// MaxPerPage is HH's documented maximum vacancy search page size.
	MaxPerPage = 100
)

// ErrNotFound means a vacancy disappeared between search and detail fetch.
var ErrNotFound = errors.New("hh vacancy not found")

// Client is an official HH API HTTP client (no scraping).
type Client struct {
	baseURL     string
	userAgent   string
	appToken    string
	httpClient  *http.Client
	sleep       func(context.Context, time.Duration) error
	maxRetries  int
	pageDelay   time.Duration
	maxRequests int64
	requests    atomic.Int64
}

// ClientOptions configures the HH client.
type ClientOptions struct {
	BaseURL    string
	UserAgent  string
	AppToken   string
	HTTPClient *http.Client
	// Sleep is injectable for tests; default uses time.After.
	Sleep      func(context.Context, time.Duration) error
	MaxRetries int
	PageDelay  time.Duration
	// MaxRequests is a hard per-client HTTP-attempt ceiling; zero is unlimited.
	MaxRequests int
}

// NewClient builds a Client. UserAgent must be non-empty (HH ToS).
func NewClient(opts ClientOptions) (*Client, error) {
	ua := strings.TrimSpace(opts.UserAgent)
	if ua == "" {
		return nil, fmt.Errorf("hh client: HH_USER_AGENT is required")
	}
	base := strings.TrimRight(strings.TrimSpace(opts.BaseURL), "/")
	if base == "" {
		base = "https://api.hh.ru"
	}
	hc := opts.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	sleep := opts.Sleep
	if sleep == nil {
		sleep = func(ctx context.Context, d time.Duration) error {
			t := time.NewTimer(d)
			defer t.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-t.C:
				return nil
			}
		}
	}
	retries := opts.MaxRetries
	if retries <= 0 {
		retries = 5
	}
	return &Client{
		baseURL:     base,
		userAgent:   ua,
		appToken:    strings.TrimSpace(opts.AppToken),
		httpClient:  hc,
		sleep:       sleep,
		maxRetries:  retries,
		pageDelay:   opts.PageDelay,
		maxRequests: int64(opts.MaxRequests),
	}, nil
}

// SearchQuery is HH vacancies search params.
type SearchQuery struct {
	Text             string
	Area             string
	ProfessionalRole string
	DateFrom         time.Time
	DateTo           time.Time
	Page             int
	PerPage          int
}

// SearchVacancies calls GET /vacancies.
func (c *Client) SearchVacancies(ctx context.Context, q SearchQuery) (SearchPage, error) {
	if c.pageDelay > 0 {
		if err := c.sleep(ctx, c.pageDelay); err != nil {
			return SearchPage{}, err
		}
	}
	perPage := q.PerPage
	if perPage <= 0 {
		perPage = MaxPerPage
	}
	if perPage > MaxPerPage {
		perPage = MaxPerPage
	}
	if q.Page < 0 {
		return SearchPage{}, fmt.Errorf("hh search: page must not be negative")
	}
	u, err := url.Parse(c.baseURL + "/vacancies")
	if err != nil {
		return SearchPage{}, fmt.Errorf("hh search url: %w", err)
	}
	vals := url.Values{}
	if q.Text != "" {
		vals.Set("text", q.Text)
	}
	if q.Area != "" {
		vals.Set("area", q.Area)
	}
	if q.ProfessionalRole != "" {
		vals.Set("professional_role", q.ProfessionalRole)
	}
	if !q.DateFrom.IsZero() {
		vals.Set("date_from", q.DateFrom.Format(time.RFC3339))
	}
	if !q.DateTo.IsZero() {
		vals.Set("date_to", q.DateTo.Format(time.RFC3339))
	}
	vals.Set("page", strconv.Itoa(q.Page))
	vals.Set("per_page", strconv.Itoa(perPage))
	u.RawQuery = vals.Encode()

	raw, err := c.get(ctx, u.String())
	if err != nil {
		return SearchPage{}, err
	}
	return ParseSearchPage(raw)
}

// ProfessionalRole is one item from HH's official role taxonomy.
type ProfessionalRole struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	SearchDeprecated bool   `json:"search_deprecated"`
}

type professionalRoleCatalog struct {
	Categories []struct {
		ID    string             `json:"id"`
		Name  string             `json:"name"`
		Roles []ProfessionalRole `json:"roles"`
	} `json:"categories"`
}

// ProfessionalRoles returns non-deprecated roles in the requested official category.
func (c *Client) ProfessionalRoles(ctx context.Context, categoryID string) ([]ProfessionalRole, error) {
	raw, err := c.get(ctx, c.baseURL+"/professional_roles")
	if err != nil {
		return nil, err
	}
	var catalog professionalRoleCatalog
	if err := json.Unmarshal(raw, &catalog); err != nil {
		return nil, fmt.Errorf("hh professional roles: %w", err)
	}
	for _, category := range catalog.Categories {
		if category.ID != categoryID {
			continue
		}
		roles := make([]ProfessionalRole, 0, len(category.Roles))
		for _, role := range category.Roles {
			if role.ID != "" && !role.SearchDeprecated {
				roles = append(roles, role)
			}
		}
		return roles, nil
	}
	return nil, fmt.Errorf("hh professional roles: category %q not found", categoryID)
}

// GetVacancyRaw calls GET /vacancies/{id}.
func (c *Client) GetVacancyRaw(ctx context.Context, id string) ([]byte, error) {
	if id == "" {
		return nil, fmt.Errorf("hh get vacancy: empty id")
	}
	if c.pageDelay > 0 {
		if err := c.sleep(ctx, c.pageDelay); err != nil {
			return nil, err
		}
	}
	return c.get(ctx, c.baseURL+"/vacancies/"+url.PathEscape(id))
}

func (c *Client) get(ctx context.Context, rawURL string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if c.maxRequests > 0 && c.requests.Add(1) > c.maxRequests {
			return nil, fmt.Errorf("hh request safety ceiling %d reached", c.maxRequests)
		}
		if attempt > 0 {
			delay := backoffDelay(attempt, 0)
			if err := c.sleep(ctx, delay); err != nil {
				return nil, err
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return nil, fmt.Errorf("hh request: %w", err)
		}
		req.Header.Set("User-Agent", c.userAgent)
		req.Header.Set("Accept", "application/json")
		if c.appToken != "" {
			req.Header.Set("Authorization", "Bearer "+c.appToken)
		}

		res, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("hh http: %w", err)
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(res.Body, 8<<20))
		_ = res.Body.Close()
		if readErr != nil {
			lastErr = fmt.Errorf("hh read body: %w", readErr)
			continue
		}

		switch {
		case res.StatusCode == http.StatusOK:
			return body, nil
		case res.StatusCode == http.StatusTooManyRequests || res.StatusCode >= 500:
			retryAfter := parseRetryAfter(res.Header.Get("Retry-After"))
			if retryAfter > 0 {
				if err := c.sleep(ctx, retryAfter); err != nil {
					return nil, err
				}
			}
			lastErr = fmt.Errorf("hh api status %d", res.StatusCode)
			continue
		case res.StatusCode == http.StatusForbidden:
			return nil, fmt.Errorf("hh api forbidden (check HH_USER_AGENT)")
		case res.StatusCode == http.StatusNotFound:
			return nil, ErrNotFound
		default:
			return nil, fmt.Errorf("hh api status %d", res.StatusCode)
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("hh api: exhausted retries")
	}
	return nil, lastErr
}

func backoffDelay(attempt int, retryAfter time.Duration) time.Duration {
	if retryAfter > 0 {
		return retryAfter
	}
	// 200ms, 400ms, 800ms, … capped at 8s
	d := 200 * time.Millisecond << (attempt - 1)
	if d > 8*time.Second {
		d = 8 * time.Second
	}
	return d
}

func parseRetryAfter(v string) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if sec, err := strconv.Atoi(v); err == nil && sec > 0 {
		return time.Duration(sec) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		d := time.Until(t)
		if d > 0 {
			return d
		}
	}
	return 0
}
