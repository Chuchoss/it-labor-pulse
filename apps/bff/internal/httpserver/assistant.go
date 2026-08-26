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
	"github.com/Chuchoss/it-labor-pulse/libs/go-common/httpx"
)

type AssistantOptions struct {
	Enabled             bool
	DevAuthEnabled      bool
	DevSubject          string
	Repository          AssistantRepository
	TelegramConfigured  bool
	AIConfigured        bool
	TelegramBotUsername string
	TelegramSender      assistant.DeliveryTelegramClient
}

type AssistantRepository interface {
	EnsureUser(context.Context, string) (string, error)
	CurrentPreferences(context.Context, string) (assistant.PreferenceRecord, error)
	ListPreferences(context.Context, string) ([]assistant.PreferenceRecord, error)
	SavePreferences(context.Context, string, string, assistant.PreferenceRecord) (assistant.PreferenceRecord, error)
	ArchivePreference(context.Context, string, string) error
	AnalysisStatus(context.Context, string, bool) (assistant.AnalysisStatus, error)
	QueueAnalysis(context.Context, string, string) (string, error)
	ListMatches(context.Context, string, int) ([]assistant.MatchRecord, error)
	TelegramStatus(context.Context, string, bool) (assistant.TelegramStatus, error)
	AutomationSettings(context.Context, string) (assistant.AutomationSettings, error)
	SaveAutomationSettings(context.Context, string, assistant.AutomationSettings) (assistant.AutomationSettings, error)
	SetTelegramOptIn(context.Context, string, bool) error
}

type telegramLinkRepository interface {
	IssueTelegramLink(context.Context, string, string, time.Time) error
	RevokeTelegram(context.Context, string) error
}
type telegramChatRepository interface {
	VerifiedTelegramChat(context.Context, string) (int64, error)
}

type assistantPreferencesPayload struct {
	ID           string             `json:"id,omitempty"`
	Version      int                `json:"version"`
	Note         string             `json:"note"`
	HardCriteria map[string]any     `json:"hard_criteria"`
	SoftCriteria map[string]any     `json:"soft_criteria"`
	Weights      map[string]float64 `json:"weights"`
	ActiveFrom   *time.Time         `json:"active_from,omitempty"`
	ArchivedAt   *time.Time         `json:"archived_at,omitempty"`
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
	mux.HandleFunc("/api/v1/assistant/preferences/list", h.preferenceList)
	mux.HandleFunc("/api/v1/assistant/preferences/archive", h.archivePreference)
	mux.HandleFunc("/api/v1/assistant/status", h.status)
	mux.HandleFunc("/api/v1/assistant/analyze", h.analyze)
	mux.HandleFunc("/api/v1/assistant/matches", h.matches)
	mux.HandleFunc("/api/v1/assistant/telegram/link", h.link)
	mux.HandleFunc("/api/v1/assistant/telegram", h.telegram)
	mux.HandleFunc("/api/v1/assistant/telegram/test", h.telegramTest)
	mux.HandleFunc("/api/v1/assistant/automation", h.automation)
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
		h.error(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required", nil)
		return false
	}
	return true
}

func (h *assistantHandler) error(w http.ResponseWriter, r *http.Request, status int, code, message string, details map[string]any) {
	writeAPIError(w, status, code, message, details, httpx.RequestID(r.Context()))
}

func (h *assistantHandler) preferenceList(w http.ResponseWriter, r *http.Request) {
	if !h.guard(w, r) {
		return
	}
	userID, err := h.user(r.Context(), r)
	if err != nil {
		h.error(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required", nil)
		return
	}
	if h.opts.Repository == nil {
		writeJSON(w, http.StatusOK, []assistantPreferencesPayload{h.currentPreferences})
		return
	}
	items, err := h.opts.Repository.ListPreferences(r.Context(), userID)
	if err != nil {
		h.error(w, r, 500, "INTERNAL_ERROR", "Could not read preferences", nil)
		return
	}
	result := make([]assistantPreferencesPayload, 0, len(items))
	for _, item := range items {
		result = append(result, preferencePayload(item))
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *assistantHandler) archivePreference(w http.ResponseWriter, r *http.Request) {
	if !h.guard(w, r) {
		return
	}
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userID, err := h.user(r.Context(), r)
	if err != nil {
		h.error(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required", nil)
		return
	}
	var body struct {
		ID string `json:"id"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4*1024)).Decode(&body) != nil || strings.TrimSpace(body.ID) == "" {
		h.error(w, r, 400, "VALIDATION_ERROR", "Preference id is required", nil)
		return
	}
	if h.opts.Repository != nil {
		if err := h.opts.Repository.ArchivePreference(r.Context(), userID, body.ID); err != nil {
			h.error(w, r, http.StatusNotFound, "NOT_FOUND", "Preference not found", nil)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *assistantHandler) status(w http.ResponseWriter, r *http.Request) {
	if !h.guard(w, r) {
		return
	}
	userID, err := h.user(r.Context(), r)
	if err != nil {
		h.error(w, r, 401, "UNAUTHORIZED", "Authentication is required", nil)
		return
	}
	if h.opts.Repository == nil {
		writeJSON(w, 200, assistant.AnalysisStatus{State: "disabled", MethodVersion: "deterministic-v1"})
		return
	}
	value, err := h.opts.Repository.AnalysisStatus(r.Context(), userID, h.opts.AIConfigured)
	if err != nil {
		h.error(w, r, 500, "INTERNAL_ERROR", "Could not read analysis status", nil)
		return
	}
	writeJSON(w, 200, value)
}

func (h *assistantHandler) analyze(w http.ResponseWriter, r *http.Request) {
	if !h.guard(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userID, err := h.user(r.Context(), r)
	if err != nil {
		h.error(w, r, 401, "UNAUTHORIZED", "Authentication is required", nil)
		return
	}
	if h.opts.Repository == nil {
		h.error(w, r, 503, "DEPENDENCY_UNAVAILABLE", "Analysis store is not configured", nil)
		return
	}
	runID, err := h.opts.Repository.QueueAnalysis(r.Context(), userID, r.Header.Get("Idempotency-Key"))
	if err != nil {
		h.error(w, r, 409, "CONFLICT", "Анализ уже выполняется", nil)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"run_id": runID, "status": "queued"})
}

func (h *assistantHandler) preferences(w http.ResponseWriter, r *http.Request) {
	if !h.guard(w, r) {
		return
	}
	userID, err := h.user(r.Context(), r)
	if err != nil {
		h.error(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required", nil)
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if r.Method == http.MethodGet {
		if h.opts.Repository != nil {
			value, err := h.opts.Repository.CurrentPreferences(r.Context(), userID)
			if err != nil {
				h.error(w, r, 500, "INTERNAL_ERROR", "Could not read preferences", nil)
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
		h.error(w, r, 400, "VALIDATION_ERROR", "Invalid JSON", nil)
		return
	}
	if h.opts.Repository != nil {
		value, err := h.opts.Repository.SavePreferences(r.Context(), userID, r.Header.Get("Idempotency-Key"), assistant.PreferenceRecord{
			Note: value.Note, HardCriteria: value.HardCriteria, SoftCriteria: value.SoftCriteria, Weights: value.Weights,
		})
		if err != nil {
			if errors.Is(err, assistant.ErrInvalidPreferences) {
				h.error(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid preferences", nil)
			} else {
				h.error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not save preferences", nil)
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
		h.error(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required", nil)
		return
	}
	if h.opts.Repository == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	matches, err := h.opts.Repository.ListMatches(r.Context(), userID, 100)
	if err != nil {
		h.error(w, r, 500, "INTERNAL_ERROR", "Could not read matches", nil)
		return
	}
	writeJSON(w, http.StatusOK, matches)
}
func (h *assistantHandler) link(w http.ResponseWriter, r *http.Request) {
	if !h.guard(w, r) {
		return
	}
	userID, err := h.user(r.Context(), r)
	if err != nil {
		h.error(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required", nil)
		return
	}
	token, err := h.linker.Issue(time.Now())
	if err != nil {
		h.error(w, r, 500, "INTERNAL_ERROR", "Could not create link", nil)
		return
	}
	expires := time.Now().Add(10 * time.Minute)
	if repo, ok := h.opts.Repository.(telegramLinkRepository); ok {
		if err := repo.IssueTelegramLink(r.Context(), userID, token, expires); err != nil {
			h.error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not create link", nil)
			return
		}
	}
	bot := h.opts.TelegramBotUsername
	if bot == "" {
		bot = "lma_assistant_bot"
	}
	writeJSON(w, http.StatusOK, map[string]any{"deep_link": "https://t.me/" + bot + "?start=" + token, "expires_at": expires})
}
func (h *assistantHandler) telegram(w http.ResponseWriter, r *http.Request) {
	if !h.guard(w, r) {
		return
	}
	if r.Method == http.MethodDelete {
		if userID, err := h.user(r.Context(), r); err == nil {
			if repo, ok := h.opts.Repository.(telegramLinkRepository); ok {
				_ = repo.RevokeTelegram(r.Context(), userID)
			}
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	userID, err := h.user(r.Context(), r)
	if err != nil {
		h.error(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required", nil)
		return
	}
	if r.Method == http.MethodPatch {
		var body struct {
			OptedIn bool `json:"opted_in"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4*1024)).Decode(&body) != nil {
			h.error(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid JSON", nil)
			return
		}
		if h.opts.Repository != nil {
			if err := h.opts.Repository.SetTelegramOptIn(r.Context(), userID, body.OptedIn); err != nil {
				h.error(w, r, http.StatusConflict, "TELEGRAM_NOT_VERIFIED", "Telegram must be linked and verified first", nil)
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]bool{"opted_in": body.OptedIn})
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if h.opts.Repository == nil {
		writeJSON(w, http.StatusOK, map[string]bool{"configured": h.opts.TelegramConfigured, "linked": false, "opted_in": false})
		return
	}
	status, err := h.opts.Repository.TelegramStatus(r.Context(), userID, h.opts.TelegramConfigured)
	if err != nil {
		h.error(w, r, 500, "INTERNAL_ERROR", "Could not read Telegram status", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"configured": status.Configured, "linked": status.Linked, "opted_in": status.OptedIn,
		"pending": status.Pending, "sent": status.Sent, "failed": status.Failed,
		"dead_lettered": status.DeadLettered, "last_error": status.LastError})
}

func (h *assistantHandler) telegramTest(w http.ResponseWriter, r *http.Request) {
	if !h.guard(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Confirm bool `json:"confirm"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&body) != nil || !body.Confirm {
		h.error(w, r, http.StatusBadRequest, "CONFIRMATION_REQUIRED", "Тестовое уведомление требует явного подтверждения", nil)
		return
	}
	userID, err := h.user(r.Context(), r)
	if err != nil {
		h.error(w, r, 401, "UNAUTHORIZED", "Authentication is required", nil)
		return
	}
	if h.opts.TelegramSender == nil || !h.opts.TelegramConfigured {
		h.error(w, r, http.StatusServiceUnavailable, "TELEGRAM_NOT_CONFIGURED", "Telegram sender is not configured", nil)
		return
	}
	status, err := h.opts.Repository.TelegramStatus(r.Context(), userID, h.opts.TelegramConfigured)
	if err != nil {
		h.error(w, r, 500, "INTERNAL_ERROR", "Could not read Telegram status", nil)
		return
	}
	if !status.Linked || !status.OptedIn {
		h.error(w, r, http.StatusConflict, "TELEGRAM_NOT_VERIFIED", "Сначала свяжите Telegram и включите уведомления", nil)
		return
	}
	repo, ok := h.opts.Repository.(telegramChatRepository)
	if !ok {
		h.error(w, r, http.StatusServiceUnavailable, "TEST_SEND_UNAVAILABLE", "Тестовая отправка недоступна", nil)
		return
	}
	chatID, err := repo.VerifiedTelegramChat(r.Context(), userID)
	if err != nil {
		h.error(w, r, http.StatusConflict, "TELEGRAM_NOT_VERIFIED", "Сначала свяжите Telegram и включите уведомления", nil)
		return
	}
	result, err := h.opts.TelegramSender.SendMessageResult(r.Context(), chatID,
		"<b>Тестовое уведомление LMA</b>\nСвязь подтверждена; автоматические уведомления не включаются.")
	if err != nil {
		h.error(w, r, http.StatusBadGateway, "TELEGRAM_SEND_FAILED", "Не удалось отправить тестовое уведомление", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"provider_message_id": result.MessageID})
}

func (h *assistantHandler) automation(w http.ResponseWriter, r *http.Request) {
	if !h.guard(w, r) {
		return
	}
	userID, err := h.user(r.Context(), r)
	if err != nil {
		h.error(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required", nil)
		return
	}
	if h.opts.Repository == nil {
		writeJSON(w, http.StatusOK, assistant.AutomationSettings{MaxAICallsPerHour: 20})
		return
	}
	if r.Method == http.MethodGet {
		settings, err := h.opts.Repository.AutomationSettings(r.Context(), userID)
		if err != nil {
			h.error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not read automation settings", nil)
			return
		}
		writeJSON(w, http.StatusOK, settings)
		return
	}
	if r.Method != http.MethodPatch {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		AIEnabled       *bool `json:"ai_enabled"`
		TelegramEnabled *bool `json:"telegram_enabled"`
		MaxAICalls      int   `json:"max_ai_calls_per_hour"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4*1024)).Decode(&body) != nil {
		h.error(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid JSON", nil)
		return
	}
	current, err := h.opts.Repository.AutomationSettings(r.Context(), userID)
	if err != nil {
		h.error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not read automation settings", nil)
		return
	}
	if body.AIEnabled != nil {
		if *body.AIEnabled && !h.opts.AIConfigured {
			h.error(w, r, http.StatusConflict, "AI_NOT_CONFIGURED", "AI provider is not configured on the server", nil)
			return
		}
		current.AIEnabled = *body.AIEnabled
	}
	if body.TelegramEnabled != nil {
		if *body.TelegramEnabled && !h.opts.TelegramConfigured {
			h.error(w, r, http.StatusConflict, "TELEGRAM_NOT_CONFIGURED", "Telegram is not configured on the server", nil)
			return
		}
		current.TelegramEnabled = *body.TelegramEnabled
	}
	if body.MaxAICalls != 0 {
		current.MaxAICallsPerHour = body.MaxAICalls
	}
	saved, err := h.opts.Repository.SaveAutomationSettings(r.Context(), userID, current)
	if err != nil {
		h.error(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid automation settings", nil)
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

func preferencePayload(value assistant.PreferenceRecord) assistantPreferencesPayload {
	return assistantPreferencesPayload{
		ID: value.ID, Version: value.Version, Note: value.Note, HardCriteria: value.HardCriteria,
		SoftCriteria: value.SoftCriteria, Weights: value.Weights, ActiveFrom: &value.ActiveFrom, ArchivedAt: value.ArchivedAt,
	}
}
