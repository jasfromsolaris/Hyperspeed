package spacegit

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

	"hyperspeed/api/internal/files"
	"hyperspeed/api/internal/store"
)

const (
	maxGitImportFileBytes = 64 << 20
	maxGitImportFiles     = 5000
)

type treeIndex struct {
	byParentKindName map[string]store.FileNode
}

func parentKey(parentID *uuid.UUID) string {
	if parentID == nil {
		return "root"
	}
	return parentID.String()
}

func newTreeIndex(nodes []store.FileNode) *treeIndex {
	ix := &treeIndex{byParentKindName: make(map[string]store.FileNode, len(nodes))}
	for _, n := range nodes {
		pk := parentKey(n.ParentID)
		ix.byParentKindName[pk+"|"+string(n.Kind)+"|"+n.Name] = n
	}
	return ix
}

func (ix *treeIndex) findChild(parentID *uuid.UUID, name string, kind store.FileNodeKind) (store.FileNode, bool) {
	pk := parentKey(parentID)
	n, ok := ix.byParentKindName[pk+"|"+string(kind)+"|"+name]
	return n, ok
}

func (ix *treeIndex) add(n store.FileNode) {
	pk := parentKey(n.ParentID)
	ix.byParentKindName[pk+"|"+string(n.Kind)+"|"+n.Name] = n
}

// ImportRepoDirIntoSpace writes files from a repo checkout into Hyperspeed under rootFolderID (nil = space root).
func ImportRepoDirIntoSpace(ctx context.Context, st *store.Store, obj *files.ObjectStore, orgID, spaceID, actorID uuid.UUID, rootFolderID *uuid.UUID, repoDir string) (int, error) {
	if rootFolderID != nil {
		f, err := st.FileNodeByID(ctx, spaceID, *rootFolderID)
		if err != nil || f.Kind != store.FileNodeFolder || f.DeletedAt != nil {
			return 0, fmt.Errorf("invalid root folder")
		}
	}
	nodes, err := st.ListAllFileNodes(ctx, spaceID)
	if err != nil {
		return 0, err
	}
	ix := newTreeIndex(nodes)

	var nFiles int
	err = filepath.Walk(repoDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if info.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(repoDir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if strings.Contains(rel, "..") {
			return fmt.Errorf("invalid path %q", rel)
		}
		if nFiles >= maxGitImportFiles {
			return fmt.Errorf("too many files (max %d)", maxGitImportFiles)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if len(data) > maxGitImportFileBytes {
			return fmt.Errorf("file too large: %s", rel)
		}
		parts := strings.Split(rel, "/")
		if len(parts) == 0 {
			return nil
		}
		parentID := rootFolderID
		for i := 0; i < len(parts)-1; i++ {
			name := parts[i]
			if name == "" || name == "." || name == ".." {
				return fmt.Errorf("invalid path %q", rel)
			}
			if fn, ok := ix.findChild(parentID, name, store.FileNodeFolder); ok {
				parentID = &fn.ID
				continue
			}
			fol, err := st.CreateFolderNode(ctx, spaceID, parentID, name, actorID)
			if err != nil {
				return err
			}
			ix.add(fol)
			parentID = &fol.ID
		}
		fileName := parts[len(parts)-1]
		if fileName == "" || fileName == "." || fileName == ".." {
			return fmt.Errorf("invalid path %q", rel)
		}
		mime := mimeFromFileName(fileName)
		size := int64(len(data))

		if fn, ok := ix.findChild(parentID, fileName, store.FileNodeFile); ok {
			if fn.StorageKey == nil || *fn.StorageKey == "" || fn.DeletedAt != nil {
				return fmt.Errorf("broken file node %s", fileName)
			}
			sz := size
			if err := obj.Put(ctx, *fn.StorageKey, mime, bytes.NewReader(data), &sz); err != nil {
				return err
			}
			if err := st.UpdateFileNodeSizeBytes(ctx, spaceID, fn.ID, size); err != nil {
				return err
			}
		} else {
			storageKey := "org/" + orgID.String() + "/project/" + spaceID.String() + "/node/" + uuid.NewString() + "/" + fileName
			n, err := st.CreateFileNode(ctx, spaceID, parentID, fileName, &mime, &size, storageKey, actorID)
			if err != nil {
				return err
			}
			ix.add(n)
			sz := size
			if err := obj.Put(ctx, storageKey, mime, bytes.NewReader(data), &sz); err != nil {
				return err
			}
		}
		nFiles++
		return nil
	})
	return nFiles, err
}

func mimeFromFileName(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".html", ".htm":
		return "text/html"
	case ".css":
		return "text/css"
	case ".js", ".mjs":
		return "text/javascript"
	case ".json":
		return "application/json"
	case ".md":
		return "text/markdown"
	case ".svg":
		return "image/svg+xml"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	default:
		return "application/octet-stream"
	}
}

// ExportSpaceTreeToDir writes files under rootFolderID (nil = entire space tree) into dir (dir is wiped first except caller handles wipe).
func ExportSpaceTreeToDir(ctx context.Context, st *store.Store, obj *files.ObjectStore, spaceID uuid.UUID, rootFolderID *uuid.UUID, destDir string) error {
	nodes, err := st.ListAllFileNodes(ctx, spaceID)
	if err != nil {
		return err
	}
	byID := make(map[uuid.UUID]store.FileNode, len(nodes))
	for _, n := range nodes {
		if n.DeletedAt != nil {
			continue
		}
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

	for _, n := range nodes {
		if n.Kind != store.FileNodeFile || n.DeletedAt != nil || n.StorageKey == nil || *n.StorageKey == "" {
			continue
		}
		fullPath := buildPath(n)
		if fullPath == "" || strings.Contains(fullPath, "..") {
			continue
		}
		var rel string
		if rootFolderID != nil {
			rf, ok := byID[*rootFolderID]
			if !ok || rf.Kind != store.FileNodeFolder {
				continue
			}
			rootPath := buildPath(rf)
			if rootPath == "" {
				continue
			}
			if fullPath == rootPath {
				continue
			}
			if !strings.HasPrefix(fullPath, rootPath+"/") {
				continue
			}
			rel = fullPath[len(rootPath)+1:]
		} else {
			rel = fullPath
		}
		rc, sz, err := obj.GetObjectStream(ctx, *n.StorageKey)
		if err != nil {
			continue
		}
		if sz > maxGitImportFileBytes {
			_ = rc.Close()
			continue
		}
		outPath := filepath.Join(destDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			_ = rc.Close()
			return err
		}
		f, err := os.Create(outPath)
		if err != nil {
			_ = rc.Close()
			return err
		}
		_, err = io.Copy(f, io.LimitReader(rc, maxGitImportFileBytes))
		_ = rc.Close()
		_ = f.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
