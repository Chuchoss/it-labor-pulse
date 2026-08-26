package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Chuchoss/it-labor-pulse/libs/go-common/assistant"
)

type AssistantOptions struct {
	Enabled            bool
	DevAuthEnabled     bool
	DevSubject         string
	Repository         AssistantRepository
	TelegramConfigured bool
}

type AssistantRepository interface {
	EnsureUser(context.Context, string) (string, error)
	CurrentPreferences(context.Context, string) (assistant.PreferenceRecord, error)
	SavePreferences(context.Context, string, string, assistant.PreferenceRecord) (assistant.PreferenceRecord, error)
	ListMatches(context.Context, string, int) ([]assistant.MatchRecord, error)
	TelegramStatus(context.Context, string, bool) (assistant.TelegramStatus, error)
}

type assistantPreferencesPayload struct {
	Version      int                `json:"version"`
	Note         string             `json:"note"`
	HardCriteria map[string]any     `json:"hard_criteria"`
	SoftCriteria map[string]any     `json:"soft_criteria"`
	Weights      map[string]float64 `json:"weights"`
	ActiveFrom   *time.Time         `json:"active_from,omitempty"`
}

type assistantHandler struct {
	opts               AssistantOptions
	mu                 sync.Mutex
	currentPreferences assistantPreferencesPayload
	linker             *assistant.Linker
}

func newAssistantHandler(opts AssistantOptions) *assistantHandler {
	return &assistantHandler{opts: opts, linker: assistant.NewLinker(10 * time.Minute)}
}

func (h *assistantHandler) register(mux *http.ServeMux) {
	if !h.opts.Enabled {
		return
	}
	mux.HandleFunc("/api/v1/assistant/preferences", h.preferences)
	mux.HandleFunc("/api/v1/assistant/matches", h.matches)
	mux.HandleFunc("/api/v1/assistant/telegram/link", h.link)
	mux.HandleFunc("/api/v1/assistant/telegram", h.telegram)
}

func (h *assistantHandler) authorized(r *http.Request) bool {
	if !h.opts.DevAuthEnabled {
		return false
	}
	return h.subject(r) != ""
}

func (h *assistantHandler) subject(r *http.Request) string {
	subject := strings.TrimSpace(r.Header.Get("X-Dev-User"))
	if subject == "" {
		subject = strings.TrimSpace(h.opts.DevSubject)
	}
	return subject
}

func (h *assistantHandler) user(ctx context.Context, r *http.Request) (string, error) {
	if !h.authorized(r) {
		return "", errors.New("unauthorized")
	}
	subject := h.subject(r)
	if h.opts.Repository == nil {
		return subject, nil
	}
	return h.opts.Repository.EnsureUser(ctx, subject)
}

func (h *assistantHandler) guard(w http.ResponseWriter, r *http.Request) bool {
	if !h.authorized(r) {
		writeAPIError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required", nil, "")
		return false
	}
	return true
}

func (h *assistantHandler) preferences(w http.ResponseWriter, r *http.Request) {
	if !h.guard(w, r) {
		return
	}
	userID, err := h.user(r.Context(), r)
	if err != nil {
		writeAPIError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required", nil, "")
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if r.Method == http.MethodGet {
		if h.opts.Repository != nil {
			value, err := h.opts.Repository.CurrentPreferences(r.Context(), userID)
			if err != nil {
				writeAPIError(w, 500, "INTERNAL_ERROR", "Could not read preferences", nil, "")
				return
			}
			writeJSON(w, http.StatusOK, preferencePayload(value))
			return
		}
		writeJSON(w, http.StatusOK, h.currentPreferences)
		return
	}
	if r.Method != http.MethodPut && r.Method != http.MethodPatch && r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var value assistantPreferencesPayload
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024)).Decode(&value) != nil {
		writeAPIError(w, 400, "VALIDATION_ERROR", "Invalid JSON", nil, "")
		return
	}
	if h.opts.Repository != nil {
		value, err := h.opts.Repository.SavePreferences(r.Context(), userID, r.Header.Get("Idempotency-Key"), assistant.PreferenceRecord{
			Note: value.Note, HardCriteria: value.HardCriteria, SoftCriteria: value.SoftCriteria, Weights: value.Weights,
		})
		if err != nil {
			if errors.Is(err, assistant.ErrInvalidPreferences) {
				writeAPIError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid preferences", nil, "")
			} else {
				writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not save preferences", nil, "")
			}
			return
		}
		writeJSON(w, http.StatusOK, preferencePayload(value))
		return
	}
	value.Version++
	h.currentPreferences = value
	writeJSON(w, http.StatusOK, value)
}

func (h *assistantHandler) matches(w http.ResponseWriter, r *http.Request) {
	if !h.guard(w, r) {
		return
	}
	userID, err := h.user(r.Context(), r)
	if err != nil {
		writeAPIError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required", nil, "")
		return
	}
	if h.opts.Repository == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	matches, err := h.opts.Repository.ListMatches(r.Context(), userID, 100)
	if err != nil {
		writeAPIError(w, 500, "INTERNAL_ERROR", "Could not read matches", nil, "")
		return
	}
	writeJSON(w, http.StatusOK, matches)
}
func (h *assistantHandler) link(w http.ResponseWriter, r *http.Request) {
	if !h.guard(w, r) {
		return
	}
	token, err := h.linker.Issue(time.Now())
	if err != nil {
		writeAPIError(w, 500, "INTERNAL_ERROR", "Could not create link", nil, "")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deep_link": "https://t.me/lma_assistant_bot?start=" + token, "expires_at": time.Now().Add(10 * time.Minute)})
}
func (h *assistantHandler) telegram(w http.ResponseWriter, r *http.Request) {
	if !h.guard(w, r) {
		return
	}
	if r.Method == http.MethodDelete {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	userID, err := h.user(r.Context(), r)
	if err != nil {
		writeAPIError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required", nil, "")
		return
	}
	if h.opts.Repository == nil {
		writeJSON(w, http.StatusOK, map[string]bool{"configured": h.opts.TelegramConfigured, "linked": false, "opted_in": false})
		return
	}
	status, err := h.opts.Repository.TelegramStatus(r.Context(), userID, h.opts.TelegramConfigured)
	if err != nil {
		writeAPIError(w, 500, "INTERNAL_ERROR", "Could not read Telegram status", nil, "")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"configured": status.Configured, "linked": status.Linked, "opted_in": status.OptedIn})
}

func preferencePayload(value assistant.PreferenceRecord) assistantPreferencesPayload {
	return assistantPreferencesPayload{
		Version: value.Version, Note: value.Note, HardCriteria: value.HardCriteria,
		SoftCriteria: value.SoftCriteria, Weights: value.Weights, ActiveFrom: &value.ActiveFrom,
	}
}
