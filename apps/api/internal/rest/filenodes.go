package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"

	"hyperspeed/api/internal/auth"
	"hyperspeed/api/internal/ctxkey"
	"hyperspeed/api/internal/events"
	"hyperspeed/api/internal/files"
	"hyperspeed/api/internal/httpx"
	"hyperspeed/api/internal/middleware"
	"hyperspeed/api/internal/rbac"
	"hyperspeed/api/internal/store"
)

type FileNodeHandler struct {
	Store *store.Store
	OS    *files.ObjectStore
	Auth  *auth.Service
	Rdb   *redis.Client
	Bus   *events.Bus
}

func (h *FileNodeHandler) publishFileTree(orgID, spaceID uuid.UUID) {
	if h.Bus == nil {
		return
	}
	sid := spaceID
	payload, err := events.Marshal(events.FileTreeUpdated, orgID, &sid, map[string]string{
		"space_id": sid.String(),
	})
	if err != nil {
		return
	}
	_ = h.Bus.Publish(context.Background(), orgID, payload)
}

func (h *FileNodeHandler) Tree(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.FilesRead) {
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
	list, err := h.Store.ListAllFileNodes(r.Context(), pid)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "files")
		return
	}
	if list == nil {
		list = []store.FileNode{}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"nodes": list})
}

func (h *FileNodeHandler) GetNode(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.FilesRead) {
		return
	}
	pid, err := uuid.Parse(chi.URLParam(r, "spaceID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid space id")
		return
	}
	nodeID, err := uuid.Parse(chi.URLParam(r, "nodeID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid node id")
		return
	}
	if _, err := h.Store.GetSpace(r.Context(), orgID, pid); err != nil {
		httpx.Error(w, http.StatusNotFound, "space not found")
		return
	}
	n, err := h.Store.FileNodeByID(r.Context(), pid, nodeID)
	if err != nil {
		if err == pgx.ErrNoRows {
			httpx.Error(w, http.StatusNotFound, "node not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "node")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"node": n})
}

func (h *FileNodeHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.FilesRead) {
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

	var parentID *uuid.UUID
	if v := strings.TrimSpace(r.URL.Query().Get("parentId")); v != "" {
		p, err := uuid.Parse(v)
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "invalid parent id")
			return
		}
		parentID = &p
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	scope := strings.TrimSpace(r.URL.Query().Get("scope")) // "folder" (default) or "space"

	if q != "" && scope == "space" {
		list, err := h.Store.SearchFileNodesInProject(r.Context(), pid, q)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "search files")
			return
		}
		if list == nil {
			list = []store.FileNode{}
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"nodes": list})
		return
	}

	list, err := h.Store.ListFileNodes(r.Context(), pid, parentID, q, false)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "list files")
		return
	}
	if list == nil {
		list = []store.FileNode{}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"nodes": list})
}

type createFolderBody struct {
	ParentID *uuid.UUID `json:"parent_id"`
	Name     string     `json:"name"`
}

func (h *FileNodeHandler) CreateFolder(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.FilesWrite) {
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
	var body createFolderBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" {
		httpx.Error(w, http.StatusBadRequest, "name required")
		return
	}
	n, err := h.Store.CreateFolderNode(r.Context(), pid, body.ParentID, body.Name, uid)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "create folder")
		return
	}
	h.publishFileTree(orgID, pid)
	httpx.JSON(w, http.StatusCreated, map[string]any{"node": n})
}

type patchNodeBody struct {
	Name     *string    `json:"name"`
	ParentID optionalUUID `json:"parent_id"`
}

// optionalUUID tracks whether a UUID field was present in JSON.
// This lets us distinguish:
// - field absent (no change)
// - field present as null (set to NULL/root)
// - field present as UUID (set to that UUID)
type optionalUUID struct {
	Set   bool
	Value *uuid.UUID
}

func (o *optionalUUID) UnmarshalJSON(b []byte) error {
	o.Set = true
	// JSON null means explicitly clear the value.
	if string(b) == "null" {
		o.Value = nil
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	id, err := uuid.Parse(strings.TrimSpace(s))
	if err != nil {
		return err
	}
	o.Value = &id
	return nil
}

func (h *FileNodeHandler) Patch(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.FilesWrite) {
		return
	}
	pid, err := uuid.Parse(chi.URLParam(r, "spaceID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid space id")
		return
	}
	nodeID, err := uuid.Parse(chi.URLParam(r, "nodeID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid node id")
		return
	}
	if _, err := h.Store.GetSpace(r.Context(), orgID, pid); err != nil {
		httpx.Error(w, http.StatusNotFound, "space not found")
		return
	}
	var body patchNodeBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	treeChanged := false
	if body.Name != nil {
		n := strings.TrimSpace(*body.Name)
		if n == "" {
			httpx.Error(w, http.StatusBadRequest, "name required")
			return
		}
		ok, err := h.Store.RenameFileNode(r.Context(), pid, nodeID, n)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "rename")
			return
		}
		if !ok {
			httpx.Error(w, http.StatusNotFound, "node not found")
			return
		}
		treeChanged = true
	}
	if body.ParentID.Set {
		ok, err := h.Store.MoveFileNode(r.Context(), pid, nodeID, body.ParentID.Value)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "move")
			return
		}
		if !ok {
			httpx.Error(w, http.StatusNotFound, "node not found")
			return
		}
		treeChanged = true
	}
	if treeChanged {
		h.publishFileTree(orgID, pid)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *FileNodeHandler) Delete(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, hasUser := ctxkey.UserID(r.Context())
	if !hasUser {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.FilesDelete) {
		return
	}
	pid, err := uuid.Parse(chi.URLParam(r, "spaceID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid space id")
		return
	}
	nodeID, err := uuid.Parse(chi.URLParam(r, "nodeID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid node id")
		return
	}
	if _, err := h.Store.GetSpace(r.Context(), orgID, pid); err != nil {
		httpx.Error(w, http.StatusNotFound, "space not found")
		return
	}
	deleted, err := h.Store.SoftDeleteFileNode(r.Context(), pid, nodeID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "delete")
		return
	}
	if !deleted {
		httpx.Error(w, http.StatusNotFound, "node not found")
		return
	}
	h.publishFileTree(orgID, pid)
	w.WriteHeader(http.StatusNoContent)
}

type uploadInitBody struct {
	ParentID  *uuid.UUID `json:"parent_id"`
	Name      string     `json:"name"`
	MimeType  *string    `json:"mime_type"`
	SizeBytes *int64     `json:"size_bytes"`
}

func (h *FileNodeHandler) UploadInit(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.FilesWrite) {
		return
	}
	if isSA, _ := h.Store.ServiceAccountInOrg(r.Context(), orgID, uid); isSA {
		httpx.Error(w, http.StatusForbidden, "service accounts cannot upload file contents directly; use edit proposals or agent tools")
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
	if h.OS == nil {
		httpx.Error(w, http.StatusInternalServerError, "object store not configured")
		return
	}
	var body uploadInitBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" {
		httpx.Error(w, http.StatusBadRequest, "name required")
		return
	}
	// Create metadata first to get node ID; use it in key for uniqueness.
	storageKey := "org/" + orgID.String() + "/project/" + pid.String() + "/node/" + uuid.NewString() + "/" + body.Name
	n, err := h.Store.CreateFileNode(r.Context(), pid, body.ParentID, body.Name, body.MimeType, body.SizeBytes, storageKey, uid)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "create file")
		return
	}
	ct := ""
	if body.MimeType != nil {
		ct = *body.MimeType
	}
	uploadURL, err := h.OS.PresignPut(r.Context(), storageKey, ct, 10*time.Minute)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "presign upload")
		return
	}
	h.publishFileTree(orgID, pid)
	httpx.JSON(w, http.StatusOK, map[string]any{
		"node":       n,
		"upload_url": uploadURL,
		"upload_via_api_url": "/api/v1/organizations/" + orgID.String() + "/spaces/" + pid.String() + "/files/" + n.ID.String() + "/upload",
	})
}

type uploadCompleteBody struct {
	NodeID uuid.UUID `json:"node_id"`
}

func (h *FileNodeHandler) UploadComplete(w http.ResponseWriter, r *http.Request) {
	// Reserved for future: verify checksum/size, set edited fields, etc.
	var body uploadCompleteBody
	_ = httpx.DecodeJSON(r, &body)
	httpx.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *FileNodeHandler) Download(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.FilesRead) {
		return
	}
	pid, err := uuid.Parse(chi.URLParam(r, "spaceID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid space id")
		return
	}
	nodeID, err := uuid.Parse(chi.URLParam(r, "nodeID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid node id")
		return
	}
	if _, err := h.Store.GetSpace(r.Context(), orgID, pid); err != nil {
		httpx.Error(w, http.StatusNotFound, "space not found")
		return
	}
	if h.OS == nil {
		httpx.Error(w, http.StatusInternalServerError, "object store not configured")
		return
	}
	n, err := h.Store.FileNodeByID(r.Context(), pid, nodeID)
	if err != nil {
		if err == pgx.ErrNoRows {
			httpx.Error(w, http.StatusNotFound, "node not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "node")
		return
	}
	if n.Kind != store.FileNodeFile || n.StorageKey == nil || *n.StorageKey == "" {
		httpx.Error(w, http.StatusBadRequest, "not a file")
		return
	}
	u, err := h.OS.PresignGet(r.Context(), *n.StorageKey, 10*time.Minute)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "presign download")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"download_url": u})
}

func (h *FileNodeHandler) UploadViaAPI(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.FilesWrite) {
		return
	}
	if isSA, _ := h.Store.ServiceAccountInOrg(r.Context(), orgID, uid); isSA {
		httpx.Error(w, http.StatusForbidden, "service accounts cannot upload file contents directly; use edit proposals or agent tools")
		return
	}
	pid, err := uuid.Parse(chi.URLParam(r, "spaceID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid space id")
		return
	}
	nodeID, err := uuid.Parse(chi.URLParam(r, "nodeID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid node id")
		return
	}
	if _, err := h.Store.GetSpace(r.Context(), orgID, pid); err != nil {
		httpx.Error(w, http.StatusNotFound, "space not found")
		return
	}
	if h.OS == nil {
		httpx.Error(w, http.StatusInternalServerError, "object store not configured")
		return
	}
	n, err := h.Store.FileNodeByID(r.Context(), pid, nodeID)
	if err != nil {
		if err == pgx.ErrNoRows {
			httpx.Error(w, http.StatusNotFound, "node not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "node")
		return
	}
	if n.Kind != store.FileNodeFile || n.StorageKey == nil || *n.StorageKey == "" {
		httpx.Error(w, http.StatusBadRequest, "not a file")
		return
	}
	ct := r.Header.Get("Content-Type")
	var size *int64
	if r.ContentLength >= 0 {
		s := r.ContentLength
		size = &s
	}
	if err := h.OS.Put(r.Context(), *n.StorageKey, ct, io.LimitReader(r.Body, 1024*1024*1024), size); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "upload")
		return
	}
	h.publishFileTree(orgID, pid)
	httpx.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

type createTextFileBody struct {
	ParentID *uuid.UUID `json:"parent_id"`
	Name     string     `json:"name"`
}

func (h *FileNodeHandler) CreateTextFile(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.FilesWrite) {
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
	if h.OS == nil {
		httpx.Error(w, http.StatusInternalServerError, "object store not configured")
		return
	}

	var body createTextFileBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" {
		httpx.Error(w, http.StatusBadRequest, "name required")
		return
	}
	mime := "text/plain"
	size := int64(0)
	storageKey := "org/" + orgID.String() + "/project/" + pid.String() + "/node/" + uuid.NewString() + "/" + body.Name
	n, err := h.Store.CreateFileNode(r.Context(), pid, body.ParentID, body.Name, &mime, &size, storageKey, uid)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "create file")
		return
	}
	// Create empty object immediately.
	if err := h.OS.Put(r.Context(), storageKey, mime, bytes.NewReader(nil), &size); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "write file")
		return
	}
	h.publishFileTree(orgID, pid)
	httpx.JSON(w, http.StatusCreated, map[string]any{"node": n})
}

func (h *FileNodeHandler) GetText(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.FilesRead) {
		return
	}
	pid, err := uuid.Parse(chi.URLParam(r, "spaceID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid space id")
		return
	}
	nodeID, err := uuid.Parse(chi.URLParam(r, "nodeID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid node id")
		return
	}
	if _, err := h.Store.GetSpace(r.Context(), orgID, pid); err != nil {
		httpx.Error(w, http.StatusNotFound, "space not found")
		return
	}
	if h.OS == nil {
		httpx.Error(w, http.StatusInternalServerError, "object store not configured")
		return
	}
	n, err := h.Store.FileNodeByID(r.Context(), pid, nodeID)
	if err != nil {
		if err == pgx.ErrNoRows {
			httpx.Error(w, http.StatusNotFound, "node not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "node")
		return
	}
	if n.Kind != store.FileNodeFile || n.StorageKey == nil || *n.StorageKey == "" || n.DeletedAt != nil {
		httpx.Error(w, http.StatusBadRequest, "not a file")
		return
	}
	b, err := h.OS.GetBytes(r.Context(), *n.StorageKey, 2<<20) // 2MB cap for MVP
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "read file")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"node":    n,
		"content": string(b),
	})
}

type putTextBody struct {
	Content string `json:"content"`
}

func (h *FileNodeHandler) PutText(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.FilesWrite) {
		return
	}
	if isSA, _ := h.Store.ServiceAccountInOrg(r.Context(), orgID, uid); isSA {
		httpx.Error(w, http.StatusForbidden, "service accounts cannot write file contents directly; use edit proposals or agent tools")
		return
	}
	pid, err := uuid.Parse(chi.URLParam(r, "spaceID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid space id")
		return
	}
	nodeID, err := uuid.Parse(chi.URLParam(r, "nodeID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid node id")
		return
	}
	if _, err := h.Store.GetSpace(r.Context(), orgID, pid); err != nil {
		httpx.Error(w, http.StatusNotFound, "space not found")
		return
	}
	if h.OS == nil {
		httpx.Error(w, http.StatusInternalServerError, "object store not configured")
		return
	}
	n, err := h.Store.FileNodeByID(r.Context(), pid, nodeID)
	if err != nil {
		if err == pgx.ErrNoRows {
			httpx.Error(w, http.StatusNotFound, "node not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "node")
		return
	}
	if n.Kind != store.FileNodeFile || n.StorageKey == nil || *n.StorageKey == "" || n.DeletedAt != nil {
		httpx.Error(w, http.StatusBadRequest, "not a file")
		return
	}
	var body putTextBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	// MVP: treat as UTF-8 plain text regardless of extension.
	mime := "text/plain"
	if err := h.OS.PutString(r.Context(), *n.StorageKey, mime, body.Content); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "write file")
		return
	}
	if err := h.Store.UpdateFileNodeSizeBytes(r.Context(), pid, nodeID, int64(len([]byte(body.Content)))); err != nil {
		// Non-fatal to content write, but return error so UI retries.
		httpx.Error(w, http.StatusInternalServerError, "update metadata")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

