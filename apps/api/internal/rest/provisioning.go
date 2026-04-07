package rest

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"hyperspeed/api/internal/ctxkey"
	"hyperspeed/api/internal/httpx"
	"hyperspeed/api/internal/provisioning"
	"hyperspeed/api/internal/rbac"
	"hyperspeed/api/internal/store"
	"hyperspeed/api/internal/version"
)

// ProvisionHandler forwards gifted-subdomain claims to the Hyperspeed-operated provisioning gateway
// (Cloudflare Worker), which authenticates install HMAC and proxies to the private control plane.
type ProvisionHandler struct {
	Store         *store.Store
	BaseURL       string // public gateway origin, e.g. https://provision-gw.hyperspeedapp.com (no trailing path)
	InstallID     string
	InstallSecret string
	HTTPClient    *http.Client
}

type claimBody struct {
	Slug string `json:"slug"`
	IPv4 string `json:"ipv4"`
}

func (h *ProvisionHandler) httpClient() *http.Client {
	if h.HTTPClient != nil {
		return h.HTTPClient
	}
	return &http.Client{Timeout: 45 * time.Second}
}

// Configured reports whether provisioning gateway URL and install credentials are set.
func (h *ProvisionHandler) Configured() bool {
	return h.configured()
}

func (h *ProvisionHandler) configured() bool {
	return strings.TrimSpace(h.BaseURL) != "" &&
		strings.TrimSpace(h.InstallID) != "" &&
		strings.TrimSpace(h.InstallSecret) != ""
}

// postClaimAndSetOrg POSTs to the control plane and sets gifted_subdomain_slug on success.
func (h *ProvisionHandler) postClaimAndSetOrg(ctx context.Context, orgID uuid.UUID, slug, ipv4 string) (successStatus int, respBody []byte, err error) {
	if !h.configured() {
		return 0, nil, provisioning.ErrProvisioningUnavailable()
	}
	status, body, netErr := provisioning.PostClaims(ctx, h.BaseURL, h.InstallID, h.InstallSecret, h.httpClient(), slug, ipv4)
	if err := provisioning.ErrFromClaimResponse(status, body, netErr); err != nil {
		return 0, nil, err
	}
	if h.Store != nil {
		s := slug
		_ = h.Store.SetOrgGiftedSubdomainSlug(ctx, orgID, &s)
	}
	return status, body, nil
}

// ClaimOrganization provisions DNS for slug and records gifted_subdomain_slug for orgID.
func (h *ProvisionHandler) ClaimOrganization(ctx context.Context, orgID uuid.UUID, slug, ipv4 string) error {
	slug = strings.ToLower(strings.TrimSpace(slug))
	ipv4 = strings.TrimSpace(ipv4)
	_, _, err := h.postClaimAndSetOrg(ctx, orgID, slug, ipv4)
	return err
}

// Claim POST /api/v1/provisioning/claim — authenticated user; body { slug, ipv4 }.
func (h *ProvisionHandler) Claim(w http.ResponseWriter, r *http.Request) {
	if !h.configured() {
		httpx.Error(w, http.StatusServiceUnavailable, "provisioning_unavailable")
		return
	}
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var body claimBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	body.Slug = strings.ToLower(strings.TrimSpace(body.Slug))
	body.IPv4 = strings.TrimSpace(body.IPv4)
	if body.Slug == "" || body.IPv4 == "" {
		httpx.Error(w, http.StatusBadRequest, "slug and ipv4 required")
		return
	}

	if h.Store == nil {
		httpx.Error(w, http.StatusInternalServerError, "provisioning_unavailable")
		return
	}
	orgs, err := h.Store.ListOrganizationsForUser(r.Context(), uid)
	if err != nil || len(orgs) == 0 {
		httpx.Error(w, http.StatusForbidden, "no organization")
		return
	}
	orgID := orgs[0].ID

	status, respBody, err := h.postClaimAndSetOrg(r.Context(), orgID, body.Slug, body.IPv4)
	if err != nil {
		var ce *provisioning.ClaimError
		if errors.As(err, &ce) {
			httpx.Error(w, ce.HTTPStatus, ce.Code)
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "provisioning_unavailable")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(respBody)
}

// DeleteClaim DELETE /api/v1/provisioning/claim/{slug} — org.manage; revokes DNS via control plane and clears local slug.
func (h *ProvisionHandler) DeleteClaim(w http.ResponseWriter, r *http.Request) {
	if !h.configured() {
		httpx.Error(w, http.StatusServiceUnavailable, "provisioning_unavailable")
		return
	}
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.Store == nil {
		httpx.Error(w, http.StatusInternalServerError, "provisioning_unavailable")
		return
	}
	slugStr := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "slug")))
	if slugStr == "" {
		httpx.Error(w, http.StatusBadRequest, "slug required")
		return
	}
	orgID, err := h.Store.OrgIDByGiftedSubdomainSlug(r.Context(), slugStr)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.Error(w, http.StatusNotFound, "claim_not_found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "organizations")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.OrgManage) {
		return
	}

	resp, respBody, netErr := provisioning.DeleteGatewayClaim(r.Context(), h.BaseURL, h.InstallID, h.InstallSecret, h.httpClient(), slugStr)
	if netErr != nil {
		httpx.Error(w, http.StatusBadGateway, "provisioning_unavailable")
		return
	}

	if resp >= 200 && resp < 300 || resp == http.StatusNotFound {
		if _, err := h.Store.ClearOrgGiftedSubdomainSlugIfMatch(r.Context(), orgID, slugStr); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "clear claim")
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if resp == http.StatusUnauthorized {
		httpx.Error(w, http.StatusBadGateway, "provisioning_unavailable")
		return
	}
	var errObj map[string]any
	if json.Unmarshal(respBody, &errObj) == nil {
		if e, ok := errObj["error"].(string); ok && e != "" {
			httpx.Error(w, http.StatusBadGateway, e)
			return
		}
	}
	httpx.Error(w, http.StatusBadGateway, "provisioning_unavailable")
}

// PublicHandler exposes non-secret instance metadata for the SPA.
type PublicHandler struct {
	Store *store.Store
	// Provisioning gateway URL + install credentials enable POST /api/v1/provisioning/claim and related flows.
	ProvisioningBaseURL       string
	ProvisioningInstallID     string
	ProvisioningInstallSecret string
	// UpstreamGitHubRepo is optional "owner/name" for SPA update checks against GitHub releases.
	UpstreamGitHubRepo string
	// UpdateManifestURL is optional HTTPS URL to a static JSON manifest for update checks.
	UpdateManifestURL string
	// PublicAppURL is optional browser origin for this install (onboarding hints).
	PublicAppURL string
}

// Instance GET /api/v1/public/instance
func (h *PublicHandler) Instance(w http.ResponseWriter, r *http.Request) {
	prov := strings.TrimSpace(h.ProvisioningBaseURL) != "" &&
		strings.TrimSpace(h.ProvisioningInstallID) != "" &&
		strings.TrimSpace(h.ProvisioningInstallSecret) != ""
	out := map[string]any{
		"provisioning_enabled": prov,
		"version":              version.Version,
		"git_sha":              version.GitSHA,
	}
	if prov {
		out["provisioning_base_domain"] = provisioning.GiftedSubdomainApex
	}
	if r := strings.TrimSpace(h.UpstreamGitHubRepo); r != "" {
		out["upstream_github_repo"] = r
	}
	if u := strings.TrimSpace(h.UpdateManifestURL); u != "" {
		out["update_manifest_url"] = u
	}
	if u := strings.TrimSpace(h.PublicAppURL); u != "" {
		out["public_app_url"] = u
	}
	if h.Store != nil {
		if n, err := h.Store.CountOrganizations(r.Context()); err == nil {
			out["needs_organization_setup"] = n == 0
		}
	}
	httpx.JSON(w, http.StatusOK, out)
}
