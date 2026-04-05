package agenttools

import (
	"encoding/json"
)

// AgentToolSpecs returns MCP/ListTools-style entries (name, description, inputSchema).
func AgentToolSpecs() []map[string]any {
	return []map[string]any{
		{
			"name":        "space.file.read",
			"description": "Read UTF-8 text content of a file in a space",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"space_id": map[string]any{"type": "string", "description": "Space UUID"},
					"node_id":  map[string]any{"type": "string", "description": "File node UUID"},
				},
				"required": []string{"space_id", "node_id"},
			},
		},
		{
			"name":        "space.list_files",
			"description": "List files and folders under a parent in a space",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"space_id":  map[string]any{"type": "string"},
					"parent_id": map[string]any{"type": "string", "description": "Optional folder node UUID; omit for root"},
				},
				"required": []string{"space_id"},
			},
		},
		{
			"name":        "space.file.propose_patch",
			"description": "Create a pending edit proposal (does not apply until accepted in the web UI)",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"space_id":         map[string]any{"type": "string"},
					"node_id":          map[string]any{"type": "string"},
					"proposed_content": map[string]any{"type": "string"},
				},
				"required": []string{"space_id", "node_id", "proposed_content"},
			},
		},
		{
			"name":        "space.file.create_text",
			"description": "Create a new empty or UTF-8 text file in a space (use propose_patch to edit existing files)",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"space_id":  map[string]any{"type": "string"},
					"parent_id": map[string]any{"type": "string", "description": "Optional folder node UUID; omit for root"},
					"name":      map[string]any{"type": "string", "description": "File name including extension"},
					"content":   map[string]any{"type": "string", "description": "Initial file body; omit or empty for empty file"},
				},
				"required": []string{"space_id", "name"},
			},
		},
		{
			"name":        "space.chat.read_recent",
			"description": "Read recent non-deleted chat messages in a room (explicit IDE/MCP context bridge)",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"space_id":     map[string]any{"type": "string", "description": "Space UUID"},
					"chat_room_id": map[string]any{"type": "string", "description": "Chat room UUID"},
					"limit":        map[string]any{"type": "integer", "description": "Max messages (default 30, max 100)"},
				},
				"required": []string{"space_id", "chat_room_id"},
			},
		},
		{
			"name":        "space.automation.propose",
			"description": "Create a pending space automation (social, etc.) for human approval in Automations — does not activate or post",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"space_id": map[string]any{"type": "string"},
					"name":     map[string]any{"type": "string"},
					"kind":     map[string]any{"type": "string", "description": "e.g. social_post"},
					"config":   map[string]any{"type": "object", "description": "JSON payload (e.g. default_text for drafts)"},
					"note":     map[string]any{"type": "string", "description": "Optional note for reviewers"},
				},
				"required": []string{"space_id", "name", "kind"},
			},
		},
	}
}

// HyperspeedFunctionToolsOpenRouterJSON returns OpenAI/OpenRouter function tool entries for the chat completions API.
func HyperspeedFunctionToolsOpenRouterJSON() ([]json.RawMessage, error) {
	specs := AgentToolSpecs()
	out := make([]json.RawMessage, 0, len(specs))
	for _, spec := range specs {
		name, _ := spec["name"].(string)
		desc, _ := spec["description"].(string)
		schema := spec["inputSchema"]
		wrapped := map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        name,
				"description": desc,
				"parameters":  schema,
			},
		}
		b, err := json.Marshal(wrapped)
		if err != nil {
			return nil, err
		}
		out = append(out, json.RawMessage(b))
	}
	return out, nil
}

// OpenRouterInvokableToolNames lists function tools Hyperspeed executes locally (allowlist).
func OpenRouterInvokableToolNames() []string {
	specs := AgentToolSpecs()
	names := make([]string, 0, len(specs))
	for _, spec := range specs {
		if n, ok := spec["name"].(string); ok {
			names = append(names, n)
		}
	}
	return names
}
