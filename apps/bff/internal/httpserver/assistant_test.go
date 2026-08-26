package httpserver

import (
	"context"
	"encoding/json"
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
func (f *assistantRepositoryFake) SavePreferences(_ context.Context, _, _ string, p assistant.PreferenceRecord) (assistant.PreferenceRecord, error) {
	f.saves++
	p.Version = f.saves
	p.ActiveFrom = time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	f.preference = p
	return p, nil
}
func (f *assistantRepositoryFake) ListMatches(context.Context, string, int) ([]assistant.MatchRecord, error) {
	return []assistant.MatchRecord{}, nil
}
func (f *assistantRepositoryFake) TelegramStatus(context.Context, string, bool) (assistant.TelegramStatus, error) {
	return assistant.TelegramStatus{}, nil
}

func TestAssistantPreferencesUseStableDevSubjectAndSupportPatch(t *testing.T) {
	repo := &assistantRepositoryFake{}
	srv := New(Options{Assistant: AssistantOptions{
		Enabled: true, DevAuthEnabled: true, DevSubject: "local-dev-user", Repository: repo,
	}})

	save := httptest.NewRequest(http.MethodPatch, "/api/v1/assistant/preferences",
		strings.NewReader(`{"note":"synthetic profile","hard_criteria":{"role":"backend"},"soft_criteria":{},"weights":{"salary":1}}`))
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
