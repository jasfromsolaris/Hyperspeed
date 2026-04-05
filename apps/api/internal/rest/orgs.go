package rest

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"hyperspeed/api/internal/ctxkey"
	"hyperspeed/api/internal/httpx"
	"hyperspeed/api/internal/middleware"
	"hyperspeed/api/internal/provisioning"
	"hyperspeed/api/internal/rbac"
	"hyperspeed/api/internal/store"
)

type OrgHandler struct {
	Store         *store.Store
	EncryptKeyB64 string // HS_SSH_ENCRYPTION_KEY (same 32-byte material for org secrets)
	Provision     *ProvisionHandler // optional; gifted DNS when provisioning is configured
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

	var wantProvision bool
	if b, ok := raw["provision_gifted_dns"]; ok {
		_ = json.Unmarshal(b, &wantProvision)
	}
	var publicIPv4 string
	if v, ok := raw["public_ipv4"]; ok {
		_ = json.Unmarshal(v, &publicIPv4)
	}
	publicIPv4 = strings.TrimSpace(publicIPv4)

	syncRuntimeOrigin := parseSyncRuntimeOrigin(raw)

	if string(rawVal) == "null" {
		if wantProvision {
			httpx.Error(w, http.StatusBadRequest, "provision_gifted_dns requires intended_public_url")
			return
		}
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
		if wantProvision {
			httpx.Error(w, http.StatusBadRequest, "provision_gifted_dns requires intended_public_url")
			return
		}
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

	if wantProvision {
		if h.Provision == nil || !h.Provision.Configured() {
			httpx.Error(w, http.StatusServiceUnavailable, "provisioning_unavailable")
			return
		}
		if publicIPv4 == "" {
			httpx.Error(w, http.StatusBadRequest, "public_ipv4 required when provision_gifted_dns is true")
			return
		}
		slug, err := provisioning.GiftedSubdomainFromIntendedURL(s)
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "intended_public_url must be https://www.{subdomain}."+provisioning.GiftedSubdomainApex)
			return
		}
		if err := h.Provision.ClaimOrganization(r.Context(), orgID, slug, publicIPv4); err != nil {
			var ce *provisioning.ClaimError
			if errors.As(err, &ce) {
				httpx.Error(w, ce.HTTPStatus, ce.Code)
				return
			}
			httpx.Error(w, http.StatusInternalServerError, "provisioning_unavailable")
			return
		}
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
