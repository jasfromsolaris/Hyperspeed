package openrouter

import (
	"encoding/json"

	"hyperspeed/api/internal/cursor"
)

// ChatMessage is an OpenAI/OpenRouter chat message (tool calls, multimodal-safe content).
// Reasoning and ReasoningDetails are used with reasoning-capable models; preserve them across
// tool rounds per https://openrouter.ai/docs/guides/best-practices/reasoning-tokens
type ChatMessage struct {
	Role             string            `json:"role"`
	Content          json.RawMessage   `json:"content,omitempty"`
	ToolCalls        []ChatToolCall    `json:"tool_calls,omitempty"`
	ToolCallID       string            `json:"tool_call_id,omitempty"`
	FunctionName     string            `json:"name,omitempty"`
	Reasoning        json.RawMessage   `json:"reasoning,omitempty"`
	ReasoningDetails json.RawMessage   `json:"reasoning_details,omitempty"`
}

// ChatToolCall is a single tool invocation from the assistant.
type ChatToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// FunctionCall carries the function name and JSON arguments string.
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// StringContent returns message content as plain text; empty if null or non-string JSON.
func (m ChatMessage) StringContent() string {
	if len(m.Content) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(m.Content, &s); err == nil {
		return s
	}
	return ""
}

// ChatMessageFromCursor converts a simple role/content pair for initial prompts.
func ChatMessageFromCursor(m cursor.Message) ChatMessage {
	raw, _ := json.Marshal(m.Content)
	return ChatMessage{Role: m.Role, Content: raw}
}

// ChatMessagesFromCursor converts a slice.
func ChatMessagesFromCursor(msgs []cursor.Message) []ChatMessage {
	out := make([]ChatMessage, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, ChatMessageFromCursor(m))
	}
	return out
}

// ToolResultMessage builds a tool role message for the next completion round.
func ToolResultMessage(toolCallID, text string) ChatMessage {
	raw, _ := json.Marshal(text)
	return ChatMessage{
		Role:       "tool",
		ToolCallID: toolCallID,
		Content:    raw,
	}
}
