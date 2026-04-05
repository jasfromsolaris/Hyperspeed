package ws

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"

	"hyperspeed/api/internal/auth"
	"hyperspeed/api/internal/events"
	"hyperspeed/api/internal/store"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type Handler struct {
	Auth  *auth.Service
	Store *store.Store
	Rdb   *redis.Client
}

func (h *Handler) ServeWS(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "token required", http.StatusUnauthorized)
		return
	}
	claims, err := h.Auth.ParseAccess(token)
	if err != nil {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}
	orgIDStr := chi.URLParam(r, "orgID")
	orgID, err := uuid.Parse(orgIDStr)
	if err != nil {
		http.Error(w, "invalid org", http.StatusBadRequest)
		return
	}
	if _, err := h.Store.MemberRole(r.Context(), orgID, claims.UserID); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("ws upgrade", "err", err)
		return
	}
	defer conn.Close()

	if h.Rdb == nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"error":"realtime_unavailable"}`))
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	pubsub := h.Rdb.Subscribe(ctx, events.OrgChannel(orgID))
	defer pubsub.Close()
	if _, err := pubsub.Receive(ctx); err != nil {
		return
	}
	ch := pubsub.Channel()

	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				cancel()
				return
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			if msg == nil {
				continue
			}
			_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteMessage(websocket.TextMessage, []byte(msg.Payload)); err != nil {
				return
			}
		}
	}
}
