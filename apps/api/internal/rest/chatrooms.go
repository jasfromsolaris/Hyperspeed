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

type ChatRoomHandler struct {
	Store *store.Store
}

func (h *ChatRoomHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, hasUser := ctxkey.UserID(r.Context())
	if !hasUser {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.ChatRead) {
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
	list, err := h.Store.ListChatRooms(r.Context(), pid)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "list chat rooms")
		return
	}
	if list == nil {
		list = []store.ChatRoom{}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"chat_rooms": list})
}

type createChatRoomBody struct {
	Name string `json:"name"`
}

func (h *ChatRoomHandler) Create(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, hasUser := ctxkey.UserID(r.Context())
	if !hasUser {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.ChatWrite) {
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
	var body createChatRoomBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" {
		httpx.Error(w, http.StatusBadRequest, "name required")
		return
	}
	cr, err := h.Store.CreateChatRoom(r.Context(), pid, body.Name)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "create chat room")
		return
	}
	httpx.JSON(w, http.StatusCreated, cr)
}

func (h *ChatRoomHandler) Delete(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, hasUser := ctxkey.UserID(r.Context())
	if !hasUser {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.ChatWrite) {
		return
	}
	pid, err := uuid.Parse(chi.URLParam(r, "spaceID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid space id")
		return
	}
	chatRoomID, err := uuid.Parse(chi.URLParam(r, "chatRoomID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid chat room id")
		return
	}
	if _, err := h.Store.GetSpace(r.Context(), orgID, pid); err != nil {
		httpx.Error(w, http.StatusNotFound, "space not found")
		return
	}
	deleted, err := h.Store.DeleteChatRoom(r.Context(), pid, chatRoomID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "delete chat room")
		return
	}
	if !deleted {
		httpx.Error(w, http.StatusNotFound, "chat room not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
