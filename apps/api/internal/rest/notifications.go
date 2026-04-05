package rest

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"hyperspeed/api/internal/ctxkey"
	"hyperspeed/api/internal/httpx"
	"hyperspeed/api/internal/store"
)

type NotificationsHandler struct {
	Store *store.Store
}

func (h *NotificationsHandler) ListMy(w http.ResponseWriter, r *http.Request) {
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	orgIDStr := strings.TrimSpace(r.URL.Query().Get("org_id"))
	orgID, err := uuid.Parse(orgIDStr)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "org_id required")
		return
	}
	if _, err := h.Store.MemberRole(r.Context(), orgID, uid); err != nil {
		httpx.Error(w, http.StatusForbidden, "forbidden")
		return
	}

	limit := 50
	if s := strings.TrimSpace(r.URL.Query().Get("limit")); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			limit = n
		}
	}
	var before *time.Time
	if s := strings.TrimSpace(r.URL.Query().Get("before")); s != "" {
		if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
			before = &t
		}
	}

	// Optional filtering for chat mention mark-read-on-view flows.
	typ := strings.TrimSpace(r.URL.Query().Get("type"))
	spaceIDStr := strings.TrimSpace(r.URL.Query().Get("space_id"))
	chatRoomIDStr := strings.TrimSpace(r.URL.Query().Get("chat_room_id"))
	unreadOnly := strings.TrimSpace(r.URL.Query().Get("unread_only")) == "1"
	if typ != "" && spaceIDStr != "" && chatRoomIDStr != "" {
		spaceID, err := uuid.Parse(spaceIDStr)
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "invalid space_id")
			return
		}
		roomID, err := uuid.Parse(chatRoomIDStr)
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "invalid chat_room_id")
			return
		}
		ids, err := h.Store.ListNotificationIDsByChatTarget(r.Context(), orgID, uid, typ, spaceID, roomID, unreadOnly, limit)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "list notifications")
			return
		}
		unread, err := h.Store.UnreadNotificationsCount(r.Context(), orgID, uid)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "unread count")
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{
			"ids":          ids,
			"unread_count": unread,
		})
		return
	}

	list, err := h.Store.ListNotificationsForUser(r.Context(), orgID, uid, limit, before)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "list notifications")
		return
	}
	if list == nil {
		list = []store.Notification{}
	}
	unread, err := h.Store.UnreadNotificationsCount(r.Context(), orgID, uid)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "unread count")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"notifications": list,
		"unread_count":  unread,
	})
}

type markReadBody struct {
	OrgID string   `json:"org_id"`
	IDs   []string `json:"ids"`
}

func (h *NotificationsHandler) MarkReadMy(w http.ResponseWriter, r *http.Request) {
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var body markReadBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	orgID, err := uuid.Parse(strings.TrimSpace(body.OrgID))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "org_id required")
		return
	}
	if _, err := h.Store.MemberRole(r.Context(), orgID, uid); err != nil {
		httpx.Error(w, http.StatusForbidden, "forbidden")
		return
	}
	var ids []uuid.UUID
	for _, s := range body.IDs {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		id, err := uuid.Parse(s)
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "invalid id")
			return
		}
		ids = append(ids, id)
	}
	n, err := h.Store.MarkNotificationsRead(r.Context(), orgID, uid, ids)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "mark read")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"updated": n})
}

type deleteNotificationsBody struct {
	OrgID string   `json:"org_id"`
	IDs   []string `json:"ids"`
}

func (h *NotificationsHandler) DeleteMy(w http.ResponseWriter, r *http.Request) {
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var body deleteNotificationsBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	orgID, err := uuid.Parse(strings.TrimSpace(body.OrgID))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "org_id required")
		return
	}
	if _, err := h.Store.MemberRole(r.Context(), orgID, uid); err != nil {
		httpx.Error(w, http.StatusForbidden, "forbidden")
		return
	}
	var ids []uuid.UUID
	for _, s := range body.IDs {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		id, err := uuid.Parse(s)
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "invalid id")
			return
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		httpx.Error(w, http.StatusBadRequest, "ids required")
		return
	}
	n, err := h.Store.DeleteNotifications(r.Context(), orgID, uid, ids)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "delete notifications")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"deleted": n})
}

