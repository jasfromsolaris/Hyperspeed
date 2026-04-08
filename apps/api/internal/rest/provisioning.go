package rest

import (
	"net/http"
	"strings"

	"hyperspeed/api/internal/httpx"
	"hyperspeed/api/internal/store"
	"hyperspeed/api/internal/version"
)

// PublicHandler exposes non-secret instance metadata for the SPA.
type PublicHandler struct {
	Store *store.Store
	// UpstreamGitHubRepo is optional "owner/name" for SPA update checks against GitHub releases.
	UpstreamGitHubRepo string
	// UpdateManifestURL is optional HTTPS URL to a static JSON manifest for update checks.
	UpdateManifestURL string
	// PublicAppURL is optional browser origin for this install (onboarding hints).
	PublicAppURL string
}

// Instance GET /api/v1/public/instance
func (h *PublicHandler) Instance(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{
		"version": version.Version,
		"git_sha": version.GitSHA,
	}
	if repo := strings.TrimSpace(h.UpstreamGitHubRepo); repo != "" {
		out["upstream_github_repo"] = repo
	}
	if manifestURL := strings.TrimSpace(h.UpdateManifestURL); manifestURL != "" {
		out["update_manifest_url"] = manifestURL
	}
	if appURL := strings.TrimSpace(h.PublicAppURL); appURL != "" {
		out["public_app_url"] = appURL
	}
	if h.Store != nil {
		if n, err := h.Store.CountOrganizations(r.Context()); err == nil {
			out["needs_organization_setup"] = n == 0
		}
	}
	httpx.JSON(w, http.StatusOK, out)
}
