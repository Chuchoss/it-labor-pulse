package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Chuchoss/it-labor-pulse/libs/go-common/assistant"
	"github.com/stretchr/testify/require"
)

type assistantRepositoryFake struct {
	preference assistant.PreferenceRecord
	subject    string
	saves      int
}

func (f *assistantRepositoryFake) EnsureUser(_ context.Context, subject string) (string, error) {
	f.subject = subject
	return "user-1", nil
}
func (f *assistantRepositoryFake) CurrentPreferences(context.Context, string) (assistant.PreferenceRecord, error) {
	return f.preference, nil
}
func (f *assistantRepositoryFake) ListPreferences(context.Context, string) ([]assistant.PreferenceRecord, error) {
	return []assistant.PreferenceRecord{f.preference}, nil
}
func (f *assistantRepositoryFake) SavePreferences(_ context.Context, _, _ string, p assistant.PreferenceRecord) (assistant.PreferenceRecord, error) {
	if _, unsupported := p.HardCriteria["role"]; unsupported {
		return assistant.PreferenceRecord{}, fmt.Errorf("%w: hard_criteria.role is unsupported; use approved_roles", assistant.ErrInvalidPreferences)
	}
	f.saves++
	p.Version = f.saves
	p.ActiveFrom = time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	f.preference = p
	return p, nil
}
func (f *assistantRepositoryFake) ArchivePreference(context.Context, string, string) error {
	return nil
}
func (f *assistantRepositoryFake) AnalysisStatus(context.Context, string, bool) (assistant.AnalysisStatus, error) {
	return assistant.AnalysisStatus{State: "never_run"}, nil
}
func (f *assistantRepositoryFake) QueueAnalysis(context.Context, string, string) (string, error) {
	return "run-1", nil
}
func (f *assistantRepositoryFake) ListMatches(context.Context, string, int) ([]assistant.MatchRecord, error) {
	return []assistant.MatchRecord{}, nil
}
func (f *assistantRepositoryFake) TelegramStatus(context.Context, string, bool) (assistant.TelegramStatus, error) {
	return assistant.TelegramStatus{}, nil
}
func (f *assistantRepositoryFake) AutomationSettings(context.Context, string) (assistant.AutomationSettings, error) {
	return assistant.AutomationSettings{MaxAICallsPerHour: 20}, nil
}
func (f *assistantRepositoryFake) SaveAutomationSettings(_ context.Context, _ string, settings assistant.AutomationSettings) (assistant.AutomationSettings, error) {
	return settings, nil
}
func (f *assistantRepositoryFake) SetTelegramOptIn(context.Context, string, bool) error { return nil }

func TestAssistantPreferencesUseStableDevSubjectAndSupportPatch(t *testing.T) {
	repo := &assistantRepositoryFake{}
	srv := New(Options{Assistant: AssistantOptions{
		Enabled: true, DevAuthEnabled: true, DevSubject: "local-dev-user", Repository: repo,
	}})

	save := httptest.NewRequest(http.MethodPatch, "/api/v1/assistant/preferences",
		strings.NewReader(`{"note":"synthetic profile","hard_criteria":{"approved_roles":["backend"]},"soft_criteria":{},"weights":{"salary":1}}`))
	save.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, save)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "local-dev-user", repo.subject)
	require.Equal(t, 1, repo.saves)

	get := httptest.NewRequest(http.MethodGet, "/api/v1/assistant/preferences", nil)
	rec = httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, get)
	require.Equal(t, http.StatusOK, rec.Code)
	var body assistantPreferencesPayload
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, 1, body.Version)
	require.Equal(t, "synthetic profile", body.Note)

	create := httptest.NewRequest(http.MethodPost, "/api/v1/assistant/preferences",
		strings.NewReader(`{"note":"synthetic profile 2","hard_criteria":{},"soft_criteria":{},"weights":{}}`))
	create.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, create)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 2, repo.saves)
}

func TestAssistantRejectsUnsupportedCriteriaField(t *testing.T) {
	srv := New(Options{Assistant: AssistantOptions{
		Enabled: true, DevAuthEnabled: true, DevSubject: "synthetic",
		Repository: &assistantRepositoryFake{},
	}})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/assistant/preferences",
		strings.NewReader(`{"note":"synthetic","hard_criteria":{"role":"backend"},"soft_criteria":{},"weights":{}}`))
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "hard_criteria.role")
}

func TestAssistantLifecycleEndpointsAreDevGatedAndBounded(t *testing.T) {
	repo := &assistantRepositoryFake{preference: assistant.PreferenceRecord{ID: "pref-1", Version: 1}}
	srv := New(Options{Assistant: AssistantOptions{Enabled: true, DevAuthEnabled: true, DevSubject: "synthetic", Repository: repo}})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/assistant/preferences/list", nil)
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	req = httptest.NewRequest(http.MethodPost, "/api/v1/assistant/analyze", nil)
	rec = httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusAccepted, rec.Code)
	require.Contains(t, rec.Body.String(), `"status":"queued"`)

	req = httptest.NewRequest(http.MethodPost, "/api/v1/assistant/analyze", nil)
	req.Header.Set("X-Dev-User", "")
	rec = httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusAccepted, rec.Code)

	req = httptest.NewRequest(http.MethodGet, "/api/v1/assistant/status", nil)
	req.Header.Set("X-Dev-User", "")
	rec = httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestAssistantErrorsIncludeRequestIDAndCategory(t *testing.T) {
	srv := New(Options{Assistant: AssistantOptions{
		Enabled: true, DevAuthEnabled: true, DevSubject: "synthetic",
		Repository: &assistantRepositoryFake{},
	}})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/assistant/preferences", strings.NewReader("{"))
	req.Header.Set("X-Request-ID", "smoke-request")
	rec := httptest.NewRecorder()

	srv.Handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), `"code":"VALIDATION_ERROR"`)
	require.Contains(t, rec.Body.String(), `"request_id":"smoke-request"`)
}
