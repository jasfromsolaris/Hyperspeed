package chatai

import (
	"encoding/json"
	"strings"
	"time"

	"hyperspeed/api/internal/agenttools"
	"hyperspeed/api/internal/config"
)

// OpenRouterChatTooling configures OpenRouter server + function tools for staff mentions.
type OpenRouterChatTooling struct {
	Enabled                  bool
	WebSearchTool            bool
	DatetimeTool             bool
	WebSearchEngine          string
	WebSearchMaxResults      int
	WebSearchMaxTotalResults int
	SkipPreread              bool
	MaxIterations            int
	StepTimeout              time.Duration
	PluginsRaw               []json.RawMessage
	// Reasoning is the OpenRouter "reasoning" request object; nil/empty to omit.
	Reasoning json.RawMessage
	// MaxTokens is sent as max_tokens when non-nil.
	MaxTokens *int
}

// OpenRouterToolingFromConfig maps server config to worker tooling.
func OpenRouterToolingFromConfig(cfg config.Config) OpenRouterChatTooling {
	step := time.Duration(cfg.OpenRouterChatToolStepTimeoutSec) * time.Second
	if cfg.OpenRouterChatToolStepTimeoutSec <= 0 {
		step = 90 * time.Second
	}
	plugins := cfg.OpenRouterPluginsRaw
	if len(plugins) == 0 {
		plugins = nil
	}
	reasoning := cfg.OpenRouterChatReasoningJSON
	if len(reasoning) == 0 {
		reasoning = nil
	}
	var maxTok *int
	if cfg.OpenRouterChatMaxTokens > 0 {
		v := cfg.OpenRouterChatMaxTokens
		maxTok = &v
	}
	return OpenRouterChatTooling{
		Enabled:                  cfg.OpenRouterChatToolsEnabled,
		WebSearchTool:            cfg.OpenRouterWebSearchTool,
		DatetimeTool:             cfg.OpenRouterDatetimeTool,
		WebSearchEngine:          cfg.OpenRouterWebSearchEngine,
		WebSearchMaxResults:      cfg.OpenRouterWebSearchMaxResults,
		WebSearchMaxTotalResults: cfg.OpenRouterWebSearchMaxTotalResults,
		SkipPreread:              cfg.OpenRouterChatSkipPreread,
		MaxIterations:            cfg.OpenRouterChatMaxToolIterations,
		StepTimeout:              step,
		PluginsRaw:               plugins,
		Reasoning:                reasoning,
		MaxTokens:                maxTok,
	}
}

// BuildRequestTools returns the full tools array for each OpenRouter chat completion request (server tools first).
func (t OpenRouterChatTooling) BuildRequestTools() ([]json.RawMessage, error) {
	if !t.Enabled {
		return nil, nil
	}
	fn, err := agenttools.HyperspeedFunctionToolsOpenRouterJSON()
	if err != nil {
		return nil, err
	}
	var server []json.RawMessage
	if t.WebSearchTool {
		m := map[string]any{"type": "openrouter:web_search"}
		params := map[string]any{}
		eng := strings.TrimSpace(t.WebSearchEngine)
		if eng != "" && !strings.EqualFold(eng, "auto") {
			params["engine"] = eng
		}
		if t.WebSearchMaxResults > 0 {
			params["max_results"] = t.WebSearchMaxResults
		}
		if t.WebSearchMaxTotalResults > 0 {
			params["max_total_results"] = t.WebSearchMaxTotalResults
		}
		if len(params) > 0 {
			m["parameters"] = params
		}
		b, err := json.Marshal(m)
		if err != nil {
			return nil, err
		}
		server = append(server, json.RawMessage(b))
	}
	if t.DatetimeTool {
		b, err := json.Marshal(map[string]any{"type": "openrouter:datetime"})
		if err != nil {
			return nil, err
		}
		server = append(server, json.RawMessage(b))
	}
	out := make([]json.RawMessage, 0, len(server)+len(fn))
	out = append(out, server...)
	out = append(out, fn...)
	return out, nil
}
