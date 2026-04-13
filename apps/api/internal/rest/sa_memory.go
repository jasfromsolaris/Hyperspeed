package rest

import (
	"errors"
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

type ServiceAccountMemoryHandler struct {
	Store *store.Store
}

func parseSAFromPath(r *http.Request, st *store.Store, orgID uuid.UUID) (uuid.UUID, bool) {
	saID, err := uuid.Parse(chi.URLParam(r, "serviceAccountID"))
	if err != nil {
		return uuid.Nil, false
	}
	if _, err := st.GetServiceAccountInOrg(r.Context(), orgID, saID); err != nil {
		return uuid.Nil, false
	}
	return saID, true
}

func (h *ServiceAccountMemoryHandler) Episodes(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.OrgMembersManage) {
		return
	}
	saID, ok := parseSAFromPath(r, h.Store, orgID)
	if !ok {
		httpx.Error(w, http.StatusNotFound, "service account not found")
		return
	}
	list, err := h.Store.ListStaffMemoryEpisodes(r.Context(), orgID, saID, 100)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "memory episodes")
		return
	}
	if list == nil {
		list = []store.StaffMemoryEpisode{}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"episodes": list})
}

func (h *ServiceAccountMemoryHandler) Facts(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.OrgMembersManage) {
		return
	}
	saID, ok := parseSAFromPath(r, h.Store, orgID)
	if !ok {
		httpx.Error(w, http.StatusNotFound, "service account not found")
		return
	}
	list, err := h.Store.ListStaffMemoryFacts(r.Context(), orgID, saID, 100)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "memory facts")
		return
	}
	if list == nil {
		list = []store.StaffMemoryFact{}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"facts": list})
}

func (h *ServiceAccountMemoryHandler) DeleteEpisode(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.OrgMembersManage) {
		return
	}
	saID, ok := parseSAFromPath(r, h.Store, orgID)
	if !ok {
		httpx.Error(w, http.StatusNotFound, "service account not found")
		return
	}
	episodeID, err := uuid.Parse(chi.URLParam(r, "episodeID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid episode id")
		return
	}
	if err := h.Store.DeleteStaffMemoryEpisode(r.Context(), orgID, saID, episodeID); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "delete episode")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *ServiceAccountMemoryHandler) DeleteFact(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.OrgMembersManage) {
		return
	}
	saID, ok := parseSAFromPath(r, h.Store, orgID)
	if !ok {
		httpx.Error(w, http.StatusNotFound, "service account not found")
		return
	}
	factID, err := uuid.Parse(chi.URLParam(r, "factID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid fact id")
		return
	}
	if err := h.Store.InvalidateStaffMemoryFact(r.Context(), orgID, saID, factID); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "delete fact")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *ServiceAccountMemoryHandler) ProfileProposals(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.OrgMembersManage) {
		return
	}
	saID, ok := parseSAFromPath(r, h.Store, orgID)
	if !ok {
		httpx.Error(w, http.StatusNotFound, "service account not found")
		return
	}
	st := strings.TrimSpace(r.URL.Query().Get("status"))
	status := store.ServiceAccountProfileProposalPending
	if st != "" {
		status = store.ServiceAccountProfileProposalStatus(st)
	}
	list, err := h.Store.ListServiceAccountProfileProposals(r.Context(), orgID, saID, status, 100)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "profile proposals")
		return
	}
	if list == nil {
		list = []store.ServiceAccountProfileProposal{}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"proposals": list})
}

func (h *ServiceAccountMemoryHandler) AcceptProfileProposal(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.OrgMembersManage) {
		return
	}
	saID, ok := parseSAFromPath(r, h.Store, orgID)
	if !ok {
		httpx.Error(w, http.StatusNotFound, "service account not found")
		return
	}
	proposalID, err := uuid.Parse(chi.URLParam(r, "proposalID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid proposal id")
		return
	}
	prop, err := h.Store.GetServiceAccountProfileProposal(r.Context(), orgID, proposalID)
	if err == pgx.ErrNoRows {
		httpx.Error(w, http.StatusNotFound, "proposal not found")
		return
	}
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "proposal")
		return
	}
	if prop.ServiceAccountID != saID {
		httpx.Error(w, http.StatusBadRequest, "proposal not in this service account")
		return
	}
	v, err := h.Store.AcceptServiceAccountProfileProposal(r.Context(), orgID, prop.ID, uid)
	if err != nil {
		if errors.Is(err, store.ErrProfileProposalNotPending) {
			httpx.Error(w, http.StatusConflict, "proposal is not pending")
			return
		}
		if errors.Is(err, store.ErrProfileTooLarge) {
			httpx.Error(w, http.StatusBadRequest, "profile too large")
			return
		}
		if err == pgx.ErrNoRows {
			httpx.Error(w, http.StatusNotFound, "proposal not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "accept proposal")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"profile": v})
}

func (h *ServiceAccountMemoryHandler) RejectProfileProposal(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.OrgMembersManage) {
		return
	}
	saID, ok := parseSAFromPath(r, h.Store, orgID)
	if !ok {
		httpx.Error(w, http.StatusNotFound, "service account not found")
		return
	}
	proposalID, err := uuid.Parse(chi.URLParam(r, "proposalID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid proposal id")
		return
	}
	prop, err := h.Store.GetServiceAccountProfileProposal(r.Context(), orgID, proposalID)
	if err == pgx.ErrNoRows {
		httpx.Error(w, http.StatusNotFound, "proposal not found")
		return
	}
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "proposal")
		return
	}
	if prop.ServiceAccountID != saID {
		httpx.Error(w, http.StatusBadRequest, "proposal not in this service account")
		return
	}
	if err := h.Store.ResolveServiceAccountProfileProposalRejected(r.Context(), orgID, prop.ID, uid); err != nil {
		if errors.Is(err, store.ErrProfileProposalNotPending) {
			httpx.Error(w, http.StatusConflict, "proposal is not pending")
			return
		}
		if err == pgx.ErrNoRows {
			httpx.Error(w, http.StatusNotFound, "proposal not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "reject proposal")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

