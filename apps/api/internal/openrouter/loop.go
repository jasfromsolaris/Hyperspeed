package openrouter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"hyperspeed/api/internal/cursor"
)

// MaxToolOutputTraceBytes is the maximum size of each tool result string stored in ToolLoopTrace.
const MaxToolOutputTraceBytes = 24 * 1024

// ToolLoopTrace captures optional observability data for Peek / audits (reasoning + tool I/O).
type ToolLoopTrace struct {
	Steps []ToolLoopStep `json:"steps"`
}

// ToolLoopStep is one model round (assistant message + optional tool calls and results).
type ToolLoopStep struct {
	Iteration        int                `json:"iteration"`
	Reasoning        json.RawMessage    `json:"reasoning,omitempty"`
	ReasoningDetails json.RawMessage    `json:"reasoning_details,omitempty"`
	AssistantText    string             `json:"assistant_text,omitempty"`
	ToolCalls        []ToolLoopTraceCall `json:"tool_calls,omitempty"`
	ToolResults      []ToolLoopTraceResult `json:"tool_results,omitempty"`
}

// ToolLoopTraceCall is a function tool requested by the assistant.
type ToolLoopTraceCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// ToolLoopTraceResult is the harness output for one tool call.
type ToolLoopTraceResult struct {
	ToolCallID string `json:"tool_call_id"`
	Name       string `json:"name"`
	Output     string `json:"output"`
	Truncated  bool   `json:"truncated,omitempty"`
}

func truncateForTrace(s string, maxBytes int) (string, bool) {
	if maxBytes <= 0 || len(s) <= maxBytes {
		return s, false
	}
	b := []byte(s)
	cut := maxBytes
	if cut > len(b) {
		cut = len(b)
	}
	for cut > 0 && cut < len(b) && b[cut-1]&0xc0 == 0x80 {
		cut--
	}
	if cut == 0 {
		return "…", true
	}
	return string(b[:cut]) + "…", true
}

// ToolLoopOptions limits agentic rounds against OpenRouter.
type ToolLoopOptions struct {
	MaxIterations int
	StepTimeout   time.Duration
	// Reasoning is the top-level JSON object for OpenRouter's "reasoning" parameter (effort, max_tokens, exclude, enabled).
	// Nil or empty omits the field (model default).
	Reasoning json.RawMessage
	// MaxTokens sets completion max_tokens when non-nil (recommended for reasoning models so output fits after thinking).
	MaxTokens *int
	// Trace when non-nil is filled with assistant rounds, reasoning fields, and tool call/results (best-effort).
	Trace *ToolLoopTrace
}

// DefaultToolLoopOptions returns safe defaults.
func DefaultToolLoopOptions() ToolLoopOptions {
	return ToolLoopOptions{
		MaxIterations: 12,
		StepTimeout:   90 * time.Second,
	}
}

// ToolExecutor runs a user-defined function tool (Hyperspeed harness). Return result as JSON-serializable text.
type ToolExecutor func(ctx context.Context, name string, arguments json.RawMessage) (resultText string, err error)

type completionRoundRequest struct {
	Model     string            `json:"model"`
	Messages  []ChatMessage     `json:"messages"`
	Tools     []json.RawMessage `json:"tools,omitempty"`
	Plugins   []json.RawMessage `json:"plugins,omitempty"`
	Reasoning json.RawMessage   `json:"reasoning,omitempty"`
	MaxTokens *int              `json:"max_tokens,omitempty"`
}

// chatRoundAssistantMessage is choices[].message from OpenRouter (incl. reasoning fields).
type chatRoundAssistantMessage struct {
	Role             string          `json:"role"`
	Content          json.RawMessage `json:"content"`
	ToolCalls        []ChatToolCall  `json:"tool_calls"`
	Reasoning        json.RawMessage `json:"reasoning"`
	ReasoningDetails json.RawMessage `json:"reasoning_details"`
}

type completionRoundResponse struct {
	Choices []struct {
		FinishReason string                   `json:"finish_reason"`
		Message      chatRoundAssistantMessage `json:"message"`
	} `json:"choices"`
	Usage json.RawMessage `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func assistantMessageFromRound(msg chatRoundAssistantMessage) ChatMessage {
	out := ChatMessage{
		Role:      msg.Role,
		Content:   msg.Content,
		ToolCalls: msg.ToolCalls,
	}
	if out.Role == "" {
		out.Role = "assistant"
	}
	if len(bytes.TrimSpace(msg.Reasoning)) > 0 {
		out.Reasoning = msg.Reasoning
	}
	if len(bytes.TrimSpace(msg.ReasoningDetails)) > 0 {
		out.ReasoningDetails = msg.ReasoningDetails
	}
	if len(out.ToolCalls) > 0 && out.StringContent() == "" {
		out.Content = json.RawMessage(`null`)
	}
	return out
}

// chatCompletionRound posts one non-streaming completion. tools must be sent on every round per OpenRouter docs.
func (c *Client) chatCompletionRound(ctx context.Context, apiKey, model string, messages []ChatMessage, tools, plugins []json.RawMessage, reasoning json.RawMessage, maxTokens *int) (completionRoundResponse, error) {
	var zero completionRoundResponse
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return zero, fmt.Errorf("openrouter client: %w", cursor.ErrAuth)
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return zero, fmt.Errorf("openrouter client: empty model")
	}
	endpoint, err := c.joinURL()
	if err != nil {
		return zero, err
	}
	hc := c.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 90 * time.Second}
	}
	body := completionRoundRequest{
		Model:     model,
		Messages:  messages,
		Tools:     tools,
		Plugins:   plugins,
		MaxTokens: maxTokens,
	}
	if len(bytes.TrimSpace(reasoning)) > 0 {
		body.Reasoning = reasoning
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return zero, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return zero, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := hc.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			if ctx.Err() == context.DeadlineExceeded {
				return zero, cursor.ErrTimeout
			}
		}
		if ne, ok := err.(interface{ Timeout() bool }); ok && ne.Timeout() {
			return zero, cursor.ErrTimeout
		}
		return zero, &cursor.ErrUpstream{Msg: err.Error()}
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))

	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return zero, cursor.ErrAuth
	case http.StatusTooManyRequests:
		return zero, cursor.ErrRateLimit
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(b))
		if len(msg) > 500 {
			msg = msg[:500] + "…"
		}
		return zero, &cursor.ErrUpstream{Status: resp.StatusCode, Msg: msg}
	}

	var out completionRoundResponse
	if err := json.Unmarshal(b, &out); err != nil {
		return zero, &cursor.ErrUpstream{Status: resp.StatusCode, Msg: "invalid json response"}
	}
	if out.Error != nil && strings.TrimSpace(out.Error.Message) != "" {
		return zero, &cursor.ErrUpstream{Status: resp.StatusCode, Msg: out.Error.Message}
	}
	if len(out.Choices) == 0 {
		return zero, &cursor.ErrUpstream{Status: resp.StatusCode, Msg: "empty choices"}
	}
	return out, nil
}

// ChatCompletionWithToolLoop runs completions until the model stops requesting function tools or max iterations is hit.
// tools must include OpenRouter server tools (e.g. openrouter:web_search) and function tools; plugins optional.
func (c *Client) ChatCompletionWithToolLoop(ctx context.Context, apiKey, model string, seed []ChatMessage, tools, plugins []json.RawMessage, opts ToolLoopOptions, exec ToolExecutor) (finalText string, lastUsage json.RawMessage, err error) {
	def := DefaultToolLoopOptions()
	if opts.MaxIterations <= 0 {
		opts.MaxIterations = def.MaxIterations
	}
	if opts.StepTimeout <= 0 {
		opts.StepTimeout = def.StepTimeout
	}
	msgs := append([]ChatMessage(nil), seed...)
	var usage json.RawMessage
	for i := 0; i < opts.MaxIterations; i++ {
		stepCtx, cancel := context.WithTimeout(ctx, opts.StepTimeout)
		res, err := c.chatCompletionRound(stepCtx, apiKey, model, msgs, tools, plugins, opts.Reasoning, opts.MaxTokens)
		cancel()
		if err != nil {
			return "", usage, err
		}
		usage = res.Usage
		ch := res.Choices[0]
		fnCalls := filterFunctionToolCalls(ch.Message.ToolCalls)
		assistant := assistantMessageFromRound(ch.Message)
		msgs = append(msgs, assistant)

		if len(fnCalls) == 0 {
			t := assistant.StringContent()
			if strings.TrimSpace(t) == "" {
				return "", usage, &cursor.ErrUpstream{Msg: "empty completion"}
			}
			if opts.Trace != nil {
				opts.Trace.Steps = append(opts.Trace.Steps, ToolLoopStep{
					Iteration:        i + 1,
					Reasoning:        assistant.Reasoning,
					ReasoningDetails: assistant.ReasoningDetails,
					AssistantText:    strings.TrimSpace(t),
				})
			}
			return strings.TrimSpace(t), usage, nil
		}

		if exec == nil {
			return "", usage, &cursor.ErrUpstream{Msg: "tool_calls but no executor configured"}
		}

		var step ToolLoopStep
		if opts.Trace != nil {
			step = ToolLoopStep{
				Iteration:        i + 1,
				Reasoning:        assistant.Reasoning,
				ReasoningDetails: assistant.ReasoningDetails,
				AssistantText:    strings.TrimSpace(assistant.StringContent()),
			}
			for _, tc := range fnCalls {
				args := json.RawMessage(tc.Function.Arguments)
				if len(bytes.TrimSpace(args)) == 0 {
					args = json.RawMessage(`{}`)
				}
				step.ToolCalls = append(step.ToolCalls, ToolLoopTraceCall{
					ID:        tc.ID,
					Name:      tc.Function.Name,
					Arguments: args,
				})
			}
		}

		for _, tc := range fnCalls {
			args := json.RawMessage(tc.Function.Arguments)
			if len(bytes.TrimSpace(args)) == 0 {
				args = json.RawMessage(`{}`)
			}
			out, err := exec(ctx, tc.Function.Name, args)
			if err != nil {
				out = fmt.Sprintf(`{"error":%q}`, err.Error())
			}
			if opts.Trace != nil {
				o, trunc := truncateForTrace(out, MaxToolOutputTraceBytes)
				step.ToolResults = append(step.ToolResults, ToolLoopTraceResult{
					ToolCallID: tc.ID,
					Name:       tc.Function.Name,
					Output:     o,
					Truncated:  trunc,
				})
			}
			msgs = append(msgs, ToolResultMessage(tc.ID, out))
		}
		if opts.Trace != nil {
			opts.Trace.Steps = append(opts.Trace.Steps, step)
		}
	}
	return "", usage, &cursor.ErrUpstream{Msg: "tool loop exceeded max iterations"}
}

func filterFunctionToolCalls(calls []ChatToolCall) []ChatToolCall {
	var out []ChatToolCall
	for _, tc := range calls {
		if strings.EqualFold(strings.TrimSpace(tc.Type), "function") && strings.TrimSpace(tc.Function.Name) != "" {
			out = append(out, tc)
		}
	}
	return out
}
