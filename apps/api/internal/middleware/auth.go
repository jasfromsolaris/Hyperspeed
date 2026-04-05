package middleware

import (
	"net/http"
	"strings"

	"hyperspeed/api/internal/auth"
	"hyperspeed/api/internal/ctxkey"
	"hyperspeed/api/internal/httpx"
	"hyperspeed/api/internal/store"
)

func Auth(a *auth.Service, st *store.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := r.Header.Get("Authorization")
			if h == "" || !strings.HasPrefix(strings.ToLower(h), "bearer ") {
				httpx.Error(w, http.StatusUnauthorized, "missing bearer token")
				return
			}
			raw := strings.TrimSpace(h[7:])
			// Service account token: sa_<random>
			if strings.HasPrefix(raw, "sa_") && st != nil {
				uid, err := st.ServiceAccountUserIDByToken(r.Context(), raw)
				if err == nil {
					st.TouchServiceAccountToken(r.Context(), raw)
					ctx := ctxkey.WithUserID(r.Context(), uid)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}

			claims, err := a.ParseAccess(raw)
			if err != nil {
				httpx.Error(w, http.StatusUnauthorized, "invalid token")
				return
			}
			ctx := ctxkey.WithUserID(r.Context(), claims.UserID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
