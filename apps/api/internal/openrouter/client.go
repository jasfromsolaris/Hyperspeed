package openrouter

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

// Client calls OpenRouter's OpenAI-compatible chat completions endpoint.
type Client struct {
	BaseURL    string
	ChatPath   string
	HTTPClient *http.Client
}

func (c *Client) joinURL() (string, error) {
	base := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if base == "" {
		return "", errors.New("openrouter client: empty base url")
	}
	path := strings.TrimSpace(c.ChatPath)
	if path == "" {
		path = "/chat/completions"
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

// ChatCompletionOpts optional OpenRouter request fields (reasoning models, token budget).
type ChatCompletionOpts struct {
	Reasoning json.RawMessage
	MaxTokens *int
}

type chatCompletionRequest struct {
	Model     string           `json:"model"`
	Messages  []cursor.Message `json:"messages"`
	Reasoning json.RawMessage  `json:"reasoning,omitempty"`
	MaxTokens *int             `json:"max_tokens,omitempty"`
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
// opts may be nil; when set, Reasoning and MaxTokens are sent per OpenRouter docs.
func (c *Client) ChatCompletion(ctx context.Context, apiKey, model string, messages []cursor.Message, opts *ChatCompletionOpts) (string, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return "", fmt.Errorf("openrouter client: %w", cursor.ErrAuth)
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return "", fmt.Errorf("openrouter client: empty model")
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
		Model:    model,
		Messages: messages,
	}
	if opts != nil {
		body.MaxTokens = opts.MaxTokens
		if len(bytes.TrimSpace(opts.Reasoning)) > 0 {
			body.Reasoning = opts.Reasoning
		}
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
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := hc.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			if ctx.Err() == context.DeadlineExceeded {
				return "", cursor.ErrTimeout
			}
		}
		if ne, ok := err.(interface{ Timeout() bool }); ok && ne.Timeout() {
			return "", cursor.ErrTimeout
		}
		return "", &cursor.ErrUpstream{Msg: err.Error()}
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return "", cursor.ErrAuth
	case http.StatusTooManyRequests:
		return "", cursor.ErrRateLimit
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(b))
		if len(msg) > 500 {
			msg = msg[:500] + "…"
		}
		return "", &cursor.ErrUpstream{Status: resp.StatusCode, Msg: msg}
	}

	var out chatCompletionResponse
	if err := json.Unmarshal(b, &out); err != nil {
		return "", &cursor.ErrUpstream{Status: resp.StatusCode, Msg: "invalid json response"}
	}
	if out.Error != nil && strings.TrimSpace(out.Error.Message) != "" {
		return "", &cursor.ErrUpstream{Status: resp.StatusCode, Msg: out.Error.Message}
	}
	if len(out.Choices) == 0 || strings.TrimSpace(out.Choices[0].Message.Content) == "" {
		return "", &cursor.ErrUpstream{Status: resp.StatusCode, Msg: "empty completion"}
	}
	return strings.TrimSpace(out.Choices[0].Message.Content), nil
}
