package rest

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"hyperspeed/api/internal/ctxkey"
	"hyperspeed/api/internal/httpx"
	"hyperspeed/api/internal/middleware"
	"hyperspeed/api/internal/rbac"
	"hyperspeed/api/internal/store"
)

type SignupRequestHandler struct {
	Store *store.Store
}

// List GET /api/v1/organizations/{orgID}/signup-requests
func (h *SignupRequestHandler) List(w http.ResponseWriter, r *http.Request) {
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
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.OrgMembersManage) {
		return
	}
	list, err := h.Store.ListSignupRequests(r.Context(), orgID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "signup requests")
		return
	}
	if list == nil {
		list = []store.SignupRequest{}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"signup_requests": list})
}

// Approve POST /api/v1/organizations/{orgID}/signup-requests/{requestID}/approve
func (h *SignupRequestHandler) Approve(w http.ResponseWriter, r *http.Request) {
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
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.OrgMembersManage) {
		return
	}
	rid, err := uuid.Parse(chi.URLParam(r, "requestID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid request id")
		return
	}
	if err := h.Store.ResolveSignupRequest(r.Context(), orgID, rid, uid, true); err != nil {
		if err == pgx.ErrNoRows {
			httpx.Error(w, http.StatusNotFound, "request not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "approve")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Deny POST /api/v1/organizations/{orgID}/signup-requests/{requestID}/deny
func (h *SignupRequestHandler) Deny(w http.ResponseWriter, r *http.Request) {
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
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.OrgMembersManage) {
		return
	}
	rid, err := uuid.Parse(chi.URLParam(r, "requestID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid request id")
		return
	}
	if err := h.Store.ResolveSignupRequest(r.Context(), orgID, rid, uid, false); err != nil {
		if err == pgx.ErrNoRows {
			httpx.Error(w, http.StatusNotFound, "request not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "deny")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
