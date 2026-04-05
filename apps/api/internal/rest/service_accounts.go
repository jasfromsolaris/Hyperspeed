package rest

import (
	"fmt"
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

type ServiceAccountsHandler struct {
	Store *store.Store
}

func (h *ServiceAccountsHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.OrgMembersManage) {
		return
	}
	list, err := h.Store.ListServiceAccounts(r.Context(), orgID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "service accounts")
		return
	}
	if list == nil {
		list = []store.ServiceAccount{}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"service_accounts": list})
}

type createServiceAccountBody struct {
	Name                 string      `json:"name"`
	RoleIDs              []uuid.UUID `json:"role_ids"`
	Provider             string      `json:"provider"`
	OpenRouterModel      *string     `json:"openrouter_model"`
	CursorDefaultRepoURL *string     `json:"cursor_default_repo_url"`
	CursorDefaultRef     *string     `json:"cursor_default_ref"`
}

func validateServiceAccountProviderFields(provider string, orModel *string, curRepo *string) error {
	p := strings.TrimSpace(strings.ToLower(provider))
	if p == "" {
		p = store.ProviderOpenRouter
	}
	if p != store.ProviderOpenRouter && p != store.ProviderCursor {
		return fmt.Errorf("provider must be %q or %q", store.ProviderOpenRouter, store.ProviderCursor)
	}
	if p == store.ProviderOpenRouter {
		if orModel == nil || strings.TrimSpace(*orModel) == "" {
			return fmt.Errorf("openrouter_model is required when provider is openrouter")
		}
	}
	// Cursor: cursor_default_repo_url is optional; Cloud Agent launch resolves from space_git_links when unset.
	return nil
}

func (h *ServiceAccountsHandler) Create(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.OrgMembersManage) {
		return
	}
	var body createServiceAccountBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" {
		httpx.Error(w, http.StatusBadRequest, "name required")
		return
	}
	prov := strings.TrimSpace(strings.ToLower(body.Provider))
	if prov == "" {
		prov = store.ProviderOpenRouter
	}
	if prov == store.ProviderOpenRouter && (body.OpenRouterModel == nil || strings.TrimSpace(*body.OpenRouterModel) == "") {
		def := "nvidia/nemotron-3-super-120b-a12b:free"
		body.OpenRouterModel = &def
	}
	if err := validateServiceAccountProviderFields(body.Provider, body.OpenRouterModel, body.CursorDefaultRepoURL); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	in := store.CreateServiceAccountInput{
		Name:                 body.Name,
		Provider:             body.Provider,
		OpenRouterModel:      body.OpenRouterModel,
		CursorDefaultRepoURL: body.CursorDefaultRepoURL,
		CursorDefaultRef:     body.CursorDefaultRef,
	}
	sa, token, err := h.Store.CreateServiceAccountWithOptions(r.Context(), orgID, uid, in)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "create service account")
		return
	}
	if len(body.RoleIDs) > 0 {
		if err := h.Store.ReplaceMemberRoles(r.Context(), orgID, sa.UserID, body.RoleIDs); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "assign roles")
			return
		}
	} else if err := h.Store.EnsureSystemRoles(r.Context(), orgID); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "system roles")
		return
	}

	httpx.JSON(w, http.StatusCreated, map[string]any{
		"service_account": sa,
		"token":           token,
	})
}

func (h *ServiceAccountsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.OrgMembersManage) {
		return
	}
	saID, err := uuid.Parse(chi.URLParam(r, "serviceAccountID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid service account id")
		return
	}
	if err := h.Store.DeleteServiceAccount(r.Context(), orgID, saID); err != nil {
		if err == pgx.ErrNoRows {
			httpx.Error(w, http.StatusNotFound, "not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "delete service account")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type patchServiceAccountBody struct {
	Provider             *string `json:"provider"`
	OpenRouterModel      *string `json:"openrouter_model"`
	CursorDefaultRepoURL *string `json:"cursor_default_repo_url"`
	CursorDefaultRef     *string `json:"cursor_default_ref"`
}

func (h *ServiceAccountsHandler) Patch(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.OrgMembersManage) {
		return
	}
	saID, err := uuid.Parse(chi.URLParam(r, "serviceAccountID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid service account id")
		return
	}
	var body patchServiceAccountBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	cur, err := h.Store.GetServiceAccountInOrg(r.Context(), orgID, saID)
	if err != nil {
		if err == pgx.ErrNoRows {
			httpx.Error(w, http.StatusNotFound, "not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "service account")
		return
	}
	nextProv := cur.Provider
	if body.Provider != nil {
		nextProv = strings.TrimSpace(strings.ToLower(*body.Provider))
	}
	orModel := cur.OpenRouterModel
	if body.OpenRouterModel != nil {
		v := strings.TrimSpace(*body.OpenRouterModel)
		if v == "" {
			orModel = nil
		} else {
			orModel = body.OpenRouterModel
		}
	}
	curRepo := cur.CursorDefaultRepoURL
	if body.CursorDefaultRepoURL != nil {
		v := strings.TrimSpace(*body.CursorDefaultRepoURL)
		if v == "" {
			curRepo = nil
		} else {
			curRepo = body.CursorDefaultRepoURL
		}
	}
	if err := validateServiceAccountProviderFields(nextProv, orModel, curRepo); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	patchIn := store.PatchServiceAccountInput{
		Provider:             body.Provider,
		OpenRouterModel:      body.OpenRouterModel,
		CursorDefaultRepoURL: body.CursorDefaultRepoURL,
		CursorDefaultRef:     body.CursorDefaultRef,
	}
	sa, err := h.Store.PatchServiceAccount(r.Context(), orgID, saID, patchIn)
	if err != nil {
		if err == pgx.ErrNoRows {
			httpx.Error(w, http.StatusNotFound, "not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "patch service account")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"service_account": sa})
}
