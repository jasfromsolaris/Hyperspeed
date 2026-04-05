package rest

import (
	"net/http"

	"github.com/google/uuid"

	"hyperspeed/api/internal/httpx"
	"hyperspeed/api/internal/rbac"
	"hyperspeed/api/internal/store"
)

func requireOrgPerm(w http.ResponseWriter, r *http.Request, st *store.Store, orgID, userID uuid.UUID, perm rbac.Permission) bool {
	ok, err := rbac.HasPermission(r.Context(), st, orgID, userID, perm)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "permissions")
		return false
	}
	if !ok {
		httpx.Error(w, http.StatusForbidden, "forbidden")
		return false
	}
	return true
}

