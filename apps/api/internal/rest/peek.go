package rest

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"hyperspeed/api/internal/ctxkey"
	"hyperspeed/api/internal/httpx"
	"hyperspeed/api/internal/middleware"
	"hyperspeed/api/internal/store"
)

type PeekHandler struct {
	Store *store.Store
}

// coVisibleAIStaffForViewer returns service-account user IDs that share at least one space with the viewer.
func (h *PeekHandler) coVisibleAIStaffForViewer(ctx context.Context, orgID, viewerUID uuid.UUID) (map[uuid.UUID]struct{}, error) {
	spaces, err := h.Store.ListSpaces(ctx, orgID)
	if err != nil {
		return nil, err
	}
	var viewerSpaces []store.Space
	for _, sp := range spaces {
		okAccess, err := h.Store.UserHasEffectiveSpaceAccess(ctx, orgID, sp.ID, viewerUID)
		if err != nil {
			return nil, err
		}
		if okAccess {
			viewerSpaces = append(viewerSpaces, sp)
		}
	}

	members, err := h.Store.ListOrgMembersWithUser(ctx, orgID)
	if err != nil {
		return nil, err
	}
	coVisible := make(map[uuid.UUID]struct{})
	for _, m := range members {
		if !m.IsServiceAccount {
			continue
		}
		for _, sp := range viewerSpaces {
			okAI, err := h.Store.UserHasEffectiveSpaceAccess(ctx, orgID, sp.ID, m.UserID)
			if err != nil {
				return nil, err
			}
			if okAI {
				coVisible[m.UserID] = struct{}{}
				break
			}
		}
	}
	return coVisible, nil
}

// AIActivity GET /organizations/{orgID}/peek/ai-activity — mention-reply runs for AI staff co-visible with the viewer.
func (h *PeekHandler) AIActivity(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	limit := 200
	if q := r.URL.Query().Get("limit"); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}

	ctx := r.Context()
	coVisible, err := h.coVisibleAIStaffForViewer(ctx, orgID, uid)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "peek access")
		return
	}

	rows, err := h.Store.ListChatAIMentionRepliesEnrichedForOrg(ctx, orgID, limit)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "peek activity")
		return
	}
	out := make([]store.ChatAIMentionReplyEnriched, 0, len(rows))
	for _, row := range rows {
		okRow, err := h.Store.UserHasEffectiveSpaceAccess(ctx, orgID, row.SpaceID, uid)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "access")
			return
		}
		if !okRow {
			continue
		}
		if _, ok := coVisible[row.AIUserID]; !ok {
			continue
		}
		out = append(out, row)
	}

	coIDs := make([]string, 0, len(coVisible))
	for id := range coVisible {
		coIDs = append(coIDs, id.String())
	}

	httpx.JSON(w, http.StatusOK, map[string]any{
		"activities":               out,
		"co_visible_ai_user_ids": coIDs,
	})
}

// AIRunDetail GET /organizations/{orgID}/peek/ai-activity/runs/{replyID} — one mention run with run_detail (structured log).
func (h *PeekHandler) AIRunDetail(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	replyID, err := uuid.Parse(chi.URLParam(r, "replyID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid reply id")
		return
	}
	ctx := r.Context()
	row, err := h.Store.GetChatAIMentionReplyEnrichedByID(ctx, orgID, replyID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.Error(w, http.StatusNotFound, "not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "peek run")
		return
	}
	okRow, err := h.Store.UserHasEffectiveSpaceAccess(ctx, orgID, row.SpaceID, uid)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "access")
		return
	}
	if !okRow {
		httpx.Error(w, http.StatusNotFound, "not found")
		return
	}
	coVisible, err := h.coVisibleAIStaffForViewer(ctx, orgID, uid)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "peek access")
		return
	}
	if _, ok := coVisible[row.AIUserID]; !ok {
		httpx.Error(w, http.StatusNotFound, "not found")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"reply": row})
}
