package middleware

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"hyperspeed/api/internal/ctxkey"
	"hyperspeed/api/internal/httpx"
	"hyperspeed/api/internal/rbac"
	"hyperspeed/api/internal/store"
)

func RequireSpaceMember(st *store.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			uid, ok := ctxkey.UserID(r.Context())
			if !ok {
				httpx.Error(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			sid, err := uuid.Parse(chi.URLParam(r, "spaceID"))
			if err != nil {
				httpx.Error(w, http.StatusBadRequest, "invalid space id")
				return
			}
			if _, err := st.SpaceMemberRole(r.Context(), sid, uid); err != nil {
				if err == pgx.ErrNoRows {
					oid, oerr := st.OrganizationIDBySpace(r.Context(), sid)
					if oerr == nil {
						// Org-level override for admins / space managers.
						if ok, _ := rbac.HasPermission(r.Context(), st, oid, uid, rbac.OrgManage); ok {
							next.ServeHTTP(w, r)
							return
						}
						if ok, _ := rbac.HasPermission(r.Context(), st, oid, uid, rbac.SpaceMembersManage); ok {
							next.ServeHTTP(w, r)
							return
						}
						// Role allowlist access (admin override handled inside).
						if ok, aerr := st.UserCanAccessSpace(r.Context(), oid, sid, uid); aerr == nil && ok {
							next.ServeHTTP(w, r)
							return
						}
					}
					httpx.Error(w, http.StatusForbidden, "no access to this space")
					return
				}
				httpx.Error(w, http.StatusInternalServerError, "membership")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

