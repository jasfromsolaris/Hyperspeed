package rest

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"path"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"hyperspeed/api/internal/ctxkey"
	"hyperspeed/api/internal/httpx"
	"hyperspeed/api/internal/middleware"
	"hyperspeed/api/internal/rbac"
	"hyperspeed/api/internal/store"
)

const (
	maxZipImportBytes   = 128 << 20
	maxZipEntries       = 5000
	maxZipUncompressed  = 256 << 20
	maxExportFileBytes  = 64 << 20
)

type zipManifest struct {
	Version int                `json:"version"`
	Files   []zipManifestEntry `json:"files"`
}

type zipManifestEntry struct {
	Path       string `json:"path"`
	NodeID     string `json:"node_id,omitempty"`
	SizeBytes  int64  `json:"size_bytes,omitempty"`
}

// ExportZip streams a zip of all non-deleted files in the space (folder structure preserved).
func (h *FileNodeHandler) ExportZip(w http.ResponseWriter, r *http.Request) {
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
	if h.OS == nil {
		httpx.Error(w, http.StatusInternalServerError, "object store not configured")
		return
	}

	nodes, err := h.Store.ListAllFileNodes(r.Context(), spaceID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "list files")
		return
	}
	byID := make(map[uuid.UUID]store.FileNode, len(nodes))
	for _, n := range nodes {
		byID[n.ID] = n
	}

	buildPath := func(n store.FileNode) string {
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

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="space-export.zip"`)
	zw := zip.NewWriter(w)
	manifest := zipManifest{Version: 1}

	for _, n := range nodes {
		if n.Kind != store.FileNodeFile || n.StorageKey == nil || *n.StorageKey == "" {
			continue
		}
		p := buildPath(n)
		if p == "" || strings.Contains(p, "..") {
			continue
		}
		rc, sz, err := h.OS.GetObjectStream(r.Context(), *n.StorageKey)
		if err != nil {
			_ = rc.Close()
			continue
		}
		if sz > maxExportFileBytes {
			_ = rc.Close()
			continue
		}
		fh, err := zw.Create(p)
		if err != nil {
			_ = rc.Close()
			continue
		}
		if _, err := io.Copy(fh, io.LimitReader(rc, maxExportFileBytes)); err != nil {
			_ = rc.Close()
			continue
		}
		_ = rc.Close()
		manifest.Files = append(manifest.Files, zipManifestEntry{
			Path:      p,
			NodeID:    n.ID.String(),
			SizeBytes: sz,
		})
	}

	sort.Slice(manifest.Files, func(i, j int) bool { return manifest.Files[i].Path < manifest.Files[j].Path })
	mb, _ := json.MarshalIndent(manifest, "", "  ")
	mw, err := zw.Create("manifest.json")
	if err == nil {
		_, _ = mw.Write(mb)
	}
	if err := zw.Close(); err != nil {
		return
	}
}

// ImportZip accepts multipart file "archive" and recreates folders/files under an optional parent folder.
func (h *FileNodeHandler) ImportZip(w http.ResponseWriter, r *http.Request) {
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
		httpx.Error(w, http.StatusForbidden, "service accounts cannot import zip")
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
	if err := r.ParseMultipartForm(maxZipImportBytes); err != nil {
		httpx.Error(w, http.StatusBadRequest, "multipart")
		return
	}
	f, _, err := r.FormFile("archive")
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "archive file required")
		return
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, maxZipImportBytes))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "read archive")
		return
	}
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid zip")
		return
	}
	if len(zr.File) > maxZipEntries {
		httpx.Error(w, http.StatusBadRequest, "too many zip entries")
		return
	}

	var parentID *uuid.UUID
	if v := strings.TrimSpace(r.FormValue("parent_id")); v != "" {
		pid, err := uuid.Parse(v)
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "invalid parent_id")
			return
		}
		parentID = &pid
	}

	folderIDs := make(map[string]uuid.UUID)

	ensureFolder := func(parts []string) error {
		for i := 0; i < len(parts); i++ {
			sub := strings.Join(parts[:i+1], "/")
			if _, ok := folderIDs[sub]; ok {
				continue
			}
			folderName := parts[i]
			var par *uuid.UUID
			if i == 0 {
				par = parentID
			} else {
				pid := folderIDs[strings.Join(parts[:i], "/")]
				par = &pid
			}
			fn, err := h.Store.CreateFolderNode(r.Context(), spaceID, par, folderName, uid)
			if err != nil {
				return err
			}
			folderIDs[sub] = fn.ID
		}
		return nil
	}

	var fileEntries []*zip.File
	var totalUC int64
	for _, zf := range zr.File {
		raw := strings.TrimPrefix(strings.ReplaceAll(zf.Name, "\\", "/"), "/")
		if raw == "" || raw == "." {
			continue
		}
		if strings.Contains(raw, "..") {
			httpx.Error(w, http.StatusBadRequest, "invalid path in zip")
			return
		}
		isDir := strings.HasSuffix(raw, "/")
		name := strings.TrimSuffix(raw, "/")
		name = path.Clean(name)
		if name == "." || name == "manifest.json" {
			continue
		}
		totalUC += int64(zf.UncompressedSize64)
		if totalUC > maxZipUncompressed {
			httpx.Error(w, http.StatusBadRequest, "zip uncompressed size too large")
			return
		}
		if isDir {
			parts := strings.Split(name, "/")
			_ = ensureFolder(parts)
			continue
		}
		fileEntries = append(fileEntries, zf)
	}

	sort.Slice(fileEntries, func(i, j int) bool {
		return fileEntries[i].Name < fileEntries[j].Name
	})

	created := 0
	for _, zf := range fileEntries {
		raw := strings.TrimPrefix(strings.ReplaceAll(zf.Name, "\\", "/"), "/")
		name := path.Clean(strings.TrimSuffix(raw, "/"))
		if name == "" || name == "manifest.json" || strings.Contains(name, "..") {
			continue
		}
		parts := strings.Split(name, "/")
		if len(parts) == 0 {
			continue
		}
		if len(parts) > 1 {
			if err := ensureFolder(parts[:len(parts)-1]); err != nil {
				continue
			}
		}
		fileName := parts[len(parts)-1]
		var par *uuid.UUID
		if len(parts) == 1 {
			par = parentID
		} else {
			pid := folderIDs[strings.Join(parts[:len(parts)-1], "/")]
			par = &pid
		}
		rc, err := zf.Open()
		if err != nil {
			continue
		}
		body, err := io.ReadAll(io.LimitReader(rc, maxExportFileBytes))
		_ = rc.Close()
		if err != nil {
			continue
		}
		mime := http.DetectContentType(body)
		if mime == "" {
			mime = "application/octet-stream"
		}
		sz := int64(len(body))
		storageKey := "org/" + orgID.String() + "/project/" + spaceID.String() + "/node/" + uuid.NewString() + "/" + fileName
		_, err = h.Store.CreateFileNode(r.Context(), spaceID, par, fileName, &mime, &sz, storageKey, uid)
		if err != nil {
			continue
		}
		if err := h.OS.Put(r.Context(), storageKey, mime, bytes.NewReader(body), &sz); err != nil {
			continue
		}
		created++
	}

	h.publishFileTree(orgID, spaceID)
	httpx.JSON(w, http.StatusOK, map[string]any{"imported_files": created, "folders": len(folderIDs)})
}
