package rest

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
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
	"hyperspeed/api/internal/store"
)

const (
	maxPreviewTotalBytes = 32 << 20
	maxPreviewFileBytes  = 64 << 20
	previewSessionTTL    = 1 * time.Hour
)

// PreviewHandler implements Phase 2 stub: token-gated static snapshot of space files (no external runner).
type PreviewHandler struct {
	Store      *store.Store
	OS         *files.ObjectStore
	PublicBase string // optional absolute origin for preview URLs (e.g. https://api.example.com)
}

type previewSessionCreateBody struct {
	Command *string `json:"command"`
	Cwd     *string `json:"cwd"`
}

type previewSessionJSON struct {
	ID          string  `json:"id"`
	Status      string  `json:"status"`
	PreviewURL  string  `json:"preview_url"`
	ExpiresAt   string  `json:"expires_at"`
	ErrorMessage *string `json:"error_message,omitempty"`
	Command     *string `json:"command,omitempty"`
	Cwd         *string `json:"cwd,omitempty"`
}

func publicBaseURL(r *http.Request, cfg string) string {
	cfg = strings.TrimSpace(cfg)
	if cfg != "" {
		return strings.TrimRight(cfg, "/")
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	host := r.Host
	if host == "" {
		host = "127.0.0.1:8080"
	}
	return scheme + "://" + host
}

// effectivePublicBase prefers DB-backed public_origin_override (workspace settings), then env, then the request.
func (h *PreviewHandler) effectivePublicBase(r *http.Request) string {
	if h.Store != nil {
		ov, err := h.Store.GetSingletonPublicOriginOverride(r.Context())
		if err == nil && ov != nil && strings.TrimSpace(*ov) != "" {
			return strings.TrimRight(strings.TrimSpace(*ov), "/")
		}
	}
	return publicBaseURL(r, h.PublicBase)
}

func randomAccessToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func buildFilePathMap(nodes []store.FileNode) map[uuid.UUID]store.FileNode {
	byID := make(map[uuid.UUID]store.FileNode, len(nodes))
	for _, n := range nodes {
		byID[n.ID] = n
	}
	return byID
}

func fileNodeRelPath(n store.FileNode, byID map[uuid.UUID]store.FileNode) string {
	var parts []string
	cur := n
	for {
		parts = append([]string{cur.Name}, parts...)
		if cur.ParentID == nil {
			break
		}
		p, ok := byID[*cur.ParentID]
		if !ok {
			break
		}
		cur = p
	}
	return strings.Join(parts, "/")
}

// buildPreviewSnapshot loads text-like files from object storage into a path -> base64(raw) map.
func (h *PreviewHandler) buildPreviewSnapshot(ctx context.Context, spaceID uuid.UUID) (map[string]string, error) {
	if h.OS == nil {
		return nil, fmt.Errorf("object store not configured")
	}
	nodes, err := h.Store.ListAllFileNodes(ctx, spaceID)
	if err != nil {
		return nil, err
	}
	byID := buildFilePathMap(nodes)
	snap := make(map[string]string)
	var total int64
	for _, n := range nodes {
		if n.Kind != store.FileNodeFile || n.StorageKey == nil || *n.StorageKey == "" {
			continue
		}
		p := fileNodeRelPath(n, byID)
		if p == "" || strings.Contains(p, "..") {
			continue
		}
		rc, sz, err := h.OS.GetObjectStream(ctx, *n.StorageKey)
		if err != nil {
			continue
		}
		if sz > maxPreviewFileBytes {
			_ = rc.Close()
			continue
		}
		if total+sz > maxPreviewTotalBytes {
			_ = rc.Close()
			break
		}
		data, err := io.ReadAll(io.LimitReader(rc, maxPreviewFileBytes))
		_ = rc.Close()
		if err != nil {
			continue
		}
		total += int64(len(data))
		snap[p] = base64.StdEncoding.EncodeToString(data)
		if total >= maxPreviewTotalBytes {
			break
		}
	}
	return snap, nil
}

func decodeSnapshot(snapJSON json.RawMessage) (map[string]string, error) {
	if len(snapJSON) == 0 {
		return map[string]string{}, nil
	}
	var m map[string]string
	if err := json.Unmarshal(snapJSON, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// CreateSession POST /organizations/{orgID}/spaces/{spaceID}/preview/sessions
func (h *PreviewHandler) CreateSession(w http.ResponseWriter, r *http.Request) {
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
	var body previewSessionCreateBody
	_ = httpx.DecodeJSON(r, &body)

	token, err := randomAccessToken()
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "token")
		return
	}
	expires := time.Now().UTC().Add(previewSessionTTL)

	snap, err := h.buildPreviewSnapshot(r.Context(), spaceID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "snapshot")
		return
	}

	row, err := h.Store.InsertPreviewSession(r.Context(), spaceID, uid, store.PreviewSessionRunning, body.Command, body.Cwd, token, snap, expires)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "create session")
		return
	}

	base := h.effectivePublicBase(r)
	previewURL := fmt.Sprintf("%s/api/v1/organizations/%s/spaces/%s/preview/sessions/%s/content/?token=%s",
		base, orgID.String(), spaceID.String(), row.ID.String(), url.QueryEscape(token))

	out := previewSessionJSON{
		ID:         row.ID.String(),
		Status:     string(row.Status),
		PreviewURL: previewURL,
		ExpiresAt:  row.ExpiresAt.UTC().Format(time.RFC3339),
		Command:    row.Command,
		Cwd:        row.Cwd,
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"session": out})
}

// GetSession GET .../preview/sessions/{sessionID}
func (h *PreviewHandler) GetSession(w http.ResponseWriter, r *http.Request) {
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
	sessionID, err := uuid.Parse(chi.URLParam(r, "sessionID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid session id")
		return
	}
	if _, err := h.Store.GetSpace(r.Context(), orgID, spaceID); err != nil {
		httpx.Error(w, http.StatusNotFound, "space not found")
		return
	}
	row, err := h.Store.GetPreviewSession(r.Context(), spaceID, sessionID)
	if err != nil {
		if err == pgx.ErrNoRows {
			httpx.Error(w, http.StatusNotFound, "session not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "session")
		return
	}
	if row.CreatedBy != uid {
		httpx.Error(w, http.StatusForbidden, "forbidden")
		return
	}
	if time.Now().UTC().After(row.ExpiresAt) {
		_ = h.Store.UpdatePreviewSessionStatus(r.Context(), spaceID, sessionID, store.PreviewSessionExpired, nil)
		row.Status = store.PreviewSessionExpired
	}

	base := h.effectivePublicBase(r)
	previewURL := fmt.Sprintf("%s/api/v1/organizations/%s/spaces/%s/preview/sessions/%s/content/?token=%s",
		base, orgID.String(), spaceID.String(), row.ID.String(), url.QueryEscape(row.AccessToken))

	out := previewSessionJSON{
		ID:           row.ID.String(),
		Status:       string(row.Status),
		PreviewURL:   previewURL,
		ExpiresAt:    row.ExpiresAt.UTC().Format(time.RFC3339),
		ErrorMessage: row.ErrorMessage,
		Command:      row.Command,
		Cwd:          row.Cwd,
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"session": out})
}

// DeleteSession DELETE .../preview/sessions/{sessionID}
func (h *PreviewHandler) DeleteSession(w http.ResponseWriter, r *http.Request) {
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
	sessionID, err := uuid.Parse(chi.URLParam(r, "sessionID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid session id")
		return
	}
	row, err := h.Store.GetPreviewSession(r.Context(), spaceID, sessionID)
	if err != nil {
		if err == pgx.ErrNoRows {
			httpx.Error(w, http.StatusNotFound, "session not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "session")
		return
	}
	if row.CreatedBy != uid {
		httpx.Error(w, http.StatusForbidden, "forbidden")
		return
	}
	okDel, err := h.Store.DeletePreviewSession(r.Context(), spaceID, sessionID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "delete")
		return
	}
	if !okDel {
		httpx.Error(w, http.StatusNotFound, "session not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ServeContent GET .../preview/sessions/{sessionID}/content/* — token query param; no JWT (iframe).
func (h *PreviewHandler) ServeContent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		httpx.Error(w, http.StatusMethodNotAllowed, "method")
		return
	}
	orgID, err := uuid.Parse(chi.URLParam(r, "orgID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid org id")
		return
	}
	spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid space id")
		return
	}
	sessionID, err := uuid.Parse(chi.URLParam(r, "sessionID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid session id")
		return
	}
	tok := strings.TrimSpace(r.URL.Query().Get("token"))
	if tok == "" {
		httpx.Error(w, http.StatusUnauthorized, "token required")
		return
	}

	row, err := h.Store.GetPreviewSession(r.Context(), spaceID, sessionID)
	if err != nil {
		if err == pgx.ErrNoRows {
			httpx.Error(w, http.StatusNotFound, "session not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "session")
		return
	}
	if row.AccessToken != tok {
		httpx.Error(w, http.StatusUnauthorized, "invalid token")
		return
	}
	if time.Now().UTC().After(row.ExpiresAt) {
		httpx.Error(w, http.StatusGone, "session expired")
		return
	}
	if row.Status != store.PreviewSessionRunning {
		httpx.Error(w, http.StatusGone, "session not available")
		return
	}
	// Verify org owns space
	if _, err := h.Store.GetSpace(r.Context(), orgID, spaceID); err != nil {
		httpx.Error(w, http.StatusNotFound, "not found")
		return
	}

	snap, err := decodeSnapshot(row.SnapshotJSON)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "snapshot")
		return
	}

	rel := strings.TrimPrefix(strings.TrimSpace(chi.URLParam(r, "*")), "/")
	if rel == "" {
		prefix := fmt.Sprintf("/api/v1/organizations/%s/spaces/%s/preview/sessions/%s/content",
			orgID.String(), spaceID.String(), sessionID.String())
		rel = strings.TrimPrefix(r.URL.Path, prefix)
		rel = strings.TrimPrefix(rel, "/")
	}
	if rel == "" {
		if _, ok := snap["index.html"]; ok {
			rel = "index.html"
		} else {
			for k := range snap {
				if strings.HasSuffix(strings.ToLower(k), "index.html") {
					rel = k
					break
				}
			}
		}
		if rel == "" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Content-Security-Policy", "frame-ancestors *")
			w.WriteHeader(http.StatusOK)
			if r.Method == http.MethodGet {
				_, _ = w.Write([]byte("<!DOCTYPE html><html><head><meta charset=\"utf-8\"><title>Preview</title></head><body><p>No index.html in snapshot.</p></body></html>"))
			}
			return
		}
	}

	rel = path.Clean(rel)
	if strings.HasPrefix(rel, "..") {
		httpx.Error(w, http.StatusBadRequest, "path")
		return
	}

	b64, ok := snap[rel]
	if !ok {
		http.NotFound(w, r)
		return
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "decode")
		return
	}
	ctype := mime.TypeByExtension(path.Ext(rel))
	if ctype == "" {
		ctype = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ctype)
	// Allow embedding from configured app origin(s) — default dev-friendly.
	w.Header().Set("Content-Security-Policy", "frame-ancestors *")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodGet {
		_, _ = w.Write(raw)
	}
}
