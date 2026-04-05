package rest

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"hyperspeed/api/internal/events"
	"hyperspeed/api/internal/ctxkey"
	"hyperspeed/api/internal/httpx"
	"hyperspeed/api/internal/middleware"
	"hyperspeed/api/internal/rbac"
	"hyperspeed/api/internal/store"
)

type SpaceHandler struct {
	Store *store.Store
	Bus   *events.Bus
}

func (h *SpaceHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.BoardRead) {
		return
	}
	list, err := h.Store.ListSpaces(r.Context(), orgID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "list spaces")
		return
	}
	if list == nil {
		list = []store.Space{}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"spaces": list})
}

type projectAccessOut struct {
	RoleIDs []uuid.UUID `json:"role_ids"`
}

func (h *SpaceHandler) GetAccess(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.SpaceMembersManage) {
		return
	}
	sid, err := uuid.Parse(chi.URLParam(r, "spaceID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid space id")
		return
	}
	if _, err := h.Store.GetSpace(r.Context(), orgID, sid); err != nil {
		httpx.Error(w, http.StatusNotFound, "space not found")
		return
	}
	ids, err := h.Store.ListSpaceAllowedRoleIDs(r.Context(), sid)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "access")
		return
	}
	httpx.JSON(w, http.StatusOK, projectAccessOut{RoleIDs: ids})
}

type putProjectAccessBody struct {
	RoleIDs []uuid.UUID `json:"role_ids"`
}

func (h *SpaceHandler) PutAccess(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.SpaceMembersManage) {
		return
	}
	sid, err := uuid.Parse(chi.URLParam(r, "spaceID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid space id")
		return
	}
	if _, err := h.Store.GetSpace(r.Context(), orgID, sid); err != nil {
		httpx.Error(w, http.StatusNotFound, "space not found")
		return
	}
	var body putProjectAccessBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.RoleIDs == nil {
		body.RoleIDs = []uuid.UUID{}
	}
	if err := h.Store.ReplaceSpaceAllowedRoles(r.Context(), sid, body.RoleIDs); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "access")
		return
	}
	httpx.JSON(w, http.StatusOK, projectAccessOut{RoleIDs: body.RoleIDs})
}

type accessSummaryOut struct {
	Projects []struct {
		ProjectID  uuid.UUID `json:"project_id"`
		CanAccess  bool      `json:"can_access"`
	} `json:"projects"`
}

func (h *SpaceHandler) AccessSummary(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.BoardRead) {
		return
	}
	list, err := h.Store.ListSpaces(r.Context(), orgID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "list spaces")
		return
	}
	out := accessSummaryOut{Projects: make([]struct {
		ProjectID uuid.UUID `json:"project_id"`
		CanAccess bool      `json:"can_access"`
	}, 0, len(list))}
	for _, p := range list {
		ok, err := h.Store.UserCanAccessSpace(r.Context(), orgID, p.ID, uid)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "access")
			return
		}
		out.Projects = append(out.Projects, struct {
			ProjectID uuid.UUID `json:"project_id"`
			CanAccess bool      `json:"can_access"`
		}{ProjectID: p.ID, CanAccess: ok})
	}
	httpx.JSON(w, http.StatusOK, out)
}

// ListAccessibleMembers returns org members who can access this space (role allowlist + overrides).
// Used by chat UI so the member rail matches space access.
func (h *SpaceHandler) ListAccessibleMembers(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	sid, err := uuid.Parse(chi.URLParam(r, "spaceID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid space id")
		return
	}
	if _, err := h.Store.GetSpace(r.Context(), orgID, sid); err != nil {
		httpx.Error(w, http.StatusNotFound, "space not found")
		return
	}
	list, err := h.Store.ListOrgMembersWithUserAccessibleToSpace(r.Context(), orgID, sid)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "members")
		return
	}
	if list == nil {
		list = []store.OrgMemberWithUser{}
	}
	needSAEnrich := false
	for _, m := range list {
		if !m.IsServiceAccount {
			continue
		}
		if m.ServiceAccountProvider == nil && m.OpenRouterModel == nil && m.CursorDefaultRepoURL == nil {
			needSAEnrich = true
			break
		}
	}
	if needSAEnrich {
		if err := h.Store.EnrichOrgMembersWithServiceAccountDetails(r.Context(), orgID, list); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "members")
			return
		}
	}
	allowed, err := h.Store.ListSpaceAllowedRoleIDs(r.Context(), sid)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "access")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"members":            list,
		"allowed_role_ids":   allowed,
		"space_has_allowlist": len(allowed) > 0,
	})
}

type createProjectBody struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (h *SpaceHandler) Create(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.SpaceCreate) {
		return
	}
	var body createProjectBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.Name == "" {
		httpx.Error(w, http.StatusBadRequest, "name required")
		return
	}
	p, err := h.Store.CreateSpace(r.Context(), orgID, body.Name, body.Description)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "create space")
		return
	}
	_ = h.Store.AddSpaceMember(r.Context(), p.ID, uid, store.SpaceRoleOwner)
	h.publishProject(orgID, p.ID)
	httpx.JSON(w, http.StatusCreated, p)
}

func (h *SpaceHandler) publishProject(orgID, projectID uuid.UUID) {
	if h.Bus == nil || h.Bus.Rdb == nil {
		return
	}
	pid := projectID
	payload, err := events.Marshal(events.ProjectUpdated, orgID, &pid, map[string]string{"action": "created"})
	if err != nil {
		return
	}
	_ = h.Bus.Publish(context.Background(), orgID, payload)
}

func (h *SpaceHandler) Get(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	pid, err := uuid.Parse(chi.URLParam(r, "spaceID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid space id")
		return
	}
	p, err := h.Store.GetSpace(r.Context(), orgID, pid)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "space not found")
		return
	}
	httpx.JSON(w, http.StatusOK, p)
}

func (h *SpaceHandler) Board(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.BoardRead) {
		return
	}
	pid, err := uuid.Parse(chi.URLParam(r, "spaceID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid space id")
		return
	}
	p, err := h.Store.GetSpace(r.Context(), orgID, pid)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "space not found")
		return
	}
	b, err := h.Store.GetBoardByProject(r.Context(), p.ID)
	if err != nil {
		if err == pgx.ErrNoRows {
			httpx.Error(w, http.StatusNotFound, "no board")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "board")
		return
	}
	cols, err := h.Store.ListColumns(r.Context(), b.ID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "columns")
		return
	}
	if cols == nil {
		cols = []store.BoardColumn{}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"board":   b,
		"columns": cols,
	})
}

func (h *SpaceHandler) ListBoards(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.BoardRead) {
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
	list, err := h.Store.ListBoardsByProject(r.Context(), pid)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "list boards")
		return
	}
	if list == nil {
		list = []store.Board{}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"boards": list})
}

type createBoardBody struct {
	Name string `json:"name"`
}

func (h *SpaceHandler) CreateBoard(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.BoardWrite) {
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
	var body createBoardBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	b, err := h.Store.CreateBoardWithDefaultColumns(r.Context(), pid, body.Name)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "create board")
		return
	}
	h.publishProject(orgID, pid)
	httpx.JSON(w, http.StatusCreated, b)
}

func (h *SpaceHandler) BoardByID(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.BoardRead) {
		return
	}
	pid, err := uuid.Parse(chi.URLParam(r, "spaceID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid space id")
		return
	}
	boardID, err := uuid.Parse(chi.URLParam(r, "boardID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid board id")
		return
	}
	if _, err := h.Store.GetSpace(r.Context(), orgID, pid); err != nil {
		httpx.Error(w, http.StatusNotFound, "space not found")
		return
	}
	b, err := h.Store.GetBoardInProject(r.Context(), pid, boardID)
	if err != nil {
		if err == pgx.ErrNoRows {
			httpx.Error(w, http.StatusNotFound, "board not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "board")
		return
	}
	cols, err := h.Store.ListColumns(r.Context(), b.ID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "columns")
		return
	}
	if cols == nil {
		cols = []store.BoardColumn{}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"board":   b,
		"columns": cols,
	})
}

func (h *SpaceHandler) DeleteBoard(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, hasUser := ctxkey.UserID(r.Context())
	if !hasUser {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.BoardWrite) {
		return
	}
	pid, err := uuid.Parse(chi.URLParam(r, "spaceID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid space id")
		return
	}
	boardID, err := uuid.Parse(chi.URLParam(r, "boardID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid board id")
		return
	}
	if _, err := h.Store.GetSpace(r.Context(), orgID, pid); err != nil {
		httpx.Error(w, http.StatusNotFound, "space not found")
		return
	}
	if _, err := h.Store.GetBoardInProject(r.Context(), pid, boardID); err != nil {
		if err == pgx.ErrNoRows {
			httpx.Error(w, http.StatusNotFound, "board not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "board")
		return
	}
	deleted, err := h.Store.DeleteBoardInProject(r.Context(), pid, boardID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "delete board")
		return
	}
	if !deleted {
		httpx.Error(w, http.StatusNotFound, "board not found")
		return
	}
	h.publishProject(orgID, pid)
	w.WriteHeader(http.StatusNoContent)
}
