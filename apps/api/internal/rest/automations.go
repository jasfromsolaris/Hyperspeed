package rest

import (
	"context"
	"encoding/base64"
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
	"hyperspeed/api/internal/middleware"
	"hyperspeed/api/internal/rbac"
	"hyperspeed/api/internal/secrets"
	"hyperspeed/api/internal/social"
	"hyperspeed/api/internal/store"
)

type AutomationsHandler struct {
	Store         *store.Store
	EncryptKeyB64 string
}

func (h *AutomationsHandler) encKey() ([]byte, error) {
	k := strings.TrimSpace(h.EncryptKeyB64)
	if k == "" {
		return nil, errors.New("encryption key not configured")
	}
	return base64.StdEncoding.DecodeString(k)
}

func (h *AutomationsHandler) requireFilesWrite(w http.ResponseWriter, r *http.Request, orgID, uid uuid.UUID) bool {
	ok, err := rbac.HasPermission(r.Context(), h.Store, orgID, uid, rbac.FilesWrite)
	if err != nil || !ok {
		httpx.Error(w, http.StatusForbidden, "forbidden")
		return false
	}
	return true
}

func (h *AutomationsHandler) requireFilesRead(w http.ResponseWriter, r *http.Request, orgID, uid uuid.UUID) bool {
	ok, err := rbac.HasPermission(r.Context(), h.Store, orgID, uid, rbac.FilesRead)
	if err != nil || !ok {
		httpx.Error(w, http.StatusForbidden, "forbidden")
		return false
	}
	return true
}

func isServiceAccount(ctx context.Context, st *store.Store, orgID, userID uuid.UUID) bool {
	_, err := st.GetServiceAccountByUserInOrg(ctx, orgID, userID)
	return err == nil
}

type createAutomationBody struct {
	Name        string          `json:"name"`
	Kind        string          `json:"kind"`
	Config      json.RawMessage `json:"config"`
	Status      string          `json:"status"`
	OAuthToken  string          `json:"oauth_token"`
	Description string          `json:"description"`
}

func (h *AutomationsHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !h.requireFilesRead(w, r, orgID, uid) {
		return
	}
	spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid space id")
		return
	}
	if _, err := h.Store.GetSpace(r.Context(), orgID, spaceID); err != nil {
		if err == pgx.ErrNoRows {
			httpx.Error(w, http.StatusNotFound, "space not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "space")
		return
	}
	okAccess, err := h.Store.UserCanAccessSpace(r.Context(), orgID, spaceID, uid)
	if err != nil || !okAccess {
		httpx.Error(w, http.StatusForbidden, "no access to this space")
		return
	}
	list, err := h.Store.ListSpaceAutomations(r.Context(), orgID, spaceID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "list automations")
		return
	}
	if list == nil {
		list = []store.SpaceAutomation{}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"automations": list})
}

func (h *AutomationsHandler) Create(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !h.requireFilesWrite(w, r, orgID, uid) {
		return
	}
	spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid space id")
		return
	}
	if _, err := h.Store.GetSpace(r.Context(), orgID, spaceID); err != nil {
		if err == pgx.ErrNoRows {
			httpx.Error(w, http.StatusNotFound, "space not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "space")
		return
	}
	okAccess, err := h.Store.UserCanAccessSpace(r.Context(), orgID, spaceID, uid)
	if err != nil || !okAccess {
		httpx.Error(w, http.StatusForbidden, "no access to this space")
		return
	}
	var body createAutomationBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" || body.Kind == "" {
		httpx.Error(w, http.StatusBadRequest, "name and kind required")
		return
	}
	kind := strings.TrimSpace(strings.ToLower(body.Kind))
	if kind != "social_post" && kind != "reverse_tunnel" && kind != "scheduled" && kind != "webhook" {
		httpx.Error(w, http.StatusBadRequest, "invalid kind")
		return
	}
	sa := isServiceAccount(r.Context(), h.Store, orgID, uid)
	status := strings.TrimSpace(strings.ToLower(body.Status))
	if sa {
		status = "pending_approval"
		body.OAuthToken = ""
		if body.Config == nil {
			body.Config = json.RawMessage(`{}`)
		}
		// Merge description into config for reviewers
		if strings.TrimSpace(body.Description) != "" {
			var m map[string]any
			_ = json.Unmarshal(body.Config, &m)
			if m == nil {
				m = map[string]any{}
			}
			m["proposal_note"] = strings.TrimSpace(body.Description)
			body.Config, _ = json.Marshal(m)
		}
	} else {
		if status == "" {
			status = "draft"
		}
		if status != "draft" && status != "pending_approval" && status != "active" && status != "paused" {
			httpx.Error(w, http.StatusBadRequest, "invalid status")
			return
		}
	}

	var oauthEnc *string
	if strings.TrimSpace(body.OAuthToken) != "" {
		key32, err := h.encKey()
		if err != nil || len(key32) != 32 {
			httpx.Error(w, http.StatusInternalServerError, "encryption not configured")
			return
		}
		enc, err := secrets.EncryptString(key32, strings.TrimSpace(body.OAuthToken))
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "encrypt token")
			return
		}
		oauthEnc = &enc
	}

	var cu *uuid.UUID
	var csa *uuid.UUID
	if sa {
		sarow, err := h.Store.GetServiceAccountByUserInOrg(r.Context(), orgID, uid)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "service account")
			return
		}
		x := sarow.ID
		csa = &x
	} else {
		cu = &uid
	}

	if body.Config == nil {
		body.Config = json.RawMessage(`{}`)
	}

	a, err := h.Store.CreateSpaceAutomation(r.Context(), orgID, spaceID, store.CreateSpaceAutomationInput{
		Name:                      name,
		Kind:                      kind,
		Config:                    body.Config,
		Status:                    status,
		OAuthTokenEnc:             oauthEnc,
		CreatedByUserID:           cu,
		CreatedByServiceAccountID: csa,
	})
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "create automation")
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"automation": a})
}

type patchAutomationBody struct {
	Name       *string         `json:"name"`
	Config     json.RawMessage `json:"config"`
	Status     *string         `json:"status"`
	OAuthToken *string         `json:"oauth_token"`
}

func (h *AutomationsHandler) Patch(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !h.requireFilesWrite(w, r, orgID, uid) {
		return
	}
	spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid space id")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "automationID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid automation id")
		return
	}
	if _, err := h.Store.GetSpace(r.Context(), orgID, spaceID); err != nil {
		if err == pgx.ErrNoRows {
			httpx.Error(w, http.StatusNotFound, "space not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "space")
		return
	}
	okAccess, err := h.Store.UserCanAccessSpace(r.Context(), orgID, spaceID, uid)
	if err != nil || !okAccess {
		httpx.Error(w, http.StatusForbidden, "no access to this space")
		return
	}
	var body patchAutomationBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	var oauthEnc *string
	if body.OAuthToken != nil && strings.TrimSpace(*body.OAuthToken) != "" {
		if isServiceAccount(r.Context(), h.Store, orgID, uid) {
			httpx.Error(w, http.StatusForbidden, "service account cannot set oauth token")
			return
		}
		key32, err := h.encKey()
		if err != nil || len(key32) != 32 {
			httpx.Error(w, http.StatusInternalServerError, "encryption not configured")
			return
		}
		enc, err := secrets.EncryptString(key32, strings.TrimSpace(*body.OAuthToken))
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "encrypt token")
			return
		}
		oauthEnc = &enc
	}
	var st *string
	if body.Status != nil {
		s := strings.TrimSpace(strings.ToLower(*body.Status))
		st = &s
	}
	a, err := h.Store.PatchSpaceAutomation(r.Context(), orgID, spaceID, id, store.PatchSpaceAutomationInput{
		Name:          body.Name,
		Config:        body.Config,
		Status:        st,
		OAuthTokenEnc: oauthEnc,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			httpx.Error(w, http.StatusNotFound, "automation not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "patch automation")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"automation": a})
}

func (h *AutomationsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !h.requireFilesWrite(w, r, orgID, uid) {
		return
	}
	spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid space id")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "automationID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid automation id")
		return
	}
	if _, err := h.Store.GetSpace(r.Context(), orgID, spaceID); err != nil {
		if err == pgx.ErrNoRows {
			httpx.Error(w, http.StatusNotFound, "space not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "space")
		return
	}
	okAccess, err := h.Store.UserCanAccessSpace(r.Context(), orgID, spaceID, uid)
	if err != nil || !okAccess {
		httpx.Error(w, http.StatusForbidden, "no access to this space")
		return
	}
	if err := h.Store.DeleteSpaceAutomation(r.Context(), orgID, spaceID, id); err != nil {
		if err == pgx.ErrNoRows {
			httpx.Error(w, http.StatusNotFound, "automation not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "delete")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AutomationsHandler) Approve(w http.ResponseWriter, r *http.Request) {
	h.review(w, r, true)
}

func (h *AutomationsHandler) Reject(w http.ResponseWriter, r *http.Request) {
	h.review(w, r, false)
}

type rejectBody struct {
	Reason string `json:"reason"`
}

func (h *AutomationsHandler) review(w http.ResponseWriter, r *http.Request, approve bool) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if isServiceAccount(r.Context(), h.Store, orgID, uid) {
		httpx.Error(w, http.StatusForbidden, "service accounts cannot approve automations")
		return
	}
	if !h.requireFilesWrite(w, r, orgID, uid) {
		return
	}
	spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid space id")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "automationID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid automation id")
		return
	}
	if _, err := h.Store.GetSpace(r.Context(), orgID, spaceID); err != nil {
		if err == pgx.ErrNoRows {
			httpx.Error(w, http.StatusNotFound, "space not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "space")
		return
	}
	okAccess, err := h.Store.UserCanAccessSpace(r.Context(), orgID, spaceID, uid)
	if err != nil || !okAccess {
		httpx.Error(w, http.StatusForbidden, "no access to this space")
		return
	}
	var a store.SpaceAutomation
	if approve {
		a, err = h.Store.ApproveSpaceAutomation(r.Context(), orgID, spaceID, id, uid)
	} else {
		var body rejectBody
		_ = httpx.DecodeJSONLenient(r, &body)
		a, err = h.Store.RejectSpaceAutomation(r.Context(), orgID, spaceID, id, uid, strings.TrimSpace(body.Reason))
	}
	if err != nil {
		if err == pgx.ErrNoRows {
			httpx.Error(w, http.StatusNotFound, "automation not pending or not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "review")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"automation": a})
}

type runBody struct {
	Text string `json:"text"`
}

func (h *AutomationsHandler) Run(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !h.requireFilesWrite(w, r, orgID, uid) {
		return
	}
	spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid space id")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "automationID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid automation id")
		return
	}
	if _, err := h.Store.GetSpace(r.Context(), orgID, spaceID); err != nil {
		if err == pgx.ErrNoRows {
			httpx.Error(w, http.StatusNotFound, "space not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "space")
		return
	}
	okAccess, err := h.Store.UserCanAccessSpace(r.Context(), orgID, spaceID, uid)
	if err != nil || !okAccess {
		httpx.Error(w, http.StatusForbidden, "no access to this space")
		return
	}
	a, err := h.Store.GetSpaceAutomation(r.Context(), orgID, spaceID, id)
	if err != nil {
		if err == pgx.ErrNoRows {
			httpx.Error(w, http.StatusNotFound, "automation not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "automation")
		return
	}
	if a.Status != "active" {
		httpx.Error(w, http.StatusBadRequest, "automation must be active to run")
		return
	}
	if a.Kind != "social_post" {
		httpx.Error(w, http.StatusBadRequest, "run not implemented for this kind")
		return
	}
	var body runBody
	_ = httpx.DecodeJSONLenient(r, &body)
	text := strings.TrimSpace(body.Text)
	if text == "" {
		var cfg map[string]any
		_ = json.Unmarshal(a.Config, &cfg)
		if v, ok := cfg["default_text"].(string); ok {
			text = strings.TrimSpace(v)
		}
	}
	if text == "" {
		httpx.Error(w, http.StatusBadRequest, "provide text in body or config.default_text")
		return
	}
	enc, err := h.Store.GetSpaceAutomationOAuthTokenEnc(r.Context(), orgID, spaceID, id)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "oauth token not configured; PATCH token first")
		return
	}
	key32, err := h.encKey()
	if err != nil || len(key32) != 32 {
		httpx.Error(w, http.StatusInternalServerError, "encryption not configured")
		return
	}
	tok, err := secrets.DecryptString(key32, *enc)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "decrypt token")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	tweetID, err := social.PostTweetV2(ctx, tok, text)
	now := time.Now()
	var errMsg *string
	var ext *string
	if err != nil {
		s := err.Error()
		errMsg = &s
	} else {
		ext = &tweetID
	}
	run, err := h.Store.InsertAutomationRun(r.Context(), id, err == nil, errMsg, ext)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "record run")
		return
	}
	le := errMsg
	_ = h.Store.TouchSpaceAutomationLastRun(r.Context(), orgID, spaceID, id, now, le)
	httpx.JSON(w, http.StatusOK, map[string]any{"run": run, "tweet_id": tweetID, "error": errMsg})
}

func (h *AutomationsHandler) ListRuns(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !h.requireFilesRead(w, r, orgID, uid) {
		return
	}
	spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid space id")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "automationID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid automation id")
		return
	}
	if _, err := h.Store.GetSpace(r.Context(), orgID, spaceID); err != nil {
		if err == pgx.ErrNoRows {
			httpx.Error(w, http.StatusNotFound, "space not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "space")
		return
	}
	okAccess, err := h.Store.UserCanAccessSpace(r.Context(), orgID, spaceID, uid)
	if err != nil || !okAccess {
		httpx.Error(w, http.StatusForbidden, "no access to this space")
		return
	}
	runs, err := h.Store.ListAutomationRuns(r.Context(), orgID, spaceID, id, 50)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "runs")
		return
	}
	if runs == nil {
		runs = []store.SpaceAutomationRun{}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"runs": runs})
}
