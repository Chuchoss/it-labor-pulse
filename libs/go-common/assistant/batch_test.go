package assistant

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
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

func TestDeepSeekBatchTwentyFiveUsesFiveRequestsAndValidatesIDs(t *testing.T) {
	requests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		var payload struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		var user struct {
			Vacancies []struct {
				ID string `json:"vacancy_id"`
			} `json:"vacancies"`
		}
		if err := json.Unmarshal([]byte(payload.Messages[1].Content), &user); err != nil {
			t.Fatal(err)
		}
		decisions := make([]map[string]any, 0, len(user.Vacancies))
		for _, item := range user.Vacancies {
			decisions = append(decisions, map[string]any{
				"vacancy_id": item.ID, "decision": "review", "score": .5,
				"confidence": "medium", "evidence_ids": []string{"vacancy:title"},
			})
		}
		content, _ := json.Marshal(map[string]any{"decisions": decisions})
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{
				"message": map[string]any{"content": string(content)}, "finish_reason": "stop",
			}},
			"usage": map[string]int{"prompt_tokens": 100, "completion_tokens": 20, "prompt_cache_hit_tokens": 40},
		})
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
			ID: "v" + strconv.Itoa(i), InputSnapshot: "VACANCY_DATA_BEGIN\nsynthetic\nVACANCY_DATA_END",
			Evidence: map[string]bool{"vacancy:title": true},
		}
	}
	result, err := provider.CompleteBatchDetailed(context.Background(), BatchRequest{
		SharedPreferences: `{"approved_roles":["96"]}`, Items: items,
	})
	if err != nil || len(result.Outputs) != 25 || len(result.Errors) != 0 {
		t.Fatalf("outputs=%d errors=%v err=%v", len(result.Outputs), result.Errors, err)
	}
	if requests != 5 || result.Stats.HTTPAttempts != 5 || result.Stats.PromptTokens != 500 ||
		result.Stats.CompletionTokens != 100 || result.Stats.CachedTokens != 200 {
		t.Fatalf("requests=%d stats=%+v", requests, result.Stats)
	}
}

func TestDeepSeekBatchSplitsDuplicateUnknownAndMissingOutputs(t *testing.T) {
	for _, mode := range []string{"duplicate", "unknown", "missing"} {
		t.Run(mode, func(t *testing.T) {
			requests := 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				var payload struct {
					Messages []struct {
						Content string `json:"content"`
					} `json:"messages"`
				}
				_ = json.NewDecoder(r.Body).Decode(&payload)
				var user struct {
					Vacancies []struct {
						ID string `json:"vacancy_id"`
					} `json:"vacancies"`
				}
				_ = json.Unmarshal([]byte(payload.Messages[1].Content), &user)
				if len(user.Vacancies) == 0 {
					_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{
						"message":       map[string]any{"content": `{"decision":"match","score":0.8,"confidence":"high","evidence_ids":[]}`},
						"finish_reason": "stop",
					}}})
					return
				}
				ids := make([]string, 0, len(user.Vacancies))
				for _, item := range user.Vacancies {
					ids = append(ids, item.ID)
				}
				if len(ids) > 1 {
					switch mode {
					case "duplicate":
						ids = []string{ids[0], ids[0]}
					case "unknown":
						ids = []string{"not-an-input"}
					case "missing":
						ids = ids[:1]
					}
				}
				decisions := make([]map[string]any, 0, len(ids))
				for _, id := range ids {
					decisions = append(decisions, map[string]any{
						"vacancy_id": id, "decision": "match", "score": .8,
						"confidence": "high", "evidence_ids": []string{},
					})
				}
				content, _ := json.Marshal(map[string]any{"decisions": decisions})
				_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{
					"message": map[string]any{"content": string(content)}, "finish_reason": "stop",
				}}})
			}))
			defer server.Close()
			provider, _ := NewDeepSeek(DeepSeekConfig{
				APIKey: "synthetic", BaseURL: server.URL, MaxAttempts: 1, MaxBatchSize: 5, MaxTokens: 3500,
			}, server.Client())
			result, err := provider.CompleteBatchDetailed(context.Background(), BatchRequest{
				SharedPreferences: `{}`,
				Items:             []BatchItem{{ID: "a"}, {ID: "b"}, {ID: "c"}},
			})
			if err != nil || len(result.Outputs) != 3 || len(result.Errors) != 0 || requests < 2 {
				t.Fatalf("mode=%s requests=%d outputs=%d errors=%v err=%v",
					mode, requests, len(result.Outputs), result.Errors, err)
			}
		})
	}
}
