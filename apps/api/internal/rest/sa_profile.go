package rest

import (
	"errors"
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

type ServiceAccountProfileHandler struct {
	Store *store.Store
}

func (h *ServiceAccountProfileHandler) Get(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.OrgMembersManage) {
		return
	}
	saID, err := uuid.Parse(chi.URLParam(r, "serviceAccountID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid service account id")
		return
	}
	if _, err := h.Store.GetServiceAccountInOrg(r.Context(), orgID, saID); err != nil {
		if err == pgx.ErrNoRows {
			httpx.Error(w, http.StatusNotFound, "service account not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "service account")
		return
	}
	v, err := h.Store.LatestServiceAccountProfile(r.Context(), saID)
	if err != nil {
		if err == pgx.ErrNoRows {
			def := store.DefaultServiceAccountProfileMarkdown()
			httpx.JSON(w, http.StatusOK, map[string]any{
				"profile":            nil,
				"default_content_md": def,
			})
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "profile")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"profile":            v,
		"default_content_md": store.DefaultServiceAccountProfileMarkdown(),
	})
}

func (h *ServiceAccountProfileHandler) Versions(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.OrgMembersManage) {
		return
	}
	saID, err := uuid.Parse(chi.URLParam(r, "serviceAccountID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid service account id")
		return
	}
	if _, err := h.Store.GetServiceAccountInOrg(r.Context(), orgID, saID); err != nil {
		if err == pgx.ErrNoRows {
			httpx.Error(w, http.StatusNotFound, "service account not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "service account")
		return
	}
	list, err := h.Store.ListServiceAccountProfileVersions(r.Context(), saID, 50)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "versions")
		return
	}
	if list == nil {
		list = []store.ServiceAccountProfileVersion{}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"versions": list})
}

type patchProfileBody struct {
	ContentMD string `json:"content_md"`
}

func (h *ServiceAccountProfileHandler) Patch(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.OrgMembersManage) {
		return
	}
	saID, err := uuid.Parse(chi.URLParam(r, "serviceAccountID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid service account id")
		return
	}
	var body patchProfileBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	v, err := h.Store.AppendServiceAccountProfile(r.Context(), orgID, saID, body.ContentMD, uid)
	if err != nil {
		if err == pgx.ErrNoRows {
			httpx.Error(w, http.StatusNotFound, "service account not found")
			return
		}
		if errors.Is(err, store.ErrProfileTooLarge) {
			httpx.Error(w, http.StatusBadRequest, "profile too large")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "save profile")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"profile": v})
}
