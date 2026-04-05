package middleware

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"hyperspeed/api/internal/ctxkey"
	"hyperspeed/api/internal/httpx"
	"hyperspeed/api/internal/store"
)

type orgKey struct{}

func OrgIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	v, ok := ctx.Value(orgKey{}).(uuid.UUID)
	return v, ok
}

func RequireOrgMember(st *store.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s := chi.URLParam(r, "orgID")
			orgID, err := uuid.Parse(s)
			if err != nil {
				httpx.Error(w, http.StatusBadRequest, "invalid organization id")
				return
			}
			uid, ok := ctxkey.UserID(r.Context())
			if !ok {
				httpx.Error(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			_, err = st.MemberRole(r.Context(), orgID, uid)
			if err != nil {
				httpx.Error(w, http.StatusForbidden, "not a member of this organization")
				return
			}
			ctx := context.WithValue(r.Context(), orgKey{}, orgID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
