package rest

import (
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"hyperspeed/api/internal/ctxkey"
	"hyperspeed/api/internal/httpx"
	"hyperspeed/api/internal/middleware"
	"hyperspeed/api/internal/rbac"
)

// GetOpenRouterIntegration returns masked OpenRouter API key state (any org member).
// PUT/DELETE remain org.manage only.
func (h *OrgHandler) GetOpenRouterIntegration(w http.ResponseWriter, r *http.Request) {
	orgID, ok := middleware.OrgIDFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "missing org")
		return
	}
	if _, ok := ctxkey.UserID(r.Context()); !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	meta, err := h.Store.GetOrgOpenRouterIntegrationMeta(r.Context(), orgID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.Error(w, http.StatusNotFound, "organization not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "openrouter integration")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"openrouter_integration": meta})
}

type putOpenRouterIntegrationBody struct {
	APIKey string `json:"api_key"`
}

// PutOpenRouterIntegration rotates the org OpenRouter API key (org admin).
func (h *OrgHandler) PutOpenRouterIntegration(w http.ResponseWriter, r *http.Request) {
	orgID, ok := middleware.OrgIDFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "missing org")
		return
	}
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.OrgManage) {
		return
	}
	key32, err := decodeAppSecretsKey(h.EncryptKeyB64)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "encryption key invalid")
		return
	}
	if len(key32) != 32 {
		httpx.JSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "HS_SSH_ENCRYPTION_KEY is not configured (required to store OpenRouter API keys)",
			"code":  "secrets_not_configured",
		})
		return
	}
	var body putOpenRouterIntegrationBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if strings.TrimSpace(body.APIKey) == "" {
		httpx.Error(w, http.StatusBadRequest, "api_key required")
		return
	}
	if err := h.Store.SetOrgOpenRouterAPIKey(r.Context(), orgID, body.APIKey, key32, uid); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.Error(w, http.StatusNotFound, "organization not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "save openrouter key")
		return
	}
	meta, err := h.Store.GetOrgOpenRouterIntegrationMeta(r.Context(), orgID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "openrouter integration")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"openrouter_integration": meta})
}

// DeleteOpenRouterIntegration clears the org OpenRouter API key (org admin).
func (h *OrgHandler) DeleteOpenRouterIntegration(w http.ResponseWriter, r *http.Request) {
	orgID, ok := middleware.OrgIDFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "missing org")
		return
	}
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.OrgManage) {
		return
	}
	if err := h.Store.ClearOrgOpenRouterAPIKey(r.Context(), orgID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.Error(w, http.StatusNotFound, "organization not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "clear openrouter key")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
