package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"hyperspeed/api/internal/ctxkey"
	"hyperspeed/api/internal/httpx"
	"hyperspeed/api/internal/slug"
	"hyperspeed/api/internal/store"
)

type registerBody struct {
	Email              string `json:"email"`
	Password           string `json:"password"`
	Name               string `json:"name"`
	OrganizationName   string `json:"organization_name"`
	IntendedPublicURL  string `json:"intended_public_url,omitempty"`
}

type loginBody struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	// SignupPending is true when the user registered for open signup and awaits admin approval.
	SignupPending bool `json:"signup_pending,omitempty"`
}

type refreshBody struct {
	RefreshToken string `json:"refresh_token"`
}

func (s *Service) Register(w http.ResponseWriter, r *http.Request) {
	var body registerBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	body.Email = strings.TrimSpace(strings.ToLower(body.Email))
	body.Name = strings.TrimSpace(body.Name)
	body.OrganizationName = strings.TrimSpace(body.OrganizationName)
	if body.Email == "" || len(body.Password) < 8 {
		httpx.Error(w, http.StatusBadRequest, "email and password (min 8 chars) required")
		return
	}
	hash, err := HashPassword(body.Password)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "hash failed")
		return
	}
	var dn *string
	if body.Name != "" {
		dn = &body.Name
	}

	ctx := r.Context()
	n, err := s.Store.CountOrganizations(ctx)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "organizations")
		return
	}

	if n == 0 {
		if body.OrganizationName == "" {
			httpx.Error(w, http.StatusBadRequest, "organization_name required for the first account")
			return
		}
		intendedTrim := strings.TrimSpace(body.IntendedPublicURL)
		if intendedTrim != "" {
			if len(intendedTrim) > 2048 {
				httpx.Error(w, http.StatusBadRequest, "intended_public_url too long")
				return
			}
			low := strings.ToLower(intendedTrim)
			if !strings.HasPrefix(low, "http://") && !strings.HasPrefix(low, "https://") {
				httpx.Error(w, http.StatusBadRequest, "intended_public_url must be an http(s) URL")
				return
			}
		}
		u, err := s.Store.CreateUser(ctx, body.Email, hash, dn)
		if err != nil {
			if strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "duplicate") {
				httpx.Error(w, http.StatusConflict, "email already registered")
				return
			}
			httpx.Error(w, http.StatusInternalServerError, "could not create user")
			return
		}
		base := slug.Slugify(body.OrganizationName)
		slugStr := base
		for i := 0; i < 20; i++ {
			o, err := s.Store.CreateOrganizationSelfHostedOne(ctx, body.OrganizationName, slugStr, u.ID)
			if err == nil {
				if intendedTrim != "" {
					if err := s.Store.SetOrgIntendedPublicURL(ctx, o.ID, &intendedTrim); err != nil {
						httpx.Error(w, http.StatusInternalServerError, "save intended url")
						return
					}
				}
				access, refresh, err := s.IssueTokens(ctx, u.ID, u.Email)
				if err != nil {
					httpx.Error(w, http.StatusInternalServerError, "tokens")
					return
				}
				httpx.JSON(w, http.StatusCreated, tokenResponse{
					AccessToken:  access,
					RefreshToken: refresh,
					TokenType:    "Bearer",
					ExpiresIn:    900,
					SignupPending: false,
				})
				return
			}
			if errors.Is(err, store.ErrSelfHostOrgAlreadyExists) {
				_ = s.Store.DeleteUser(ctx, u.ID)
				httpx.Error(w, http.StatusForbidden, "single_org_exists")
				return
			}
			if store.IsUniqueViolation(err) {
				slugStr = fmt.Sprintf("%s-%s", base, uuid.NewString()[:8])
				continue
			}
			_ = s.Store.DeleteUser(ctx, u.ID)
			httpx.Error(w, http.StatusInternalServerError, "create organization")
			return
		}
		_ = s.Store.DeleteUser(ctx, u.ID)
		httpx.Error(w, http.StatusConflict, "could not allocate slug")
		return
	}

	org, err := s.Store.FirstOrganization(ctx)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.Error(w, http.StatusInternalServerError, "organization state")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "organization")
		return
	}
	if !org.OpenSignupsEnabled {
		httpx.Error(w, http.StatusForbidden, "signups_disabled")
		return
	}

	u, err := s.Store.CreateUser(ctx, body.Email, hash, dn)
	if err != nil {
		if strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "duplicate") {
			httpx.Error(w, http.StatusConflict, "email already registered")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "could not create user")
		return
	}
	if _, err := s.Store.CreateSignupRequest(ctx, org.ID, u.ID); err != nil {
		if store.IsUniqueViolation(err) {
			_ = s.Store.DeleteUser(ctx, u.ID)
			httpx.Error(w, http.StatusConflict, "signup_already_pending")
			return
		}
		_ = s.Store.DeleteUser(ctx, u.ID)
		httpx.Error(w, http.StatusInternalServerError, "signup request")
		return
	}
	access, refresh, err := s.IssueTokens(ctx, u.ID, u.Email)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "tokens")
		return
	}
	httpx.JSON(w, http.StatusCreated, tokenResponse{
		AccessToken:   access,
		RefreshToken:  refresh,
		TokenType:     "Bearer",
		ExpiresIn:     900,
		SignupPending: true,
	})
}

func (s *Service) Login(w http.ResponseWriter, r *http.Request) {
	var body loginBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	body.Email = strings.TrimSpace(strings.ToLower(body.Email))
	id, hash, err := s.Store.UserByEmail(r.Context(), body.Email)
	if err != nil {
		if err == pgx.ErrNoRows {
			httpx.Error(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "db")
		return
	}
	if !CheckPassword(hash, body.Password) {
		httpx.Error(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	u, err := s.Store.UserByID(r.Context(), id)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "user")
		return
	}
	access, refresh, err := s.IssueTokens(r.Context(), u.ID, u.Email)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "tokens")
		return
	}
	httpx.JSON(w, http.StatusOK, tokenResponse{
		AccessToken:  access,
		RefreshToken: refresh,
		TokenType:    "Bearer",
		ExpiresIn:    900,
	})
}

func hashRefresh(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

func (s *Service) Refresh(w http.ResponseWriter, r *http.Request) {
	var body refreshBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.RefreshToken == "" {
		httpx.Error(w, http.StatusBadRequest, "refresh_token required")
		return
	}
	h := hashRefresh(body.RefreshToken)
	uid, err := s.Store.UserIDByRefreshToken(r.Context(), h)
	if err != nil {
		httpx.Error(w, http.StatusUnauthorized, "invalid refresh token")
		return
	}
	_ = s.Store.DeleteRefreshToken(r.Context(), h)
	u, err := s.Store.UserByID(r.Context(), uid)
	if err != nil {
		httpx.Error(w, http.StatusUnauthorized, "user")
		return
	}
	access, refresh, err := s.IssueTokens(r.Context(), u.ID, u.Email)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "tokens")
		return
	}
	httpx.JSON(w, http.StatusOK, tokenResponse{
		AccessToken:  access,
		RefreshToken: refresh,
		TokenType:    "Bearer",
		ExpiresIn:    900,
	})
}

func (s *Service) Me(w http.ResponseWriter, r *http.Request) {
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	u, err := s.Store.UserByID(r.Context(), uid)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "user")
		return
	}
	out := map[string]any{
		"id":           u.ID,
		"email":        u.Email,
		"display_name": u.DisplayName,
		"created_at":   u.CreatedAt,
	}
	if sa, err := s.Store.ServiceAccountIdentityByUser(r.Context(), uid); err == nil && sa != nil {
		out["service_account"] = map[string]any{
			"id":              sa.ServiceAccountID,
			"organization_id": sa.OrganizationID,
		}
	} else {
		out["service_account"] = nil
	}

	orgs, err := s.Store.ListOrganizationsForUser(r.Context(), uid)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "organizations")
		return
	}
	if len(orgs) > 0 {
		out["signup_pending"] = false
		httpx.JSON(w, http.StatusOK, out)
		return
	}
	pending, err := s.Store.PendingSignupRequestForUser(r.Context(), uid)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "signup state")
		return
	}
	if pending != nil {
		out["signup_pending"] = true
		out["signup_request"] = map[string]any{
			"id":              pending.ID,
			"organization_id": pending.OrganizationID,
			"status":          pending.Status,
		}
	} else {
		out["signup_pending"] = false
	}
	httpx.JSON(w, http.StatusOK, out)
}

type patchMeBody struct {
	DisplayName *string `json:"display_name"`
}

func (s *Service) PatchMe(w http.ResponseWriter, r *http.Request) {
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var body patchMeBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.DisplayName != nil {
		v := strings.TrimSpace(*body.DisplayName)
		body.DisplayName = &v
		if v == "" {
			body.DisplayName = nil
		}
	}
	u, err := s.Store.UpdateUserDisplayName(r.Context(), uid, body.DisplayName)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "update user")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"id":           u.ID,
		"email":        u.Email,
		"display_name": u.DisplayName,
		"created_at":   u.CreatedAt,
	})
}

// Logout invalidates refresh token body.
func (s *Service) Logout(w http.ResponseWriter, r *http.Request) {
	var body refreshBody
	_ = httpx.DecodeJSON(r, &body)
	if body.RefreshToken != "" {
		_ = s.Store.DeleteRefreshToken(r.Context(), hashRefresh(body.RefreshToken))
	}
	w.WriteHeader(http.StatusNoContent)
}
