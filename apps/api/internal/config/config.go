package config

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	DatabaseURL   string
	RedisURL      string
	JWTSecret     string
	SSHEncryptKey string
	HTTPAddr      string
	CORSOrigin    string
	// CursorAPIBaseURL is the origin for org-backed chat completions (OpenAI-compatible path).
	CursorAPIBaseURL string
	// CursorChatCompletionsPath is appended to CursorAPIBaseURL (default /v1/chat/completions).
	CursorChatCompletionsPath string
	CursorCompletionModel     string
	// CursorHTTPAuth is "bearer" (default) or "basic" (API key as Basic username, empty password).
	CursorHTTPAuth string
	// OpenRouterAPIBaseURL is the origin for org-backed OpenRouter chat completions (OpenAI-compatible).
	OpenRouterAPIBaseURL string
	// OpenRouterChatCompletionsPath is appended to OpenRouterAPIBaseURL (default /chat/completions).
	OpenRouterChatCompletionsPath string
	// OpenRouterChatToolsEnabled enables function + server tools for OpenRouter staff mentions.
	OpenRouterChatToolsEnabled bool
	// OpenRouterWebSearchTool adds openrouter:web_search (OpenRouter server tool; extra billing may apply).
	OpenRouterWebSearchTool bool
	// OpenRouterDatetimeTool adds openrouter:datetime server tool.
	OpenRouterDatetimeTool bool
	// OpenRouterWebSearchEngine: e.g. auto, exa, native (empty = omit, OpenRouter default).
	OpenRouterWebSearchEngine          string
	OpenRouterWebSearchMaxResults      int
	OpenRouterWebSearchMaxTotalResults int
	OpenRouterChatSkipPreread          bool
	OpenRouterChatMaxToolIterations    int
	OpenRouterChatToolStepTimeoutSec   int
	// OpenRouterChatReasoningJSON is optional top-level "reasoning" object (e.g. {"effort":"high"} or {"max_tokens":4096}).
	OpenRouterChatReasoningJSON json.RawMessage
	// OpenRouterChatMaxTokens sets max_tokens on completions when >0 (recommended with reasoning models).
	OpenRouterChatMaxTokens int
	OpenRouterPluginsRaw    []json.RawMessage
	// CursorAgentsBaseURL is the origin for Cloud Agents v0 (default same host as CursorAPIBaseURL).
	CursorAgentsBaseURL string
	// Debug enables verbose logs and relaxed dev defaults (use with Air / local dev only).
	Debug bool
	// PublicAPIBaseURL is the absolute origin for preview iframe URLs (e.g. https://api.example.com). Empty = derive from each HTTP request.
	PublicAPIBaseURL string
	// PublicAppURL is optional public browser origin for this install (shown in onboarding / instance metadata). Empty = omit.
	PublicAppURL string
	// GitWorkdirBase is the directory for per-space git clones (IDE Git integration). Empty = disabled.
	GitWorkdirBase string
	// ProvisioningBaseURL is the public Hyperspeed provisioning gateway origin (HTTPS Worker URL, no trailing path). Empty = provisioning disabled.
	ProvisioningBaseURL string
	// ProvisioningInstallID identifies this deployment to the gateway; Hyperspeed stores the matching secret in Worker KV.
	ProvisioningInstallID string
	// ProvisioningInstallSecret is the HMAC key shared with the gateway for this install (not the control-plane bearer).
	ProvisioningInstallSecret string
	// UpstreamGitHubRepo is optional "owner/name" for client-side update checks (public metadata only).
	UpstreamGitHubRepo string
	// UpdateManifestURL is optional HTTPS URL to a static JSON manifest for update checks (public metadata only).
	UpdateManifestURL string
}

func (c Config) Validate() error {
	var errs []string

	// In production-like modes, prevent shipping insecure defaults.
	if !c.Debug {
		secret := strings.TrimSpace(c.JWTSecret)
		if len(secret) < 32 {
			errs = append(errs, "JWT_SECRET must be at least 32 characters")
		}
		if secret == "dev-change-me-in-production-min-32-chars-long" {
			errs = append(errs, "JWT_SECRET must be changed from the default value")
		}
		// SSH secret storage depends on this; require it in non-debug.
		k := strings.TrimSpace(c.SSHEncryptKey)
		if k == "" {
			errs = append(errs, "HS_SSH_ENCRYPTION_KEY is required (base64 32 bytes)")
		} else {
			raw, err := base64.StdEncoding.DecodeString(k)
			if err != nil {
				errs = append(errs, "HS_SSH_ENCRYPTION_KEY must be base64")
			} else if len(raw) != 32 {
				errs = append(errs, "HS_SSH_ENCRYPTION_KEY must decode to 32 bytes")
			}
		}
	}

	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("invalid config:\n- %s", strings.Join(errs, "\n- "))
}

func Load() Config {
	return Config{
		DatabaseURL:                        getEnv("DATABASE_URL", "postgres://hyperspeed:hyperspeed@localhost:5432/hyperspeed?sslmode=disable"),
		RedisURL:                           getEnv("REDIS_URL", "redis://localhost:6379/0"),
		JWTSecret:                          getEnv("JWT_SECRET", "dev-change-me-in-production-min-32-chars-long"),
		SSHEncryptKey:                      getEnv("HS_SSH_ENCRYPTION_KEY", ""),
		HTTPAddr:                           getEnv("HTTP_ADDR", ":8080"),
		CORSOrigin:                         getEnv("CORS_ORIGIN", "http://localhost:5173"),
		CursorAPIBaseURL:                   getEnv("CURSOR_API_BASE_URL", "https://api.cursor.com"),
		CursorChatCompletionsPath:          getEnv("CURSOR_CHAT_COMPLETIONS_PATH", "/v1/chat/completions"),
		CursorCompletionModel:              getEnv("CURSOR_COMPLETION_MODEL", "auto"),
		CursorHTTPAuth:                     strings.ToLower(getEnv("CURSOR_HTTP_AUTH", "bearer")),
		OpenRouterAPIBaseURL:               getEnv("OPENROUTER_API_BASE_URL", "https://openrouter.ai/api/v1"),
		OpenRouterChatCompletionsPath:      getEnv("OPENROUTER_CHAT_COMPLETIONS_PATH", "/chat/completions"),
		OpenRouterChatToolsEnabled:         envBoolDefault("OPENROUTER_CHAT_TOOLS_ENABLED", true),
		OpenRouterWebSearchTool:            envBoolDefault("OPENROUTER_WEB_SEARCH_TOOL", true),
		OpenRouterDatetimeTool:             envBoolDefault("OPENROUTER_DATETIME_TOOL", true),
		OpenRouterWebSearchEngine:          getEnv("OPENROUTER_WEB_SEARCH_ENGINE", ""),
		OpenRouterWebSearchMaxResults:      envInt("OPENROUTER_WEB_SEARCH_MAX_RESULTS", 5),
		OpenRouterWebSearchMaxTotalResults: envInt("OPENROUTER_WEB_SEARCH_MAX_TOTAL_RESULTS", 15),
		OpenRouterChatSkipPreread:          envBoolDefault("OPENROUTER_CHAT_SKIP_PREREAD", true),
		OpenRouterChatMaxToolIterations:    envInt("OPENROUTER_CHAT_MAX_TOOL_ITERATIONS", 0),
		OpenRouterChatToolStepTimeoutSec:   envInt("OPENROUTER_CHAT_TOOL_STEP_TIMEOUT_SEC", 90),
		OpenRouterChatReasoningJSON:        parseJSONObjectEnv("OPENROUTER_CHAT_REASONING_JSON"),
		OpenRouterChatMaxTokens:            envInt("OPENROUTER_CHAT_MAX_TOKENS", 0),
		OpenRouterPluginsRaw:               parseJSONArrayEnv("OPENROUTER_PLUGINS_JSON"),
		CursorAgentsBaseURL:                getEnv("CURSOR_AGENTS_BASE_URL", ""),
		PublicAPIBaseURL:                   getEnv("PUBLIC_API_BASE_URL", ""),
		PublicAppURL:                       strings.TrimSpace(getEnv("PUBLIC_APP_URL", "")),
		GitWorkdirBase:                     getEnv("HS_GIT_WORKDIR_BASE", ""),
		ProvisioningBaseURL:       strings.TrimSpace(getEnv("PROVISIONING_BASE_URL", "")),
		ProvisioningInstallID:     strings.TrimSpace(getEnv("PROVISIONING_INSTALL_ID", "")),
		ProvisioningInstallSecret: strings.TrimSpace(getEnv("PROVISIONING_INSTALL_SECRET", "")),
		UpstreamGitHubRepo:                 strings.TrimSpace(getEnv("UPSTREAM_GITHUB_REPO", "")),
		UpdateManifestURL:                  strings.TrimSpace(getEnv("UPDATE_MANIFEST_URL", "")),
		Debug:                              envBool("DEBUG") || envBool("HYPERSPEED_DEBUG"),
	}
}

func envBool(key string) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	switch v {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envBoolDefault(key string, def bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok {
		return def
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return def
	}
}

func envInt(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func parseJSONArrayEnv(key string) []json.RawMessage {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return nil
	}
	var out []json.RawMessage
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

// parseJSONObjectEnv returns raw JSON for a single JSON object (not an array). Used for OpenRouter "reasoning".
func parseJSONObjectEnv(key string) json.RawMessage {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" || !json.Valid([]byte(raw)) {
		return nil
	}
	var probe any
	if err := json.Unmarshal([]byte(raw), &probe); err != nil {
		return nil
	}
	if _, ok := probe.(map[string]any); !ok {
		return nil
	}
	return json.RawMessage(raw)
}
