package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5"

	"hyperspeed/api/internal/ctxkey"
	"hyperspeed/api/internal/rbac"
	"hyperspeed/api/internal/store"
)

var collabUpgrader = websocket.Upgrader{
	ReadBufferSize:  4098,
	WriteBufferSize: 4098,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

func collabRedisChannel(spaceID, fileID uuid.UUID) string {
	return fmt.Sprintf("hs:collab:%s:%s", spaceID.String(), fileID.String())
}

// ServeCollabWS upgrades to WebSocket, authenticates via ?token=, and relays JSON messages
// for the given file_id across connections using Redis pub/sub (presence + optional Yjs updates).
func (h *FileNodeHandler) ServeCollabWS(w http.ResponseWriter, r *http.Request) {
	if h.Auth == nil || h.Rdb == nil || h.Store == nil {
		http.Error(w, "collab unavailable", http.StatusServiceUnavailable)
		return
	}
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		http.Error(w, "token required", http.StatusUnauthorized)
		return
	}
	claims, err := h.Auth.ParseAccess(token)
	if err != nil {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}
	ctx := ctxkey.WithUserID(r.Context(), claims.UserID)

	orgID, err := uuid.Parse(chi.URLParam(r, "orgID"))
	if err != nil {
		http.Error(w, "invalid organization id", http.StatusBadRequest)
		return
	}
	spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
	if err != nil {
		http.Error(w, "invalid space id", http.StatusBadRequest)
		return
	}
	fileIDStr := strings.TrimSpace(r.URL.Query().Get("file_id"))
	if fileIDStr == "" {
		http.Error(w, "file_id required", http.StatusBadRequest)
		return
	}
	fileID, err := uuid.Parse(fileIDStr)
	if err != nil {
		http.Error(w, "invalid file_id", http.StatusBadRequest)
		return
	}

	if _, err := h.Store.MemberRole(ctx, orgID, claims.UserID); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	ok, err := rbac.HasPermission(ctx, h.Store, orgID, claims.UserID, rbac.FilesRead)
	if err != nil || !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if _, err := h.Store.SpaceMemberRole(ctx, spaceID, claims.UserID); err != nil {
		if err == pgx.ErrNoRows {
			if ok, _ := rbac.HasPermission(ctx, h.Store, orgID, claims.UserID, rbac.OrgManage); !ok {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
		} else {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
	}
	if _, err := h.Store.GetSpace(ctx, orgID, spaceID); err != nil {
		http.Error(w, "space not found", http.StatusNotFound)
		return
	}
	node, err := h.Store.FileNodeByID(ctx, spaceID, fileID)
	if err != nil {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}
	if node.Kind != store.FileNodeFile {
		http.Error(w, "not a file", http.StatusBadRequest)
		return
	}

	displayName := strings.TrimSpace(r.URL.Query().Get("name"))
	if displayName == "" {
		displayName = "Anonymous"
	}

	conn, err := collabUpgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("collab ws upgrade", "err", err)
		return
	}
	defer conn.Close()

	chName := collabRedisChannel(spaceID, fileID)
	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	pubsub := h.Rdb.Subscribe(subCtx, chName)
	defer pubsub.Close()
	if _, err := pubsub.Receive(subCtx); err != nil {
		return
	}
	redisCh := pubsub.Channel()

	var writeMu sync.Mutex

	publish := func(payload []byte) {
		_ = h.Rdb.Publish(subCtx, chName, payload).Err()
	}

	// Announce join
	hello, _ := json.Marshal(map[string]any{
		"type":    "presence",
		"event":   "join",
		"user_id": claims.UserID.String(),
		"name":    displayName,
	})
	publish(hello)

	go func() {
		for {
			select {
			case <-subCtx.Done():
				return
			case msg, ok := <-redisCh:
				if !ok || msg == nil {
					return
				}
				writeMu.Lock()
				_ = conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
				_ = conn.WriteMessage(websocket.TextMessage, []byte(msg.Payload))
				writeMu.Unlock()
			}
		}
	}()

	for {
		_ = conn.SetReadDeadline(time.Now().Add(120 * time.Second))
		_, data, err := conn.ReadMessage()
		if err != nil {
			break
		}
		var envelope map[string]any
		if err := json.Unmarshal(data, &envelope); err != nil {
			continue
		}
		t, _ := envelope["type"].(string)
		switch t {
		case "presence":
			envelope["user_id"] = claims.UserID.String()
			envelope["name"] = displayName
			out, _ := json.Marshal(envelope)
			publish(out)
		case "yjs_update":
			// Relay opaque base64 payload; clients merge via Yjs.
			envelope["user_id"] = claims.UserID.String()
			out, _ := json.Marshal(envelope)
			publish(out)
		default:
			continue
		}
	}

	goodbye, _ := json.Marshal(map[string]any{
		"type":    "presence",
		"event":   "leave",
		"user_id": claims.UserID.String(),
	})
	publish(goodbye)
}
