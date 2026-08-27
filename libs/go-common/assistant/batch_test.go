package assistant

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestPackBatchItemsUsesCountAndTokenBudget(t *testing.T) {
	if got := PackBatchItems("shared", nil, 5, 1000); len(got) != 0 {
		t.Fatalf("empty input packed into %d batches", len(got))
	}
	items := make([]BatchItem, 5)
	for i := range items {
		items[i] = BatchItem{ID: strconv.Itoa(i), InputSnapshot: "короткое UTF-8 описание"}
	}
	if got := PackBatchItems("shared", items, 5, 10000); len(got) != 1 || len(got[0]) != 5 {
		t.Fatalf("five short items packed as %#v", got)
	}
	items[2].InputSnapshot = strings.Repeat("длинное описание ", 2000)
	got := PackBatchItems("shared", items, 5, 2000)
	if len(got) < 2 {
		t.Fatalf("large UTF-8 item did not force adaptive split: %d", len(got))
	}
	for _, batch := range got {
		if len(batch) > 5 {
			t.Fatalf("max count exceeded: %d", len(batch))
		}
	}
}

func TestCompactVacancyFieldsBoundsAndRedaction(t *testing.T) {
	title, skills, description := CompactVacancyFields(
		strings.Repeat("T", 400),
		[]string{"React.js", "React.js", strings.Repeat("S", 80), "user@example.com"},
		strings.Repeat("D", 5000)+" user@example.com",
	)
	if len([]rune(title)) != compactTitleRunes || len(skills) != 3 || len([]rune(description)) != compactDescRunes {
		t.Fatalf("bounds title=%d skills=%d desc=%d", len([]rune(title)), len(skills), len([]rune(description)))
	}
	if strings.Contains(description, "@") || strings.Contains(strings.Join(skills, " "), "@") {
		t.Fatal("PII leaked into compact vacancy fields")
	}
}

func TestProviderHangIsBoundedAndObservable(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(250 * time.Millisecond)
		_ = json.NewEncoder(w).Encode(map[string]any{})
	}))
	defer server.Close()
	client := server.Client()
	client.Timeout = 40 * time.Millisecond
	provider, err := NewDeepSeek(DeepSeekConfig{
		APIKey: "synthetic", BaseURL: server.URL, Timeout: 40 * time.Millisecond,
		MaxAttempts: 1, MaxBatchSize: 5, MaxTokens: 3500,
	}, client)
	if err != nil {
		t.Fatal(err)
	}
	activities := make(chan ProviderActivity, 2)
	provider.SetActivityObserver(func(activity ProviderActivity) { activities <- activity })
	started := time.Now()
	result, err := provider.CompleteBatchDetailed(context.Background(), BatchRequest{
		Items: []BatchItem{{ID: "v1", Evidence: map[string]bool{}}},
	})
	if err != nil || result.Errors["v1"] != ProviderErrorTimeout {
		t.Fatalf("err=%v result=%+v", err, result)
	}
	if time.Since(started) > 200*time.Millisecond {
		t.Fatal("provider hang exceeded bounded timeout")
	}
	select {
	case activity := <-activities:
		if activity.Phase != "provider_request" {
			t.Fatalf("activity=%+v", activity)
		}
	default:
		t.Fatal("provider request activity was not emitted")
	}
}

func TestRetryDelayIsBounded(t *testing.T) {
	if got := boundedRetryDelay(10*time.Minute, 90*time.Second); got != 30*time.Second {
		t.Fatalf("bounded delay=%s", got)
	}
}

func TestDeepSeekBatchTwentyFiveUsesFiveRequestsAndValidatesIDs(t *testing.T) {
	requests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		ids := decodeBatchVacancyIDs(t, r)
		writeIDListResponse(w, ids, nil, map[string]int{
			"prompt_tokens": 100, "completion_tokens": 20, "prompt_cache_hit_tokens": 40,
		}, "stop")
	}))
	defer server.Close()
	provider, err := NewDeepSeek(DeepSeekConfig{
		APIKey: "synthetic", BaseURL: server.URL, MaxAttempts: 1,
		MaxBatchSize: 5, InputTokenBudget: 24000, MaxTokens: 3500,
	}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	items := make([]BatchItem, 25)
	for i := range items {
		items[i] = BatchItem{
			ID: "item-" + strconv.Itoa(i), InputSnapshot: "VACANCY_DATA_BEGIN\nsynthetic\nVACANCY_DATA_END",
			Evidence: map[string]bool{"vacancy:title": true},
		}
	}
	result, err := provider.CompleteBatchDetailed(context.Background(), BatchRequest{
		SharedPreferences: `{"approved_roles":["96"]}`, Items: items,
	})
	if err != nil || len(result.Outputs) != 25 || len(result.Errors) != 0 {
		t.Fatalf("outputs=%d errors=%v err=%v", len(result.Outputs), result.Errors, err)
	}
	for _, output := range result.Outputs {
		if output.Decision != "review" {
			t.Fatalf("decision=%s, want review", output.Decision)
		}
	}
	if requests != 5 || result.Stats.HTTPAttempts != 5 || result.Stats.PromptTokens != 500 ||
		result.Stats.CompletionTokens != 100 || result.Stats.CachedTokens != 200 {
		t.Fatalf("requests=%d stats=%+v", requests, result.Stats)
	}
}

func TestIDListParseMatchReviewRejectByOmission(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = decodeBatchVacancyIDs(t, r)
		writeJSONResponse(w, map[string]any{
			"match":  []string{"keep"},
			"review": []map[string]string{{"id": "maybe", "reason": "remote_unknown"}},
		}, "stop", map[string]int{"prompt_tokens": 10, "completion_tokens": 5})
	}))
	defer server.Close()
	provider, _ := NewDeepSeek(DeepSeekConfig{
		APIKey: "synthetic", BaseURL: server.URL, MaxAttempts: 1, MaxBatchSize: 5, MaxTokens: 3500,
	}, server.Client())
	result, err := provider.CompleteBatchDetailed(context.Background(), BatchRequest{
		Items: []BatchItem{{ID: "keep"}, {ID: "maybe"}, {ID: "drop"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Outputs["keep"].Decision != "match" || result.Outputs["maybe"].Decision != "review" ||
		result.Outputs["drop"].Decision != "reject" || len(result.Errors) != 0 {
		t.Fatalf("outputs=%+v errors=%v", result.Outputs, result.Errors)
	}
	if result.Outputs["maybe"].Unknowns[0] != "remote_unknown" {
		t.Fatalf("review reason=%v", result.Outputs["maybe"].Unknowns)
	}
}

func TestIDListUnknownIDsIgnoredAndDuplicatesCollapsed(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ids := decodeBatchVacancyIDs(t, r)
		writeJSONResponse(w, map[string]any{
			"match":  []string{ids[0], ids[0], "not-an-input"},
			"review": []map[string]string{{"id": "also-unknown", "reason": "other"}},
		}, "stop", nil)
	}))
	defer server.Close()
	provider, _ := NewDeepSeek(DeepSeekConfig{
		APIKey: "synthetic", BaseURL: server.URL, MaxAttempts: 1, MaxBatchSize: 5, MaxTokens: 3500,
	}, server.Client())
	result, err := provider.CompleteBatchDetailed(context.Background(), BatchRequest{
		Items: []BatchItem{{ID: "a"}, {ID: "b"}},
	})
	if err != nil || result.Outputs["a"].Decision != "match" || result.Outputs["b"].Decision != "reject" ||
		len(result.Outputs) != 2 || len(result.Errors) != 0 {
		t.Fatalf("outputs=%+v errors=%v err=%v", result.Outputs, result.Errors, err)
	}
}

func TestTruncatedIDListRetriesInsteadOfRejecting(t *testing.T) {
	requests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		ids := decodeBatchVacancyIDs(t, r)
		if len(ids) > 1 {
			writeJSONResponse(w, map[string]any{"match": []string{ids[0]}}, "length", nil)
			return
		}
		if len(ids) == 1 {
			writeJSONResponse(w, map[string]any{"match": ids, "review": []any{}}, "stop", nil)
			return
		}
		writeSingletonDecision(w, "match")
	}))
	defer server.Close()
	provider, _ := NewDeepSeek(DeepSeekConfig{
		APIKey: "synthetic", BaseURL: server.URL, MaxAttempts: 1, MaxBatchSize: 5, MaxTokens: 3500,
	}, server.Client())
	result, err := provider.CompleteBatchDetailed(context.Background(), BatchRequest{
		Items: []BatchItem{{ID: "a"}, {ID: "b"}, {ID: "c"}},
	})
	if err != nil || len(result.Outputs) != 3 || len(result.Errors) != 0 || requests < 2 {
		t.Fatalf("requests=%d outputs=%d errors=%v err=%v", requests, len(result.Outputs), result.Errors, err)
	}
	for id, output := range result.Outputs {
		if output.Decision != "match" {
			t.Fatalf("%s silently rejected after truncation: %s", id, output.Decision)
		}
	}
}

func TestMalformedIDListSplitsInsteadOfRejecting(t *testing.T) {
	requests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		ids := decodeBatchVacancyIDs(t, r)
		if len(ids) > 1 {
			writeJSONResponse(w, "not-json", "stop", nil)
			return
		}
		if len(ids) == 1 {
			writeJSONResponse(w, map[string]any{"match": ids, "review": []any{}}, "stop", nil)
			return
		}
		writeSingletonDecision(w, "review")
	}))
	defer server.Close()
	provider, _ := NewDeepSeek(DeepSeekConfig{
		APIKey: "synthetic", BaseURL: server.URL, MaxAttempts: 1, MaxBatchSize: 5, MaxTokens: 3500,
	}, server.Client())
	result, err := provider.CompleteBatchDetailed(context.Background(), BatchRequest{
		Items: []BatchItem{{ID: "a"}, {ID: "b"}},
	})
	if err != nil || len(result.Outputs) != 2 || requests < 2 {
		t.Fatalf("requests=%d outputs=%d err=%v", requests, len(result.Outputs), err)
	}
}

func TestHistoricalFullDecisionBatchStillReadable(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ids := decodeBatchVacancyIDs(t, r)
		decisions := make([]map[string]any, 0, len(ids))
		for i, id := range ids {
			decision := "reject"
			if i == 0 {
				decision = "match"
			} else if i == 1 {
				decision = "review"
			}
			decisions = append(decisions, map[string]any{
				"vacancy_id": id, "decision": decision, "score": .5,
				"confidence": "medium", "evidence_ids": []string{},
				"criterion_evidence": map[string]any{
					"specialization": map[string]any{"pass": true, "source": "title"},
				},
			})
		}
		writeJSONResponse(w, map[string]any{"decisions": decisions}, "stop", nil)
	}))
	defer server.Close()
	provider, _ := NewDeepSeek(DeepSeekConfig{
		APIKey: "synthetic", BaseURL: server.URL, MaxAttempts: 1, MaxBatchSize: 5, MaxTokens: 3500,
	}, server.Client())
	result, err := provider.CompleteBatchDetailed(context.Background(), BatchRequest{
		Items: []BatchItem{
			{ID: "old-match", Evidence: map[string]bool{}},
			{ID: "old-review", Evidence: map[string]bool{}},
			{ID: "old-reject", Evidence: map[string]bool{}},
		},
	})
	if err != nil || result.Outputs["old-match"].Decision != "match" ||
		result.Outputs["old-review"].Decision != "review" ||
		result.Outputs["old-reject"].Decision != "reject" {
		t.Fatalf("historical parse=%+v err=%v", result.Outputs, err)
	}
}

func TestUUIDPromptIDsAreShortOpaqueTokens(t *testing.T) {
	realA := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	realB := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ids := decodeBatchVacancyIDs(t, r)
		if len(ids) != 2 || ids[0] != "v1" || ids[1] != "v2" {
			t.Errorf("prompt ids=%v, want opaque v1/v2", ids)
		}
		for _, id := range ids {
			if strings.Contains(id, "aaaa") || strings.Contains(id, "bbbb") {
				t.Errorf("raw vacancy UUID leaked into prompt: %s", id)
			}
		}
		writeJSONResponse(w, map[string]any{"match": []string{"v1"}, "review": []any{}}, "stop", nil)
	}))
	defer server.Close()
	provider, _ := NewDeepSeek(DeepSeekConfig{
		APIKey: "synthetic", BaseURL: server.URL, MaxAttempts: 1, MaxBatchSize: 5, MaxTokens: 3500,
	}, server.Client())
	result, err := provider.CompleteBatchDetailed(context.Background(), BatchRequest{
		Items: []BatchItem{{ID: realA}, {ID: realB}},
	})
	if err != nil || result.Outputs[realA].Decision != "match" || result.Outputs[realB].Decision != "reject" {
		t.Fatalf("mapped outputs=%+v err=%v", result.Outputs, err)
	}
}

func TestProductionBatchPromptSemanticFixtures(t *testing.T) {
	expected := map[string]string{
		"frontend-ic-remote": "match",
		"backend":            "reject",
		"fullstack":          "reject",
		"lead":               "reject",
		"frontend-ambiguous": "review",
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
			return
		}
		if len(payload.Messages) != 2 || payload.Messages[0].Content != batchSystemPrompt {
			t.Error("production system prompt was not used")
			return
		}
		var user struct {
			Preferences string `json:"preferences"`
			Vacancies   []struct {
				ID string `json:"id"`
			} `json:"vacancies"`
		}
		if err := json.Unmarshal([]byte(payload.Messages[1].Content), &user); err != nil {
			t.Error(err)
			return
		}
		if strings.Contains(user.Preferences, "optional wish") {
			t.Error("optional note leaked into mandatory AI preferences")
		}
		var match []string
		var review []map[string]string
		for _, vacancy := range user.Vacancies {
			switch expected[vacancy.ID] {
			case "match":
				match = append(match, vacancy.ID)
			case "review":
				review = append(review, map[string]string{"id": vacancy.ID, "reason": "other"})
			}
		}
		writeJSONResponse(w, map[string]any{"match": match, "review": review}, "stop", nil)
	}))
	defer server.Close()
	provider, err := NewDeepSeek(DeepSeekConfig{
		APIKey: "synthetic", BaseURL: server.URL, MaxAttempts: 1,
		MaxBatchSize: 50, MaxConcurrency: 5, MaxTokens: 4096,
	}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	items := make([]BatchItem, 0, len(expected))
	for id := range expected {
		items = append(items, BatchItem{
			ID: id, InputSnapshot: "VACANCY_DATA_BEGIN\nsynthetic fixture\nVACANCY_DATA_END",
			Evidence: map[string]bool{"vacancy:title": true},
		})
	}
	result, err := provider.CompleteBatchDetailed(context.Background(), BatchRequest{
		SharedPreferences: `{"hard_criteria":{"specialization":"frontend","include_leadership":false,"remote_only":true}}`,
		Items:             items,
	})
	if err != nil {
		t.Fatal(err)
	}
	for id, decision := range expected {
		if result.Outputs[id].Decision != decision {
			t.Fatalf("%s: got %q, want %q", id, result.Outputs[id].Decision, decision)
		}
	}
}

func TestBatchThroughputSmokeHundredVacancies(t *testing.T) {
	run := func(maxBatch, concurrency int) (requests int64, maxActive int64, elapsed time.Duration) {
		var active atomic.Int64
		var peak atomic.Int64
		var calls atomic.Int64
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			current := active.Add(1)
			defer active.Add(-1)
			for current > peak.Load() && !peak.CompareAndSwap(peak.Load(), current) {
			}
			time.Sleep(15 * time.Millisecond)
			ids := decodeBatchVacancyIDs(t, r)
			writeIDListResponse(w, ids, nil, nil, "stop")
		}))
		defer server.Close()
		provider, _ := NewDeepSeek(DeepSeekConfig{
			APIKey: "synthetic", BaseURL: server.URL, MaxAttempts: 1,
			MaxBatchSize: maxBatch, MaxConcurrency: concurrency, MaxTokens: 40000,
			InputTokenBudget: 200000,
		}, server.Client())
		items := make([]BatchItem, 100)
		for i := range items {
			items[i] = BatchItem{ID: strconv.Itoa(i), Title: "Synthetic", Description: "compact"}
		}
		start := time.Now()
		result, err := provider.CompleteBatchDetailed(context.Background(), BatchRequest{Items: items})
		if err != nil || len(result.Outputs) != 100 {
			t.Fatalf("outputs=%d err=%v", len(result.Outputs), err)
		}
		return calls.Load(), peak.Load(), time.Since(start)
	}
	previousRequests, _, previousElapsed := run(15, 3)
	newRequests, newPeak, newElapsed := run(50, 5)
	if previousRequests != 7 || newRequests != 2 {
		t.Fatalf("requests previous_15x3=%d new_50x5=%d", previousRequests, newRequests)
	}
	if newPeak < 1 || newElapsed >= previousElapsed*2 && newRequests >= previousRequests {
		t.Fatalf("peak=%d elapsed previous=%s new=%s", newPeak, previousElapsed, newElapsed)
	}
	t.Logf("BENCHMARK_100 previous_15x3_requests=%d new_50x5_requests=%d previous_ms=%d new_ms=%d peak_concurrency=%d",
		previousRequests, newRequests, previousElapsed.Milliseconds(), newElapsed.Milliseconds(), newPeak)
}

func TestLoadConfigAIPackingDefaults(t *testing.T) {
	t.Setenv("ASSISTANT_AI_MAX_BATCH_SIZE", "")
	t.Setenv("ASSISTANT_AI_CONCURRENCY", "")
	t.Setenv("ASSISTANT_AI_INPUT_TOKEN_BUDGET", "")
	t.Setenv("ASSISTANT_AI_MAX_OUTPUT_TOKENS", "")
	cfg := LoadConfig()
	if cfg.AIMaxBatchSize != 50 || cfg.AIConcurrency != 5 || cfg.AIInputTokenBudget != 120000 || cfg.AIMaxOutputTokens != 4096 {
		t.Fatalf("defaults=%+v", cfg)
	}
}

func decodeBatchVacancyIDs(t *testing.T, r *http.Request) []string {
	t.Helper()
	var payload struct {
		Messages []struct {
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		return nil
	}
	if len(payload.Messages) < 2 {
		return nil
	}
	var user struct {
		Vacancies []struct {
			ID string `json:"id"`
		} `json:"vacancies"`
	}
	if err := json.Unmarshal([]byte(payload.Messages[1].Content), &user); err != nil {
		return nil
	}
	ids := make([]string, 0, len(user.Vacancies))
	for _, vacancy := range user.Vacancies {
		ids = append(ids, vacancy.ID)
	}
	return ids
}

func writeIDListResponse(w http.ResponseWriter, reviewIDs []string, matchIDs []string, usage map[string]int, finish string) {
	if matchIDs == nil {
		matchIDs = []string{}
	}
	review := make([]map[string]string, 0, len(reviewIDs))
	for _, id := range reviewIDs {
		review = append(review, map[string]string{"id": id, "reason": "other"})
	}
	writeJSONResponse(w, map[string]any{"match": matchIDs, "review": review}, finish, usage)
}

func writeJSONResponse(w http.ResponseWriter, content any, finish string, usage map[string]int) {
	if finish == "" {
		finish = "stop"
	}
	encoded, err := json.Marshal(content)
	if err != nil {
		encoded = []byte(`{"match":[],"review":[]}`)
	}
	body := map[string]any{
		"choices": []any{map[string]any{
			"message": map[string]any{"content": string(encoded)}, "finish_reason": finish,
		}},
	}
	if usage != nil {
		body["usage"] = usage
	}
	_ = json.NewEncoder(w).Encode(body)
}

func writeSingletonDecision(w http.ResponseWriter, decision string) {
	content, _ := json.Marshal(map[string]any{
		"decision": decision, "score": .8, "confidence": "high", "evidence_ids": []string{},
		"criterion_evidence": map[string]any{
			"specialization": map[string]any{"pass": true, "source": "title"},
		},
	})
	_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{
		"message": map[string]any{"content": string(content)}, "finish_reason": "stop",
	}}})
}
