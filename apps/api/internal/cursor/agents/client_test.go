package agents

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClient_LaunchAndPoll(t *testing.T) {
	var agentID string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || u != "key_test" || p != "" {
			t.Fatalf("basic auth")
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v0/agents":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			agentID = "ag_1"
			_ = json.NewEncoder(w).Encode(map[string]any{"id": agentID, "status": "RUNNING", "url": "https://example.com/run"})
		case r.Method == http.MethodGet && r.URL.Path == "/v0/agents/ag_1":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "ag_1", "status": "FINISHED", "url": "https://example.com/run"})
		case r.Method == http.MethodGet && r.URL.Path == "/v0/agents/ag_1/conversation":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"messages": []map[string]any{
					{"role": "assistant", "content": "done"},
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	c := &Client{BaseURL: ts.URL, HTTPClient: ts.Client()}
	launch, err := c.Launch(context.Background(), "key_test", LaunchInput{
		Prompt:        "fix it",
		RepositoryURL: "https://github.com/o/r",
		Ref:           "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	if launch.ID != "ag_1" {
		t.Fatalf("id %q", launch.ID)
	}
	st, err := c.PollUntilTerminal(context.Background(), "key_test", launch.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if st.Status != "FINISHED" {
		t.Fatalf("status %q", st.Status)
	}
	msgs, err := c.GetConversation(context.Background(), "key_test", launch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if SummarizeConversation(msgs) != "done" {
		t.Fatalf("summary %q", SummarizeConversation(msgs))
	}
}

func TestTerminal(t *testing.T) {
	if !Terminal("finished") || !Terminal("FAILED") {
		t.Fatal("expected terminal")
	}
	if Terminal("running") {
		t.Fatal("not terminal")
	}
}

func TestSummarizeConversationFallback(t *testing.T) {
	s := SummarizeConversation([]ConversationMessage{{Role: "user", Content: "only"}})
	if s != "only" {
		t.Fatalf("got %q", s)
	}
}

func TestClient_LaunchValidation(t *testing.T) {
	c := &Client{BaseURL: "http://x", HTTPClient: http.DefaultClient}
	_, err := c.Launch(context.Background(), "k", LaunchInput{})
	if err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("expected validation, got %v", err)
	}
}
