package rest

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"hyperspeed/api/internal/ctxkey"
	"hyperspeed/api/internal/httpx"
	"hyperspeed/api/internal/middleware"
	"hyperspeed/api/internal/rbac"
	"hyperspeed/api/internal/store"
)

type RolesHandler struct {
	Store *store.Store
}

type roleOut struct {
	store.Role
	Permissions []string `json:"permissions"`
}

func (h *RolesHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.OrgMembersManage) {
		return
	}
	_ = h.Store.EnsureSystemRoles(r.Context(), orgID)

	roles, err := h.Store.ListRoles(r.Context(), orgID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "roles")
		return
	}
	out := make([]roleOut, 0, len(roles))
	for _, rr := range roles {
		perms, err := h.Store.RolePermissions(r.Context(), rr.ID)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "permissions")
			return
		}
		out = append(out, roleOut{Role: rr, Permissions: perms})
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"roles": out})
}

type createRoleBody struct {
	Name        string   `json:"name"`
	Permissions []string `json:"permissions"`
}

func (h *RolesHandler) Create(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.OrgMembersManage) {
		return
	}
	var body createRoleBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" {
		httpx.Error(w, http.StatusBadRequest, "name required")
		return
	}

	role, err := h.Store.CreateRole(r.Context(), orgID, body.Name)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "create role")
		return
	}
	if err := h.Store.ReplaceRolePermissions(r.Context(), orgID, role.ID, body.Permissions); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "permissions")
		return
	}
	perms, _ := h.Store.RolePermissions(r.Context(), role.ID)
	httpx.JSON(w, http.StatusCreated, map[string]any{"role": roleOut{Role: role, Permissions: perms}})
}

type patchRoleBody struct {
	Name        *string  `json:"name"`
	Permissions []string `json:"permissions"`
	SetPerms    bool     `json:"set_permissions"`
}

func (h *RolesHandler) Patch(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.OrgMembersManage) {
		return
	}
	roleID, err := uuid.Parse(chi.URLParam(r, "roleID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid role id")
		return
	}

	var body patchRoleBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}

	var rr store.Role
	if body.Name != nil {
		name := strings.TrimSpace(*body.Name)
		if name == "" {
			httpx.Error(w, http.StatusBadRequest, "name required")
			return
		}
		rr, err = h.Store.RenameRole(r.Context(), orgID, roleID, name)
		if err != nil {
			if errors.Is(err, store.ErrSystemRoleImmutable) {
				httpx.Error(w, http.StatusBadRequest, "cannot rename system role")
				return
			}
			if err == pgx.ErrNoRows {
				httpx.Error(w, http.StatusNotFound, "role not found")
				return
			}
			httpx.Error(w, http.StatusInternalServerError, "rename")
			return
		}
	} else {
		// Fetch to include fields in response.
		list, err := h.Store.ListRoles(r.Context(), orgID)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "role")
			return
		}
		found := false
		for _, x := range list {
			if x.ID == roleID {
				rr = x
				found = true
				break
			}
		}
		if !found {
			httpx.Error(w, http.StatusNotFound, "role not found")
			return
		}
	}

	if body.SetPerms {
		if rr.IsSystem {
			httpx.Error(w, http.StatusBadRequest, "cannot modify system role permissions")
			return
		}
		if err := h.Store.ReplaceRolePermissions(r.Context(), orgID, roleID, body.Permissions); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "permissions")
			return
		}
	}
	perms, _ := h.Store.RolePermissions(r.Context(), roleID)
	httpx.JSON(w, http.StatusOK, map[string]any{"role": roleOut{Role: rr, Permissions: perms}})
}

func (h *RolesHandler) Delete(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.OrgMembersManage) {
		return
	}
	roleID, err := uuid.Parse(chi.URLParam(r, "roleID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid role id")
		return
	}
	if err := h.Store.DeleteRole(r.Context(), orgID, roleID); err != nil {
		if errors.Is(err, store.ErrCannotDeleteOwnerRole) {
			httpx.Error(w, http.StatusBadRequest, "cannot delete owner role")
			return
		}
		if err == pgx.ErrNoRows {
			httpx.Error(w, http.StatusNotFound, "role not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "delete role")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type setMemberRolesBody struct {
	RoleIDs []uuid.UUID `json:"role_ids"`
}

func (h *RolesHandler) MemberRoles(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.OrgMembersManage) {
		return
	}
	targetUserID, err := uuid.Parse(chi.URLParam(r, "userID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid user id")
		return
	}
	ids, err := h.Store.MemberRoleIDs(r.Context(), orgID, targetUserID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "roles")
		return
	}
	if ids == nil {
		ids = []uuid.UUID{}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"role_ids": ids})
}

func (h *RolesHandler) SetMemberRoles(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.OrgMembersManage) {
		return
	}
	targetUserID, err := uuid.Parse(chi.URLParam(r, "userID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid user id")
		return
	}
	var body setMemberRolesBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	// Ensure target user is a member of this org.
	if _, err := h.Store.MemberRole(r.Context(), orgID, targetUserID); err != nil {
		if err == pgx.ErrNoRows {
			httpx.Error(w, http.StatusNotFound, "member not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "member")
		return
	}

	if err := h.Store.ReplaceMemberRoles(r.Context(), orgID, targetUserID, body.RoleIDs); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "set roles")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

