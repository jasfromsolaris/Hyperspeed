package rest

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"hyperspeed/api/internal/chatmentions"
	"hyperspeed/api/internal/ctxkey"
	"hyperspeed/api/internal/events"
	"hyperspeed/api/internal/httpx"
	"hyperspeed/api/internal/middleware"
	"hyperspeed/api/internal/rbac"
	"hyperspeed/api/internal/store"
)

type ChatMessageHandler struct {
	Store *store.Store
	Bus   *events.Bus
}

// aiMentionAckEmoji is added by the AI staff user on the source message as soon as a mention is queued.
const aiMentionAckEmoji = "💭"

func (h *ChatMessageHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
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
	roomID, err := uuid.Parse(chi.URLParam(r, "chatRoomID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid chat room id")
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
	msgs, err := h.Store.ListChatMessages(r.Context(), roomID, limit, before)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "list messages")
		return
	}
	if msgs == nil {
		msgs = []store.ChatMessage{}
	}
	ids := make([]uuid.UUID, 0, len(msgs))
	for _, m := range msgs {
		ids = append(ids, m.ID)
	}
	reactions, err := h.Store.ListReactionsForMessages(r.Context(), ids)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "reactions")
		return
	}
	attachments, err := h.Store.ListAttachmentsForMessages(r.Context(), ids)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "attachments")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"messages":    msgs,
		"reactions":   reactions,
		"attachments": attachments,
	})
}

type createAttachmentBody struct {
	Name      string `json:"name"`
	Mime      string `json:"mime"`
	SizeBytes int    `json:"size_bytes"`
	URL       string `json:"url"`
}

type createMessageBody struct {
	Content     string                 `json:"content"`
	Attachments []createAttachmentBody `json:"attachments"`
}

func (h *ChatMessageHandler) Create(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
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
	roomID, err := uuid.Parse(chi.URLParam(r, "chatRoomID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid chat room id")
		return
	}
	var body createMessageBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	body.Content = strings.TrimSpace(body.Content)
	if body.Content == "" && len(body.Attachments) == 0 {
		httpx.Error(w, http.StatusBadRequest, "content or attachment required")
		return
	}
	m, err := h.Store.CreateChatMessage(r.Context(), pid, roomID, uid, body.Content)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "create message")
		return
	}
	var atts []store.ChatMessageAttachment
	for _, a := range body.Attachments {
		if strings.TrimSpace(a.URL) == "" {
			continue
		}
		aa, err := h.Store.AddAttachment(r.Context(), m.ID, a.Name, a.Mime, a.SizeBytes, a.URL)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "attachment")
			return
		}
		atts = append(atts, aa)
	}
	h.publishChat(orgID, pid, roomID, events.ChatMessageCreated, map[string]any{
		"message":     m,
		"attachments": atts,
	})

	// Mentions -> persistent notifications + realtime ping.
	mentionedUsers, mentionedRoles := chatmentions.ParseMentionIDs(m.Content)
	var aiMentionedUsers []uuid.UUID
	if len(mentionedUsers) > 0 {
		aiMentionedUsers, err = h.Store.FilterServiceAccountUserIDsInOrg(r.Context(), orgID, mentionedUsers)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "ai staff lookup")
			return
		}
	}
	if len(mentionedUsers) > 0 || len(mentionedRoles) > 0 {
		if err := chatmentions.NotifyMentionRecipients(r.Context(), h.Store, h.Bus, orgID, pid, roomID, m.ID, uid, m.Content); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "notify")
			return
		}
	}
	for _, aiUserID := range aiMentionedUsers {
		// Immediate UI ack: AI user reacts on the message so clients know the job was queued.
		if rr, rerr := h.Store.AddReaction(r.Context(), m.ID, aiUserID, aiMentionAckEmoji); rerr != nil {
			slog.Warn("ai mention ack reaction", "err", rerr, "message_id", m.ID, "ai_user_id", aiUserID)
		} else {
			h.publishChat(orgID, pid, roomID, events.ChatReactionAdded, rr)
		}
		// Mentioned AI staff should respond asynchronously in this room.
		// Do not require the service account to pass space allowlist checks: many orgs
		// restrict spaces by role while AI staff only has member roles. The author
		// already proved write access by posting here.
		h.publishAIMentionRequested(orgID, events.ChatAIMentionRequestedPayload{
			OrganizationID:    orgID,
			SpaceID:           pid,
			ChatRoomID:        roomID,
			SourceMessageID:   m.ID,
			AIUserID:          aiUserID,
			RequestedByUserID: uid,
		})
	}

	httpx.JSON(w, http.StatusCreated, map[string]any{
		"message":     m,
		"attachments": atts,
	})
}

type patchMessageBody struct {
	Content *string `json:"content"`
}

func (h *ChatMessageHandler) Patch(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
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
	roomID, err := uuid.Parse(chi.URLParam(r, "chatRoomID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid chat room id")
		return
	}
	msgID, err := uuid.Parse(chi.URLParam(r, "messageID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid message id")
		return
	}
	var body patchMessageBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.Content == nil {
		httpx.Error(w, http.StatusBadRequest, "content required")
		return
	}
	m, err := h.Store.UpdateChatMessageContent(r.Context(), pid, roomID, msgID, uid, *body.Content)
	if err != nil {
		if err == pgx.ErrNoRows {
			httpx.Error(w, http.StatusNotFound, "message not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "patch message")
		return
	}
	h.publishChat(orgID, pid, roomID, events.ChatMessageUpdated, m)
	httpx.JSON(w, http.StatusOK, m)
}

func (h *ChatMessageHandler) Delete(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
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
	roomID, err := uuid.Parse(chi.URLParam(r, "chatRoomID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid chat room id")
		return
	}
	msgID, err := uuid.Parse(chi.URLParam(r, "messageID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid message id")
		return
	}
	ok2, err := h.Store.SoftDeleteChatMessage(r.Context(), pid, roomID, msgID, uid)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "delete message")
		return
	}
	if !ok2 {
		httpx.Error(w, http.StatusNotFound, "message not found")
		return
	}
	h.publishChat(orgID, pid, roomID, events.ChatMessageDeleted, map[string]string{"id": msgID.String()})
	w.WriteHeader(http.StatusNoContent)
}

type reactionBody struct {
	Emoji string `json:"emoji"`
}

func (h *ChatMessageHandler) AddReaction(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
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
	roomID, err := uuid.Parse(chi.URLParam(r, "chatRoomID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid chat room id")
		return
	}
	msgID, err := uuid.Parse(chi.URLParam(r, "messageID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid message id")
		return
	}
	var body reactionBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	body.Emoji = strings.TrimSpace(body.Emoji)
	if body.Emoji == "" {
		httpx.Error(w, http.StatusBadRequest, "emoji required")
		return
	}
	rr, err := h.Store.AddReaction(r.Context(), msgID, uid, body.Emoji)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "reaction")
		return
	}
	h.publishChat(orgID, pid, roomID, events.ChatReactionAdded, rr)
	httpx.JSON(w, http.StatusCreated, rr)
}

func (h *ChatMessageHandler) RemoveReaction(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
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
	roomID, err := uuid.Parse(chi.URLParam(r, "chatRoomID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid chat room id")
		return
	}
	msgID, err := uuid.Parse(chi.URLParam(r, "messageID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid message id")
		return
	}
	emoji := strings.TrimSpace(r.URL.Query().Get("emoji"))
	if emoji == "" {
		httpx.Error(w, http.StatusBadRequest, "emoji required")
		return
	}
	ok2, err := h.Store.RemoveReaction(r.Context(), msgID, uid, emoji)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "reaction")
		return
	}
	if !ok2 {
		httpx.Error(w, http.StatusNotFound, "reaction not found")
		return
	}
	h.publishChat(orgID, pid, roomID, events.ChatReactionRemoved, map[string]any{
		"message_id": msgID,
		"user_id":    uid,
		"emoji":      emoji,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (h *ChatMessageHandler) Search(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
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
	roomID, err := uuid.Parse(chi.URLParam(r, "chatRoomID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid chat room id")
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	msgs, err := h.Store.SearchChatMessages(r.Context(), roomID, q, 50)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "search")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"messages": msgs})
}

func (h *ChatMessageHandler) publishChat(orgID, projectID, chatRoomID uuid.UUID, typ events.Type, payload any) {
	if h.Bus == nil {
		return
	}
	pid := projectID
	envPayload := map[string]any{
		"chat_room_id": chatRoomID,
		"payload":      payload,
	}
	b, err := events.Marshal(typ, orgID, &pid, envPayload)
	if err != nil {
		return
	}
	_ = h.Bus.Publish(context.Background(), orgID, b)
}

func (h *ChatMessageHandler) publishAIMentionRequested(orgID uuid.UUID, payload events.ChatAIMentionRequestedPayload) {
	if h.Bus == nil {
		return
	}
	pid := payload.SpaceID
	b, err := events.Marshal(events.ChatAIMentionRequested, orgID, &pid, payload)
	if err != nil {
		return
	}
	_ = h.Bus.Publish(context.Background(), orgID, b)
}

