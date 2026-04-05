package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"hyperspeed/api/internal/cursor"
)

// Client calls Cursor Cloud Agents API v0 (Basic auth: API key as username, empty password).
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

func (c *Client) base() string {
	return strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
}

func (c *Client) doJSON(ctx context.Context, method, path string, apiKey string, body any, out any) (int, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return 0, cursor.ErrAuth
	}
	u, err := url.Parse(c.base() + path)
	if err != nil {
		return 0, err
	}
	var rdr io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		rdr = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), rdr)
	if err != nil {
		return 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.SetBasicAuth(apiKey, "")

	hc := c.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 120 * time.Second}
	}
	resp, err := hc.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			if ctx.Err() == context.DeadlineExceeded {
				return 0, cursor.ErrTimeout
			}
		}
		if ne, ok := err.(interface{ Timeout() bool }); ok && ne.Timeout() {
			return 0, cursor.ErrTimeout
		}
		return 0, &cursor.ErrUpstream{Msg: err.Error()}
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return resp.StatusCode, cursor.ErrAuth
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return resp.StatusCode, cursor.ErrRateLimit
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(b))
		if len(msg) > 500 {
			msg = msg[:500] + "…"
		}
		return resp.StatusCode, &cursor.ErrUpstream{Status: resp.StatusCode, Msg: msg}
	}
	if out != nil && len(b) > 0 {
		if err := json.Unmarshal(b, out); err != nil {
			return resp.StatusCode, &cursor.ErrUpstream{Status: resp.StatusCode, Msg: "invalid json response"}
		}
	}
	return resp.StatusCode, nil
}

// LaunchInput describes a repo-backed Cloud Agent run.
type LaunchInput struct {
	Prompt        string
	RepositoryURL string
	Ref           string
	Model         string
}

type launchRequest struct {
	Prompt string `json:"prompt"`
	Source struct {
		Repository string `json:"repository"`
		Ref        string `json:"ref,omitempty"`
	} `json:"source"`
	Model string `json:"model,omitempty"`
}

// LaunchResponse is a subset of POST /v0/agents JSON.
type LaunchResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	URL    string `json:"url"`
}

// Launch starts a new agent run.
func (c *Client) Launch(ctx context.Context, apiKey string, in LaunchInput) (LaunchResponse, error) {
	var out LaunchResponse
	in.Prompt = strings.TrimSpace(in.Prompt)
	in.RepositoryURL = strings.TrimSpace(in.RepositoryURL)
	if in.Prompt == "" || in.RepositoryURL == "" {
		return LaunchResponse{}, fmt.Errorf("agents: prompt and repository required")
	}
	var body launchRequest
	body.Prompt = in.Prompt
	body.Source.Repository = in.RepositoryURL
	body.Source.Ref = strings.TrimSpace(in.Ref)
	if body.Source.Ref == "" {
		body.Source.Ref = "main"
	}
	if m := strings.TrimSpace(in.Model); m != "" {
		body.Model = m
	}
	_, err := c.doJSON(ctx, http.MethodPost, "/v0/agents", apiKey, body, &out)
	return out, err
}

// AgentStatusResponse is a subset of GET /v0/agents/{id} JSON.
type AgentStatusResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	URL    string `json:"url"`
	// Some responses nest links
	Target *struct {
		URL string `json:"url"`
	} `json:"target,omitempty"`
}

func (a *AgentStatusResponse) EffectiveURL() string {
	if strings.TrimSpace(a.URL) != "" {
		return a.URL
	}
	if a.Target != nil && strings.TrimSpace(a.Target.URL) != "" {
		return a.Target.URL
	}
	return ""
}

// GetAgent returns current agent status.
func (c *Client) GetAgent(ctx context.Context, apiKey, agentID string) (AgentStatusResponse, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return AgentStatusResponse{}, fmt.Errorf("agents: empty id")
	}
	var out AgentStatusResponse
	path := "/v0/agents/" + url.PathEscape(agentID)
	_, err := c.doJSON(ctx, http.MethodGet, path, apiKey, nil, &out)
	return out, err
}

// ConversationMessage is one entry in GET /v0/agents/{id}/conversation.
type ConversationMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type conversationResponse struct {
	Messages []ConversationMessage `json:"messages"`
}

// GetConversation fetches conversation history (shape may vary; we extract assistant text).
func (c *Client) GetConversation(ctx context.Context, apiKey, agentID string) ([]ConversationMessage, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil, fmt.Errorf("agents: empty id")
	}
	var raw json.RawMessage
	path := "/v0/agents/" + url.PathEscape(agentID) + "/conversation"
	_, err := c.doJSON(ctx, http.MethodGet, path, apiKey, nil, &raw)
	if err != nil {
		return nil, err
	}
	var conv conversationResponse
	if err := json.Unmarshal(raw, &conv); err == nil && len(conv.Messages) > 0 {
		return conv.Messages, nil
	}
	var alt []ConversationMessage
	if err := json.Unmarshal(raw, &alt); err == nil && len(alt) > 0 {
		return alt, nil
	}
	return nil, nil
}

// Terminal returns true when polling should stop.
func Terminal(status string) bool {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "FINISHED", "COMPLETED", "DONE", "FAILED", "ERROR", "CANCELLED", "CANCELED", "STOPPED", "TERMINATED":
		return true
	default:
		return false
	}
}

// PollUntilTerminal polls GetAgent until status is terminal or ctx expires.
func (c *Client) PollUntilTerminal(ctx context.Context, apiKey, agentID string, interval time.Duration) (AgentStatusResponse, error) {
	if interval <= 0 {
		interval = 3 * time.Second
	}
	var last AgentStatusResponse
	for {
		st, err := c.GetAgent(ctx, apiKey, agentID)
		if err != nil {
			return AgentStatusResponse{}, err
		}
		last = st
		if Terminal(st.Status) {
			return last, nil
		}
		select {
		case <-ctx.Done():
			return last, ctx.Err()
		case <-time.After(interval):
		}
	}
}

// SummarizeConversation returns the last non-empty assistant message text, or concatenation.
func SummarizeConversation(msgs []ConversationMessage) string {
	var b strings.Builder
	for _, m := range msgs {
		if strings.ToLower(strings.TrimSpace(m.Role)) != "assistant" {
			continue
		}
		line := strings.TrimSpace(m.Content)
		if line == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(line)
	}
	s := strings.TrimSpace(b.String())
	if s != "" {
		return s
	}
	// Fallback: any message
	for _, m := range msgs {
		line := strings.TrimSpace(m.Content)
		if line != "" {
			return line
		}
	}
	return ""
}
