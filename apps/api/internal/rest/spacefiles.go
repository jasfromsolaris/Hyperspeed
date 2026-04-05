package rest

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"hyperspeed/api/internal/ctxkey"
	"hyperspeed/api/internal/httpx"
	"hyperspeed/api/internal/middleware"
	"hyperspeed/api/internal/rbac"
	"hyperspeed/api/internal/store"
)

type SpaceFileHandler struct {
	Store *store.Store
}

func (h *SpaceFileHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, hasUser := ctxkey.UserID(r.Context())
	if !hasUser {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.FilesRead) {
		return
	}
	pid, err := uuid.Parse(chi.URLParam(r, "spaceID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid space id")
		return
	}
	if _, err := h.Store.GetSpace(r.Context(), orgID, pid); err != nil {
		httpx.Error(w, http.StatusNotFound, "space not found")
		return
	}
	list, err := h.Store.ListSpaceFiles(r.Context(), pid)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "list files")
		return
	}
	if list == nil {
		list = []store.SpaceFile{}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"files": list})
}

type createSpaceFileBody struct {
	Name string `json:"name"`
}

func (h *SpaceFileHandler) Create(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, hasUser := ctxkey.UserID(r.Context())
	if !hasUser {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.FilesWrite) {
		return
	}
	pid, err := uuid.Parse(chi.URLParam(r, "spaceID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid space id")
		return
	}
	if _, err := h.Store.GetSpace(r.Context(), orgID, pid); err != nil {
		httpx.Error(w, http.StatusNotFound, "space not found")
		return
	}
	var body createSpaceFileBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" {
		httpx.Error(w, http.StatusBadRequest, "name required")
		return
	}
	f, err := h.Store.CreateSpaceFile(r.Context(), pid, body.Name)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "create file")
		return
	}
	httpx.JSON(w, http.StatusCreated, f)
}

func (h *SpaceFileHandler) Delete(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, hasUser := ctxkey.UserID(r.Context())
	if !hasUser {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.FilesDelete) {
		return
	}
	pid, err := uuid.Parse(chi.URLParam(r, "spaceID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid space id")
		return
	}
	fileID, err := uuid.Parse(chi.URLParam(r, "fileID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid file id")
		return
	}
	if _, err := h.Store.GetSpace(r.Context(), orgID, pid); err != nil {
		httpx.Error(w, http.StatusNotFound, "space not found")
		return
	}
	deleted, err := h.Store.DeleteSpaceFile(r.Context(), pid, fileID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "delete file")
		return
	}
	if !deleted {
		httpx.Error(w, http.StatusNotFound, "file not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
