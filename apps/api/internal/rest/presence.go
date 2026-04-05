package rest

import (
	"net/http"

	"hyperspeed/api/internal/ctxkey"
	"hyperspeed/api/internal/httpx"
	"hyperspeed/api/internal/store"
)

type PresenceHandler struct {
	Store *store.Store
}

func (h *PresenceHandler) Ping(w http.ResponseWriter, r *http.Request) {
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err := h.Store.UpdateLastSeen(r.Context(), uid); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "presence")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

