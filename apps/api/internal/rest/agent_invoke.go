package rest

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"hyperspeed/api/internal/agenttools"
	"hyperspeed/api/internal/ctxkey"
	"hyperspeed/api/internal/httpx"
	"hyperspeed/api/internal/middleware"
	"hyperspeed/api/internal/rbac"
	"hyperspeed/api/internal/store"
)

type AgentInvokeHandler struct {
	Store   *store.Store
	Harness *agenttools.Harness
}

type agentInvokeBody struct {
	Tool      string          `json:"tool"`
	Arguments json.RawMessage `json:"arguments"`
	SessionID *string         `json:"session_id"`
	Mode      *string         `json:"mode"` // ask | plan | agent (default agent)
}

func (h *AgentInvokeHandler) Invoke(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.AgentToolsInvoke) {
		return
	}
	if h.Harness == nil {
		httpx.Error(w, http.StatusInternalServerError, "agent tools not configured")
		return
	}
	var body agentInvokeBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.JSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json", "code": "invalid_json"})
		return
	}
	if body.Tool == "" {
		httpx.JSON(w, http.StatusBadRequest, map[string]any{"error": "tool required", "code": "missing_tool"})
		return
	}
	mode := ""
	if body.Mode != nil {
		mode = strings.TrimSpace(*body.Mode)
	}
	if mode != "" && mode != "ask" && mode != "plan" && mode != "agent" {
		httpx.JSON(w, http.StatusBadRequest, map[string]any{"error": "invalid mode", "code": "invalid_mode"})
		return
	}
	if mode == "" {
		mode = "agent"
	}
	start := time.Now()
	in := agenttools.InvokeInput{Tool: body.Tool, Arguments: body.Arguments, SessionID: body.SessionID, Mode: mode}
	result, err := h.Harness.Invoke(r.Context(), orgID, uid, in)
	h.Harness.LogInvocation(r.Context(), orgID, uid, body.SessionID, body.Tool, body.Arguments, result, err, start)

	if err != nil {
		if he, ok := agenttools.IsHarnessError(err); ok {
			switch he.Code {
			case "forbidden":
				httpx.JSON(w, http.StatusForbidden, map[string]any{"error": he.Message, "code": he.Code})
				return
			case "mode_policy":
				httpx.JSON(w, http.StatusForbidden, map[string]any{"error": he.Message, "code": he.Code})
				return
			case "not_found":
				httpx.JSON(w, http.StatusNotFound, map[string]any{"error": he.Message, "code": he.Code})
				return
			case "invalid_arguments":
				httpx.JSON(w, http.StatusBadRequest, map[string]any{"error": he.Message, "code": he.Code})
				return
			}
		}
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"result": result})
}

// ListTools returns static tool definitions for MCP / docs.
func (h *AgentInvokeHandler) ListTools(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.AgentToolsInvoke) {
		return
	}
	tools := agenttools.AgentToolSpecs()
	httpx.JSON(w, http.StatusOK, map[string]any{"tools": tools})
}
