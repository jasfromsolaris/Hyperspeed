package rest

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"hyperspeed/api/internal/ctxkey"
	"hyperspeed/api/internal/httpx"
	"hyperspeed/api/internal/middleware"
	"hyperspeed/api/internal/rbac"
)

// GetCursorIntegration returns masked Cursor API key state (any org member).
// PUT/DELETE remain org.manage only.
func (h *OrgHandler) GetCursorIntegration(w http.ResponseWriter, r *http.Request) {
	orgID, ok := middleware.OrgIDFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "missing org")
		return
	}
	if _, ok := ctxkey.UserID(r.Context()); !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	meta, err := h.Store.GetOrgCursorIntegrationMeta(r.Context(), orgID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.Error(w, http.StatusNotFound, "organization not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "cursor integration")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"cursor_integration": meta})
}

type putCursorIntegrationBody struct {
	APIKey string `json:"api_key"`
}

// PutCursorIntegration rotates the org Cursor API key (org admin).
func (h *OrgHandler) PutCursorIntegration(w http.ResponseWriter, r *http.Request) {
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
			"error": "HS_SSH_ENCRYPTION_KEY is not configured (required to store Cursor API keys)",
			"code":  "secrets_not_configured",
		})
		return
	}
	var body putCursorIntegrationBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if strings.TrimSpace(body.APIKey) == "" {
		httpx.Error(w, http.StatusBadRequest, "api_key required")
		return
	}
	if err := h.Store.SetOrgCursorAPIKey(r.Context(), orgID, body.APIKey, key32, uid); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.Error(w, http.StatusNotFound, "organization not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "save cursor key")
		return
	}
	meta, err := h.Store.GetOrgCursorIntegrationMeta(r.Context(), orgID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "cursor integration")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"cursor_integration": meta})
}

// DeleteCursorIntegration clears the org Cursor API key (org admin).
func (h *OrgHandler) DeleteCursorIntegration(w http.ResponseWriter, r *http.Request) {
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
	if err := h.Store.ClearOrgCursorAPIKey(r.Context(), orgID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.Error(w, http.StatusNotFound, "organization not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "clear cursor key")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func decodeAppSecretsKey(b64 string) ([]byte, error) {
	k := strings.TrimSpace(b64)
	if k == "" {
		return nil, nil
	}
	return base64.StdEncoding.DecodeString(k)
}
