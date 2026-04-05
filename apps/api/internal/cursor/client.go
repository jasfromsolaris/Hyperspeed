package cursor

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
)

// Message is an OpenAI-style chat message for completion requests.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Client calls a chat-completions-compatible HTTP endpoint.
type Client struct {
	BaseURL    string
	ChatPath   string
	HTTPClient *http.Client
	Model      string
	// HTTPAuth: "bearer" (default) or "basic" (RFC 7617: API key as username, empty password).
	HTTPAuth string
}

func (c *Client) setAuthorization(req *http.Request, apiKey string) {
	switch strings.ToLower(strings.TrimSpace(c.HTTPAuth)) {
	case "basic":
		req.SetBasicAuth(apiKey, "")
	default:
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
}

func (c *Client) effectiveModel() string {
	m := strings.TrimSpace(c.Model)
	if m == "" {
		return "auto"
	}
	return m
}

func (c *Client) joinURL() (string, error) {
	base := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if base == "" {
		return "", errors.New("cursor client: empty base url")
	}
	path := strings.TrimSpace(c.ChatPath)
	if path == "" {
		path = "/v1/chat/completions"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	u, err := url.Parse(base + path)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

type chatCompletionRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// ChatCompletion returns the assistant text from choices[0].message.content.
func (c *Client) ChatCompletion(ctx context.Context, apiKey string, messages []Message) (string, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return "", fmt.Errorf("cursor client: %w", ErrAuth)
	}
	endpoint, err := c.joinURL()
	if err != nil {
		return "", err
	}
	hc := c.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 90 * time.Second}
	}
	body := chatCompletionRequest{
		Model:    c.effectiveModel(),
		Messages: messages,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	c.setAuthorization(req, apiKey)

	resp, err := hc.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			if ctx.Err() == context.DeadlineExceeded {
				return "", ErrTimeout
			}
		}
		if ne, ok := err.(interface{ Timeout() bool }); ok && ne.Timeout() {
			return "", ErrTimeout
		}
		return "", &ErrUpstream{Msg: err.Error()}
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return "", ErrAuth
	case http.StatusTooManyRequests:
		return "", ErrRateLimit
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(b))
		if len(msg) > 500 {
			msg = msg[:500] + "…"
		}
		return "", &ErrUpstream{Status: resp.StatusCode, Msg: msg}
	}

	var out chatCompletionResponse
	if err := json.Unmarshal(b, &out); err != nil {
		return "", &ErrUpstream{Status: resp.StatusCode, Msg: "invalid json response"}
	}
	if out.Error != nil && strings.TrimSpace(out.Error.Message) != "" {
		return "", &ErrUpstream{Status: resp.StatusCode, Msg: out.Error.Message}
	}
	if len(out.Choices) == 0 || strings.TrimSpace(out.Choices[0].Message.Content) == "" {
		return "", &ErrUpstream{Status: resp.StatusCode, Msg: "empty completion"}
	}
	return strings.TrimSpace(out.Choices[0].Message.Content), nil
}
