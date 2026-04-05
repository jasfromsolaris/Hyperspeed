package openrouter

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"hyperspeed/api/internal/cursor"
)

func TestClient_ChatCompletion(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer sk-test" {
			t.Fatalf("auth header")
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": "hello"}},
			},
		})
	}))
	defer ts.Close()

	c := &Client{
		BaseURL:    ts.URL + "/v1",
		ChatPath:   "/chat/completions",
		HTTPClient: ts.Client(),
	}
	out, err := c.ChatCompletion(context.Background(), "sk-test", "x/y", []cursor.Message{{Role: "user", Content: "hi"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out != "hello" {
		t.Fatalf("got %q", out)
	}
}

func TestClient_ChatCompletionAuthError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer ts.Close()

	c := &Client{BaseURL: ts.URL, ChatPath: "/v1/chat/completions", HTTPClient: ts.Client()}
	_, err := c.ChatCompletion(context.Background(), "sk", "m", nil, nil)
	if !errors.Is(err, cursor.ErrAuth) {
		t.Fatalf("expected auth err, got %v", err)
	}
}

func TestClient_ChatCompletionWithToolLoop(t *testing.T) {
	step := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		step++
		if step == 1 {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{{
					"finish_reason": "tool_calls",
					"message": map[string]any{
						"role":    "assistant",
						"content": nil,
						"tool_calls": []map[string]any{{
							"id":   "call_1",
							"type": "function",
							"function": map[string]any{
								"name":      "space.list_files",
								"arguments": `{"space_id":"550e8400-e29b-41d4-a716-446655440000"}`,
							},
						}},
					},
				}},
				"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 2},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"finish_reason": "stop",
				"message":       map[string]any{"role": "assistant", "content": "done"},
			}},
			"usage": map[string]any{"prompt_tokens": 3, "completion_tokens": 4},
		})
	}))
	defer ts.Close()

	c := &Client{
		BaseURL:    ts.URL + "/v1",
		ChatPath:   "/chat/completions",
		HTTPClient: ts.Client(),
	}
	fnTool := json.RawMessage(`{"type":"function","function":{"name":"space.list_files","description":"x","parameters":{"type":"object","properties":{"space_id":{"type":"string"}},"required":["space_id"]}}}`)
	seed := []ChatMessage{{Role: "user", Content: json.RawMessage(`"hi"`)}}
	text, _, err := c.ChatCompletionWithToolLoop(context.Background(), "sk", "m", seed, []json.RawMessage{fnTool}, nil, ToolLoopOptions{MaxIterations: 5, StepTimeout: 5 * time.Second}, func(ctx context.Context, name string, args json.RawMessage) (string, error) {
		if name != "space.list_files" {
			t.Fatalf("unexpected tool %q", name)
		}
		return `{"nodes":[]}`, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if text != "done" {
		t.Fatalf("got %q", text)
	}
	if step != 2 {
		t.Fatalf("expected 2 HTTP rounds, got %d", step)
	}
}

func TestClient_ChatCompletionWithToolLoopFillsTrace(t *testing.T) {
	step := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		step++
		if step == 1 {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{{
					"finish_reason": "tool_calls",
					"message": map[string]any{
						"role":    "assistant",
						"content": nil,
						"tool_calls": []map[string]any{{
							"id":   "call_1",
							"type": "function",
							"function": map[string]any{
								"name":      "space.list_files",
								"arguments": `{"space_id":"550e8400-e29b-41d4-a716-446655440000"}`,
							},
						}},
					},
				}},
				"usage": map[string]any{"prompt_tokens": 1},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"finish_reason": "stop",
				"message":       map[string]any{"role": "assistant", "content": "done"},
			}},
			"usage": map[string]any{"prompt_tokens": 2},
		})
	}))
	defer ts.Close()

	c := &Client{
		BaseURL:    ts.URL + "/v1",
		ChatPath:   "/chat/completions",
		HTTPClient: ts.Client(),
	}
	fnTool := json.RawMessage(`{"type":"function","function":{"name":"space.list_files","description":"x","parameters":{"type":"object","properties":{"space_id":{"type":"string"}},"required":["space_id"]}}}`)
	seed := []ChatMessage{{Role: "user", Content: json.RawMessage(`"hi"`)}}
	trace := &ToolLoopTrace{}
	_, _, err := c.ChatCompletionWithToolLoop(context.Background(), "sk", "m", seed, []json.RawMessage{fnTool}, nil, ToolLoopOptions{
		MaxIterations: 5,
		StepTimeout:   5 * time.Second,
		Trace:         trace,
	}, func(ctx context.Context, name string, args json.RawMessage) (string, error) {
		return `{"nodes":[]}`, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(trace.Steps) != 2 {
		t.Fatalf("expected 2 trace steps (tool round + final), got %d", len(trace.Steps))
	}
	if len(trace.Steps[0].ToolCalls) != 1 || trace.Steps[0].ToolCalls[0].Name != "space.list_files" {
		t.Fatalf("unexpected first step tool calls: %#v", trace.Steps[0].ToolCalls)
	}
	if len(trace.Steps[0].ToolResults) != 1 || trace.Steps[0].ToolResults[0].Output != `{"nodes":[]}` {
		t.Fatalf("unexpected first step tool results: %#v", trace.Steps[0].ToolResults)
	}
	if trace.Steps[1].AssistantText != "done" {
		t.Fatalf("final step text %q", trace.Steps[1].AssistantText)
	}
}

func TestClient_ChatCompletionWithToolLoopPreservesReasoningDetails(t *testing.T) {
	details := json.RawMessage(`[{"type":"reasoning.text","text":"think","format":"anthropic-claude-v1","index":0}]`)
	var detailsSlice []any
	if err := json.Unmarshal(details, &detailsSlice); err != nil {
		t.Fatal(err)
	}
	step := 0
	var secondBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		step++
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if step == 2 {
			secondBody = body
		}
		if step == 1 {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{{
					"finish_reason": "tool_calls",
					"message": map[string]any{
						"role":              "assistant",
						"content":           nil,
						"reasoning_details": detailsSlice,
						"tool_calls": []map[string]any{{
							"id":   "call_1",
							"type": "function",
							"function": map[string]any{
								"name":      "space.list_files",
								"arguments": `{"space_id":"550e8400-e29b-41d4-a716-446655440000"}`,
							},
						}},
					},
				}},
				"usage": map[string]any{"prompt_tokens": 1},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"finish_reason": "stop",
				"message":       map[string]any{"role": "assistant", "content": "done"},
			}},
			"usage": map[string]any{"prompt_tokens": 2},
		})
	}))
	defer ts.Close()

	c := &Client{
		BaseURL:    ts.URL + "/v1",
		ChatPath:   "/chat/completions",
		HTTPClient: ts.Client(),
	}
	fnTool := json.RawMessage(`{"type":"function","function":{"name":"space.list_files","description":"x","parameters":{"type":"object","properties":{"space_id":{"type":"string"}},"required":["space_id"]}}}`)
	seed := []ChatMessage{{Role: "user", Content: json.RawMessage(`"hi"`)}}
	reasoning := json.RawMessage(`{"effort":"high"}`)
	_, _, err := c.ChatCompletionWithToolLoop(context.Background(), "sk", "m", seed, []json.RawMessage{fnTool}, nil, ToolLoopOptions{
		MaxIterations: 5,
		StepTimeout:   5 * time.Second,
		Reasoning:     reasoning,
	}, func(ctx context.Context, name string, args json.RawMessage) (string, error) {
		return `{"nodes":[]}`, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if step != 2 {
		t.Fatalf("expected 2 rounds, got %d", step)
	}
	msgs, ok := secondBody["messages"].([]any)
	if !ok || len(msgs) < 2 {
		t.Fatalf("expected messages in second request, got %#v", secondBody["messages"])
	}
	assistant, ok := msgs[1].(map[string]any)
	if !ok {
		t.Fatalf("assistant message shape")
	}
	rd, ok := assistant["reasoning_details"]
	if !ok {
		t.Fatal("second request should preserve reasoning_details on assistant message")
	}
	rdBytes, err := json.Marshal(rd)
	if err != nil {
		t.Fatal(err)
	}
	var got, want []any
	if err := json.Unmarshal(rdBytes, &got); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(details, &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("reasoning_details changed: %s vs %s", rdBytes, details)
	}
}

func TestClient_ChatCompletionSendsReasoningOpts(t *testing.T) {
	var got map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": "ok"}},
			},
		})
	}))
	defer ts.Close()
	c := &Client{BaseURL: ts.URL, ChatPath: "/v1/chat/completions", HTTPClient: ts.Client()}
	mt := 16000
	_, err := c.ChatCompletion(context.Background(), "k", "m", []cursor.Message{{Role: "user", Content: "x"}}, &ChatCompletionOpts{
		Reasoning: json.RawMessage(`{"effort":"medium"}`),
		MaxTokens: &mt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got["reasoning"] == nil {
		t.Fatal("expected reasoning in request body")
	}
	if got["max_tokens"] == nil || got["max_tokens"].(float64) != 16000 {
		t.Fatalf("max_tokens: %#v", got["max_tokens"])
	}
}
