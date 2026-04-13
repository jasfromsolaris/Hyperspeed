package rest

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
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

type OrgHandler struct {
	Store         *store.Store
	EncryptKeyB64 string // HS_SSH_ENCRYPTION_KEY (same 32-byte material for org secrets)
}

func (h *OrgHandler) List(w http.ResponseWriter, r *http.Request) {
	uid, _ := ctxkey.UserID(r.Context())
	list, err := h.Store.ListOrganizationsForUser(r.Context(), uid)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "list organizations")
		return
	}
	if list == nil {
		list = []store.Organization{}
	}
	n, err := h.Store.CountOrganizations(r.Context())
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "list organizations")
		return
	}
	canCreate := n == 0
	httpx.JSON(w, http.StatusOK, map[string]any{
		"organizations":             list,
		"can_create_organization": canCreate,
	})
}

type createOrgBody struct {
	Name string `json:"name"`
}

func (h *OrgHandler) Create(w http.ResponseWriter, r *http.Request) {
	uid, _ := ctxkey.UserID(r.Context())
	var body createOrgBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" {
		httpx.Error(w, http.StatusBadRequest, "name required")
		return
	}
	base := Slugify(body.Name)
	slug := base

	for i := 0; i < 20; i++ {
		o, err := h.Store.CreateOrganizationSelfHostedOne(r.Context(), body.Name, slug, uid)
		if err == nil {
			httpx.JSON(w, http.StatusCreated, o)
			return
		}
		if errors.Is(err, store.ErrSelfHostOrgAlreadyExists) {
			httpx.Error(w, http.StatusForbidden, "single_org_exists")
			return
		}
		if store.IsUniqueViolation(err) {
			slug = fmt.Sprintf("%s-%s", base, uuid.NewString()[:8])
			continue
		}
		httpx.Error(w, http.StatusInternalServerError, "create organization")
		return
	}
	httpx.Error(w, http.StatusConflict, "could not allocate slug")
}

func (h *OrgHandler) Get(w http.ResponseWriter, r *http.Request) {
	orgID, ok := middleware.OrgIDFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "missing org")
		return
	}
	o, err := h.Store.GetOrganization(r.Context(), orgID)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "organization not found")
		return
	}
	httpx.JSON(w, http.StatusOK, o)
}

// MyPermissions returns the current user's effective RBAC permission strings for this org.
// Any org member may call this (RequireOrgMember); used by the web UI for navigation.
func (h *OrgHandler) MyPermissions(w http.ResponseWriter, r *http.Request) {
	orgID, ok := middleware.OrgIDFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "missing org")
		return
	}
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err := h.Store.EnsureSystemRoles(r.Context(), orgID); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "permissions")
		return
	}
	if err := h.Store.EnsureLegacyRoleMapped(r.Context(), orgID, uid); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "permissions")
		return
	}
	perms, err := h.Store.MemberEffectivePermissions(r.Context(), orgID, uid)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "permissions")
		return
	}
	if perms == nil {
		perms = []string{}
	}
	if ok, err := h.Store.IsLegacyOrgAdmin(r.Context(), orgID, uid); err == nil && ok {
		perms = mergeLegacyAdminPermissionStrings(perms)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"permissions": perms})
}

// mergeLegacyAdminPermissionStrings ensures the SPA sees the same capability set as rbac.HasPermission
// grants for organization_members.role=admin when RBAC rows are missing or incomplete.
func mergeLegacyAdminPermissionStrings(have []string) []string {
	set := make(map[string]struct{}, len(have)+len(rbac.AllPermissions))
	for _, p := range have {
		set[p] = struct{}{}
	}
	for _, p := range rbac.AllPermissions {
		set[string(p)] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func (h *OrgHandler) Patch(w http.ResponseWriter, r *http.Request) {
	orgID, ok := middleware.OrgIDFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "missing org")
		return
	}
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.OrgManage) {
		return
	}
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	rawVal, has := raw["intended_public_url"]
	if !has {
		httpx.Error(w, http.StatusBadRequest, "intended_public_url required")
		return
	}

	syncRuntimeOrigin := parseSyncRuntimeOrigin(raw)

	if string(rawVal) == "null" {
		applyRo := syncRuntimeOrigin
		o, err := h.Store.UpdateOrgIntendedPublicURL(r.Context(), orgID, nil, applyRo, nil)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "update organization")
			return
		}
		httpx.JSON(w, http.StatusOK, o)
		return
	}
	var s string
	if err := json.Unmarshal(rawVal, &s); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid intended_public_url")
		return
	}
	s = strings.TrimSpace(s)
	if s == "" {
		applyRo := syncRuntimeOrigin
		o, err := h.Store.UpdateOrgIntendedPublicURL(r.Context(), orgID, nil, applyRo, nil)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "update organization")
			return
		}
		httpx.JSON(w, http.StatusOK, o)
		return
	}
	if len(s) > 2048 {
		httpx.Error(w, http.StatusBadRequest, "intended_public_url too long")
		return
	}
	low := strings.ToLower(s)
	if !strings.HasPrefix(low, "http://") && !strings.HasPrefix(low, "https://") {
		httpx.Error(w, http.StatusBadRequest, "intended_public_url must be an http(s) URL")
		return
	}

	var runtimeOrigin *string
	applyRo := syncRuntimeOrigin
	if applyRo {
		ro, err := publicOriginFromIntendedURL(s)
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "invalid intended_public_url")
			return
		}
		runtimeOrigin = &ro
	}

	o, err := h.Store.UpdateOrgIntendedPublicURL(r.Context(), orgID, &s, applyRo, runtimeOrigin)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "update organization")
		return
	}
	httpx.JSON(w, http.StatusOK, o)
}

func parseSyncRuntimeOrigin(raw map[string]json.RawMessage) bool {
	if b, ok := raw["sync_runtime_origin"]; ok {
		var v bool
		if json.Unmarshal(b, &v) == nil {
			return v
		}
	}
	return true
}

func publicOriginFromIntendedURL(s string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(s))
	if err != nil {
		return "", err
	}
	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("missing scheme or host")
	}
	return u.Scheme + "://" + u.Host, nil
}

func (h *OrgHandler) Members(w http.ResponseWriter, r *http.Request) {
	orgID, ok := middleware.OrgIDFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "missing org")
		return
	}
	list, err := h.Store.ListOrgMembersWithUser(r.Context(), orgID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "members")
		return
	}
	if list == nil {
		list = []store.OrgMemberWithUser{}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"members": list})
}

func (h *OrgHandler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	orgID, ok := middleware.OrgIDFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "missing org")
		return
	}
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.OrgMembersManage) {
		return
	}
	targetID, err := uuid.Parse(chi.URLParam(r, "userID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid user id")
		return
	}
	if targetID == uid {
		httpx.Error(w, http.StatusBadRequest, "cannot remove yourself")
		return
	}
	if err := h.Store.RemoveMember(r.Context(), orgID, targetID); err != nil {
		if err == pgx.ErrNoRows || strings.Contains(err.Error(), "no rows") {
			httpx.Error(w, http.StatusNotFound, "member not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "remove member")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *OrgHandler) Features(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	f, err := h.Store.OrgFeatures(r.Context(), orgID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "features")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"features": f})
}

type patchOrgFeaturesBody struct {
	DatasetsEnabled     *bool `json:"datasets_enabled"`
	OpenSignupsEnabled  *bool `json:"open_signups_enabled"`
}

func (h *OrgHandler) PatchFeatures(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.OrgManage) {
		return
	}
	var body patchOrgFeaturesBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	cur, err := h.Store.OrgFeatures(r.Context(), orgID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "features")
		return
	}
	next := cur
	if body.DatasetsEnabled != nil {
		next.DatasetsEnabled = *body.DatasetsEnabled
	}
	if body.OpenSignupsEnabled != nil {
		next.OpenSignupsEnabled = *body.OpenSignupsEnabled
	}
	out, err := h.Store.SetOrgFeatures(r.Context(), orgID, next)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "update features")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"features": out})
}
