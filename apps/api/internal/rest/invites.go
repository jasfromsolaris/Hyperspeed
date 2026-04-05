package rest

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"hyperspeed/api/internal/ctxkey"
	"hyperspeed/api/internal/httpx"
	"hyperspeed/api/internal/middleware"
	"hyperspeed/api/internal/rbac"
	"hyperspeed/api/internal/store"
)

type InviteHandler struct {
	Store *store.Store
}

type createInviteBody struct {
	Email *string `json:"email"`
}

func (h *InviteHandler) Create(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.OrgMembersManage) {
		return
	}

	var body createInviteBody
	_ = httpx.DecodeJSON(r, &body)

	ttl := 7 * 24 * time.Hour
	inv, raw, err := h.Store.CreateOrgInvite(r.Context(), orgID, uid, body.Email, ttl)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "create invite")
		return
	}

	// Return token once (like API keys).
	httpx.JSON(w, http.StatusCreated, map[string]any{
		"invite": inv,
		"token":  raw,
	})
}

func (h *InviteHandler) Accept(w http.ResponseWriter, r *http.Request) {
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	raw := strings.TrimSpace(chi.URLParam(r, "token"))
	if raw == "" {
		httpx.Error(w, http.StatusBadRequest, "token required")
		return
	}

	inv, err := h.Store.ConsumeOrgInvite(r.Context(), raw, uid)
	if err != nil {
		if err == pgx.ErrNoRows {
			httpx.Error(w, http.StatusNotFound, "invite not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "invite")
		return
	}

	u, err := h.Store.UserByID(r.Context(), uid)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "user")
		return
	}
	if inv.Email != nil && strings.ToLower(strings.TrimSpace(*inv.Email)) != strings.ToLower(strings.TrimSpace(u.Email)) {
		httpx.Error(w, http.StatusForbidden, "invite not for this user")
		return
	}

	// Add membership (legacy role), then map into RBAC role assignment.
	if err := h.Store.AddMember(r.Context(), inv.OrganizationID, uid, store.RoleMember); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "add member")
		return
	}
	_ = h.Store.EnsureLegacyRoleMapped(r.Context(), inv.OrganizationID, uid)

	httpx.JSON(w, http.StatusOK, map[string]any{
		"ok":             true,
		"organization_id": inv.OrganizationID,
	})
}

