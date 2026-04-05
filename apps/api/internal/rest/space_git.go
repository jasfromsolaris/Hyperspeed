package rest

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"hyperspeed/api/internal/ctxkey"
	"hyperspeed/api/internal/files"
	"hyperspeed/api/internal/httpx"
	"hyperspeed/api/internal/middleware"
	"hyperspeed/api/internal/rbac"
	"hyperspeed/api/internal/secrets"
	"hyperspeed/api/internal/spacegit"
	"hyperspeed/api/internal/store"
)

// SpaceGitHandler wires IDE Git integration (HTTPS + PAT).
type SpaceGitHandler struct {
	Store         *store.Store
	OS            *files.ObjectStore
	EncryptKeyB64 string
	GitWorkdirBase string
}

func (h *SpaceGitHandler) gitKey() ([]byte, error) {
	return decodeAppSecretsKey(h.EncryptKeyB64)
}

func (h *SpaceGitHandler) gitIntegrationAvailable() bool {
	return strings.TrimSpace(h.GitWorkdirBase) != ""
}

func (h *SpaceGitHandler) enabled(w http.ResponseWriter) bool {
	if !h.gitIntegrationAvailable() {
		httpx.JSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "git integration disabled (set HS_GIT_WORKDIR_BASE on the API)",
			"code":  "git_disabled",
		})
		return false
	}
	return true
}

// Get returns link metadata (no token).
func (h *SpaceGitHandler) Get(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.FilesRead) {
		return
	}
	spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid space id")
		return
	}
	if _, err := h.Store.GetSpace(r.Context(), orgID, spaceID); err != nil {
		httpx.Error(w, http.StatusNotFound, "space not found")
		return
	}
	link, err := h.Store.GetSpaceGitLink(r.Context(), spaceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.JSON(w, http.StatusOK, map[string]any{
				"git_link":                    nil,
				"git_integration_available": h.gitIntegrationAvailable(),
			})
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "git link")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"git_link":                    h.enrichGitLink(r.Context(), spaceID, link),
		"git_integration_available": h.gitIntegrationAvailable(),
	})
}

type putSpaceGitBody struct {
	RemoteURL    string     `json:"remote_url"`
	Branch       string     `json:"branch"`
	RootFolderID *uuid.UUID `json:"root_folder_id"`
	AccessToken  string     `json:"access_token"`
}

// Put creates or updates the git link (org.manage). Token optional on update to keep existing.
func (h *SpaceGitHandler) Put(w http.ResponseWriter, r *http.Request) {
	if !h.enabled(w) {
		return
	}
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.OrgManage) {
		return
	}
	key32, err := h.gitKey()
	if err != nil || len(key32) != 32 {
		httpx.JSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "HS_SSH_ENCRYPTION_KEY not configured",
			"code":  "secrets_not_configured",
		})
		return
	}
	spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid space id")
		return
	}
	if _, err := h.Store.GetSpace(r.Context(), orgID, spaceID); err != nil {
		httpx.Error(w, http.StatusNotFound, "space not found")
		return
	}
	var body putSpaceGitBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	body.RemoteURL = strings.TrimSpace(body.RemoteURL)
	body.Branch = strings.TrimSpace(body.Branch)
	if body.Branch == "" {
		body.Branch = "main"
	}
	if body.RemoteURL == "" || !strings.HasPrefix(body.RemoteURL, "https://") {
		httpx.Error(w, http.StatusBadRequest, "remote_url must be an https:// Git URL")
		return
	}
	if body.RootFolderID != nil {
		fn, err := h.Store.FileNodeByID(r.Context(), spaceID, *body.RootFolderID)
		if err != nil || fn.Kind != store.FileNodeFolder || fn.DeletedAt != nil {
			httpx.Error(w, http.StatusBadRequest, "invalid root_folder_id")
			return
		}
	}

	existing, err := h.Store.GetSpaceGitLink(r.Context(), spaceID)
	hasExisting := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, http.StatusInternalServerError, "git link")
		return
	}

	var tokCipher *string
	var tokLast4 *string
	tok := strings.TrimSpace(body.AccessToken)
	if tok != "" {
		enc, err := secrets.EncryptString(key32, tok)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "encrypt token")
			return
		}
		tokCipher = &enc
		last4 := tok
		if len(last4) > 4 {
			last4 = last4[len(last4)-4:]
		}
		tokLast4 = &last4
	} else if hasExisting {
		tokCipher = existing.TokenCiphertext
		tokLast4 = existing.TokenLast4
	} else {
		// Public HTTPS clone: allow empty token (no encrypted secret stored).
		tokCipher = nil
		tokLast4 = nil
	}

	link := store.SpaceGitLink{
		SpaceID:         spaceID,
		RemoteURL:       body.RemoteURL,
		Branch:          body.Branch,
		RootFolderID:    body.RootFolderID,
		TokenCiphertext: tokCipher,
		TokenLast4:      tokLast4,
	}
	if err := h.Store.UpsertSpaceGitLink(r.Context(), link); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "save git link")
		return
	}
	saved, _ := h.Store.GetSpaceGitLink(r.Context(), spaceID)
	httpx.JSON(w, http.StatusOK, map[string]any{"git_link": h.enrichGitLink(r.Context(), spaceID, saved)})
}

// Delete removes the git link.
func (h *SpaceGitHandler) Delete(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.OrgManage) {
		return
	}
	spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid space id")
		return
	}
	if _, err := h.Store.GetSpace(r.Context(), orgID, spaceID); err != nil {
		httpx.Error(w, http.StatusNotFound, "space not found")
		return
	}
	if err := h.Store.DeleteSpaceGitLink(r.Context(), spaceID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.Error(w, http.StatusNotFound, "no git link")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "delete git link")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Test checks remote reachability (files.write).
func (h *SpaceGitHandler) Test(w http.ResponseWriter, r *http.Request) {
	if !h.enabled(w) {
		return
	}
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.FilesWrite) {
		return
	}
	key32, err := h.gitKey()
	if err != nil || len(key32) != 32 {
		httpx.JSON(w, http.StatusServiceUnavailable, map[string]any{"error": "encryption key not configured"})
		return
	}
	spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid space id")
		return
	}
	if _, err := h.Store.GetSpace(r.Context(), orgID, spaceID); err != nil {
		httpx.Error(w, http.StatusNotFound, "space not found")
		return
	}
	link, err := h.Store.GetSpaceGitLink(r.Context(), spaceID)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "configure git link first")
		return
	}
	tok, err := decryptToken(link, key32)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "stored token invalid; re-save access token")
		return
	}
	authed, err := spacegit.AuthedHTTPSURL(link.RemoteURL, tok)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := spacegit.LsRemote(r.Context(), authed, link.Branch); err != nil {
		_ = h.Store.UpdateSpaceGitLinkSyncError(r.Context(), spaceID, err.Error())
		httpx.Error(w, http.StatusBadGateway, "remote: "+err.Error())
		return
	}
	_ = h.Store.UpdateSpaceGitLinkSyncError(r.Context(), spaceID, "")
	httpx.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

type pushBody struct {
	Message string `json:"message"`
}

// Pull fetches from remote and imports into the space.
func (h *SpaceGitHandler) Pull(w http.ResponseWriter, r *http.Request) {
	if !h.enabled(w) {
		return
	}
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.FilesWrite) {
		return
	}
	key32, err := h.gitKey()
	if err != nil || len(key32) != 32 {
		httpx.JSON(w, http.StatusServiceUnavailable, map[string]any{"error": "encryption key not configured"})
		return
	}
	spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid space id")
		return
	}
	if _, err := h.Store.GetSpace(r.Context(), orgID, spaceID); err != nil {
		httpx.Error(w, http.StatusNotFound, "space not found")
		return
	}
	if h.OS == nil {
		httpx.Error(w, http.StatusInternalServerError, "object store not configured")
		return
	}
	link, err := h.Store.GetSpaceGitLink(r.Context(), spaceID)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "configure git link first")
		return
	}
	tok, err := decryptToken(link, key32)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "token decrypt failed")
		return
	}
	authed, err := spacegit.AuthedHTTPSURL(link.RemoteURL, tok)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	repoDir := h.repoDir(spaceID)
	if err := spacegit.EnsureRepo(r.Context(), authed, link.Branch, repoDir); err != nil {
		_ = h.Store.UpdateSpaceGitLinkSyncError(r.Context(), spaceID, err.Error())
		httpx.Error(w, http.StatusBadGateway, err.Error())
		return
	}
	n, err := spacegit.ImportRepoDirIntoSpace(r.Context(), h.Store, h.OS, orgID, spaceID, uid, link.RootFolderID, repoDir)
	if err != nil {
		_ = h.Store.UpdateSpaceGitLinkSyncError(r.Context(), spaceID, err.Error())
		httpx.Error(w, http.StatusBadGateway, err.Error())
		return
	}
	sha, err := spacegit.HeadSHA(r.Context(), repoDir)
	if err != nil {
		sha = ""
	}
	_ = h.Store.UpdateSpaceGitLinkSyncOK(r.Context(), spaceID, sha, time.Now().UTC())
	httpx.JSON(w, http.StatusOK, map[string]any{"ok": true, "imported_files": n, "head_sha": sha})
}

// Push exports the space tree and pushes to origin.
func (h *SpaceGitHandler) Push(w http.ResponseWriter, r *http.Request) {
	if !h.enabled(w) {
		return
	}
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.FilesWrite) {
		return
	}
	key32, err := h.gitKey()
	if err != nil || len(key32) != 32 {
		httpx.JSON(w, http.StatusServiceUnavailable, map[string]any{"error": "encryption key not configured"})
		return
	}
	spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid space id")
		return
	}
	if _, err := h.Store.GetSpace(r.Context(), orgID, spaceID); err != nil {
		httpx.Error(w, http.StatusNotFound, "space not found")
		return
	}
	if h.OS == nil {
		httpx.Error(w, http.StatusInternalServerError, "object store not configured")
		return
	}
	var body pushBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	body.Message = strings.TrimSpace(body.Message)
	if body.Message == "" {
		body.Message = "Sync from Hyperspeed"
	}
	link, err := h.Store.GetSpaceGitLink(r.Context(), spaceID)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "configure git link first")
		return
	}
	tok, err := decryptToken(link, key32)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "token decrypt failed")
		return
	}
	authed, err := spacegit.AuthedHTTPSURL(link.RemoteURL, tok)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	repoDir := h.repoDir(spaceID)
	if err := spacegit.EnsureRepo(r.Context(), authed, link.Branch, repoDir); err != nil {
		_ = h.Store.UpdateSpaceGitLinkSyncError(r.Context(), spaceID, err.Error())
		httpx.Error(w, http.StatusBadGateway, err.Error())
		return
	}
	if err := spacegit.WipeWorktree(repoDir); err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := spacegit.ExportSpaceTreeToDir(r.Context(), h.Store, h.OS, spaceID, link.RootFolderID, repoDir); err != nil {
		_ = h.Store.UpdateSpaceGitLinkSyncError(r.Context(), spaceID, err.Error())
		httpx.Error(w, http.StatusBadGateway, err.Error())
		return
	}
	spacegit.ConfigureLocalUser(r.Context(), repoDir)
	if err := spacegit.CommitAll(r.Context(), repoDir, body.Message); err != nil {
		_ = h.Store.UpdateSpaceGitLinkSyncError(r.Context(), spaceID, err.Error())
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := spacegit.Push(r.Context(), repoDir, link.Branch, authed); err != nil {
		_ = h.Store.UpdateSpaceGitLinkSyncError(r.Context(), spaceID, err.Error())
		httpx.Error(w, http.StatusBadGateway, err.Error())
		return
	}
	sha, _ := spacegit.HeadSHA(r.Context(), repoDir)
	_ = h.Store.UpdateSpaceGitLinkSyncOK(r.Context(), spaceID, sha, time.Now().UTC())
	httpx.JSON(w, http.StatusOK, map[string]any{"ok": true, "head_sha": sha})
}

func (h *SpaceGitHandler) repoDir(spaceID uuid.UUID) string {
	base := strings.TrimSpace(h.GitWorkdirBase)
	if base == "" {
		return filepath.Join(spaceID.String(), "repo")
	}
	return filepath.Join(base, spaceID.String(), "repo")
}

func (h *SpaceGitHandler) enrichGitLink(ctx context.Context, spaceID uuid.UUID, link store.SpaceGitLink) map[string]any {
	gl := publicGitLink(link)
	repoDir := h.repoDir(spaceID)
	if st, err := os.Stat(filepath.Join(repoDir, ".git")); err == nil && st.IsDir() {
		gl["workdir_ready"] = true
		if sha, err := spacegit.HeadSHA(ctx, repoDir); err == nil {
			gl["local_head_sha"] = sha
		}
	} else {
		gl["workdir_ready"] = false
	}
	return gl
}

func publicGitLink(l store.SpaceGitLink) map[string]any {
	m := map[string]any{
		"space_id":          l.SpaceID,
		"remote_url":        l.RemoteURL,
		"branch":            l.Branch,
		"root_folder_id":    l.RootFolderID,
		"token_last4":       l.TokenLast4,
		"last_commit_sha":   l.LastCommitSHA,
		"last_error":        l.LastError,
		"last_sync_at":      l.LastSyncAt,
		"created_at":        l.CreatedAt,
		"updated_at":        l.UpdatedAt,
	}
	return m
}

func decryptToken(link store.SpaceGitLink, key32 []byte) (string, error) {
	if link.TokenCiphertext == nil || *link.TokenCiphertext == "" {
		return "", nil
	}
	return secrets.DecryptString(key32, *link.TokenCiphertext)
}
