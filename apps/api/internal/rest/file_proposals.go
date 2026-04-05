package rest

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"

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

type FileProposalHandler struct {
	Store *store.Store
	OS    *files.ObjectStore
}

func sha256HexBytes(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

type createProposalBody struct {
	ProposedContent string `json:"proposed_content"`
}

func (h *FileProposalHandler) Create(w http.ResponseWriter, r *http.Request) {
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
	okAccess, err := h.Store.UserCanAccessSpace(r.Context(), orgID, pid, uid)
	if err != nil || !okAccess {
		httpx.Error(w, http.StatusForbidden, "no access to this space")
		return
	}
	var body createProposalBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if h.OS == nil {
		httpx.Error(w, http.StatusInternalServerError, "object store not configured")
		return
	}
	n, err := h.Store.FileNodeByID(r.Context(), pid, nodeID)
	if err != nil {
		if err == pgx.ErrNoRows {
			httpx.Error(w, http.StatusNotFound, "file not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "file")
		return
	}
	if n.Kind != store.FileNodeFile || n.StorageKey == nil || *n.StorageKey == "" || n.DeletedAt != nil {
		httpx.Error(w, http.StatusBadRequest, "not a file")
		return
	}
	b, err := h.OS.GetBytes(r.Context(), *n.StorageKey, 2<<20)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "read file")
		return
	}
	baseHex := sha256HexBytes(b)
	p, err := h.Store.CreateFileEditProposal(r.Context(), orgID, pid, nodeID, uid, baseHex, string(b), body.ProposedContent)
	if err != nil {
		if errors.Is(err, store.ErrProposalContentTooLarge) {
			httpx.Error(w, http.StatusBadRequest, "proposed content too large")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "create proposal")
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"proposal": p})
}

func (h *FileProposalHandler) List(w http.ResponseWriter, r *http.Request) {
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
	okAccess, err := h.Store.UserCanAccessSpace(r.Context(), orgID, pid, uid)
	if err != nil || !okAccess {
		httpx.Error(w, http.StatusForbidden, "no access to this space")
		return
	}
	list, err := h.Store.ListFileEditProposalsForNode(r.Context(), orgID, pid, nodeID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "list proposals")
		return
	}
	if list == nil {
		list = []store.FileEditProposal{}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"proposals": list})
}

func (h *FileProposalHandler) Accept(w http.ResponseWriter, r *http.Request) {
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
	proposalID, err := uuid.Parse(chi.URLParam(r, "proposalID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid proposal id")
		return
	}
	if h.OS == nil {
		httpx.Error(w, http.StatusInternalServerError, "object store not configured")
		return
	}
	if _, err := h.Store.GetSpace(r.Context(), orgID, pid); err != nil {
		httpx.Error(w, http.StatusNotFound, "space not found")
		return
	}
	okAccess, err := h.Store.UserCanAccessSpace(r.Context(), orgID, pid, uid)
	if err != nil || !okAccess {
		httpx.Error(w, http.StatusForbidden, "no access to this space")
		return
	}
	prop, err := h.Store.GetFileEditProposal(r.Context(), orgID, proposalID)
	if err != nil {
		if err == pgx.ErrNoRows {
			httpx.Error(w, http.StatusNotFound, "proposal not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "proposal")
		return
	}
	if prop.SpaceID != pid {
		httpx.Error(w, http.StatusBadRequest, "proposal not in this space")
		return
	}
	if prop.Status != store.ProposalPending {
		httpx.Error(w, http.StatusConflict, "proposal is not pending")
		return
	}
	n, err := h.Store.FileNodeByID(r.Context(), pid, prop.NodeID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "file")
		return
	}
	if n.Kind != store.FileNodeFile || n.StorageKey == nil || *n.StorageKey == "" || n.DeletedAt != nil {
		httpx.Error(w, http.StatusBadRequest, "not a file")
		return
	}
	cur, err := h.OS.GetBytes(r.Context(), *n.StorageKey, 2<<20)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "read file")
		return
	}
	if sha256HexBytes(cur) != prop.BaseContentSHA256 {
		httpx.Error(w, http.StatusConflict, "base file changed")
		return
	}
	mime := "text/plain"
	if err := h.OS.PutString(r.Context(), *n.StorageKey, mime, prop.ProposedContent); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "write file")
		return
	}
	if err := h.Store.UpdateFileNodeSizeBytes(r.Context(), pid, prop.NodeID, int64(len([]byte(prop.ProposedContent)))); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "update metadata")
		return
	}
	if err := h.Store.MarkProposalAccepted(r.Context(), orgID, proposalID, uid); err != nil {
		if errors.Is(err, store.ErrProposalNotPending) {
			httpx.Error(w, http.StatusConflict, "proposal is not pending")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "finalize proposal")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *FileProposalHandler) Reject(w http.ResponseWriter, r *http.Request) {
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
	proposalID, err := uuid.Parse(chi.URLParam(r, "proposalID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid proposal id")
		return
	}
	if _, err := h.Store.GetSpace(r.Context(), orgID, pid); err != nil {
		httpx.Error(w, http.StatusNotFound, "space not found")
		return
	}
	okAccess, err := h.Store.UserCanAccessSpace(r.Context(), orgID, pid, uid)
	if err != nil || !okAccess {
		httpx.Error(w, http.StatusForbidden, "no access to this space")
		return
	}
	prop, err := h.Store.GetFileEditProposal(r.Context(), orgID, proposalID)
	if err != nil {
		if err == pgx.ErrNoRows {
			httpx.Error(w, http.StatusNotFound, "proposal not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "proposal")
		return
	}
	if prop.SpaceID != pid {
		httpx.Error(w, http.StatusBadRequest, "proposal not in this space")
		return
	}
	if err := h.Store.ResolveFileEditProposalRejected(r.Context(), orgID, proposalID, uid); err != nil {
		if errors.Is(err, store.ErrProposalNotPending) {
			httpx.Error(w, http.StatusConflict, "proposal is not pending")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "reject proposal")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
