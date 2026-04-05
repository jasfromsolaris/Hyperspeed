package rest

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"hyperspeed/api/internal/ctxkey"
	"hyperspeed/api/internal/httpx"
	"hyperspeed/api/internal/middleware"
	"hyperspeed/api/internal/rbac"
	"hyperspeed/api/internal/secrets"
	"hyperspeed/api/internal/store"
)

type SSHConnectionsHandler struct {
	Store          *store.Store
	EncryptKeyB64  string
	encryptKeyOnce []byte
}

func (h *SSHConnectionsHandler) key() ([]byte, error) {
	if h.encryptKeyOnce != nil {
		return h.encryptKeyOnce, nil
	}
	raw := strings.TrimSpace(h.EncryptKeyB64)
	if raw == "" {
		return nil, fmt.Errorf("HS_SSH_ENCRYPTION_KEY not set")
	}
	b, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("HS_SSH_ENCRYPTION_KEY must be base64")
	}
	if len(b) != 32 {
		return nil, fmt.Errorf("HS_SSH_ENCRYPTION_KEY must decode to 32 bytes")
	}
	h.encryptKeyOnce = b
	return b, nil
}

func (h *SSHConnectionsHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.SSHConnectionsManage) {
		return
	}
	list, err := h.Store.ListSSHConnections(r.Context(), orgID, uid)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "list ssh connections")
		return
	}
	if list == nil {
		list = []store.SSHConnection{}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"connections": list})
}

type createSSHConnectionBody struct {
	Name       string  `json:"name"`
	Host       string  `json:"host"`
	Port       int     `json:"port"`
	Username   string  `json:"username"`
	AuthMethod string  `json:"auth_method"` // key | password
	PrivateKey *string `json:"private_key"`
	Passphrase *string `json:"passphrase"`
	Password   *string `json:"password"`
}

func (h *SSHConnectionsHandler) Create(w http.ResponseWriter, r *http.Request) {
	k, err := h.key()
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.SSHConnectionsManage) {
		return
	}
	var body createSSHConnectionBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	body.Host = strings.TrimSpace(body.Host)
	body.Username = strings.TrimSpace(body.Username)
	body.AuthMethod = strings.TrimSpace(strings.ToLower(body.AuthMethod))
	if body.Port <= 0 {
		body.Port = 22
	}

	if body.AuthMethod == "" {
		body.AuthMethod = "key"
	}
	if body.AuthMethod != "key" && body.AuthMethod != "password" {
		httpx.Error(w, http.StatusBadRequest, "auth_method must be key or password")
		return
	}

	if body.Name == "" || body.Host == "" || body.Username == "" {
		httpx.Error(w, http.StatusBadRequest, "name, host, username required")
		return
	}

	var privEnc *string
	var passEnc *string
	var pwdEnc *string

	if body.AuthMethod == "key" {
		pk := ""
		if body.PrivateKey != nil {
			pk = strings.TrimSpace(*body.PrivateKey)
		}
		if pk == "" {
			httpx.Error(w, http.StatusBadRequest, "private_key required for key auth")
			return
		}
		enc, err := secrets.EncryptString(k, pk)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "encrypt private key")
			return
		}
		privEnc = &enc
		if body.Passphrase != nil && strings.TrimSpace(*body.Passphrase) != "" {
			pe, err := secrets.EncryptString(k, strings.TrimSpace(*body.Passphrase))
			if err != nil {
				httpx.Error(w, http.StatusInternalServerError, "encrypt passphrase")
				return
			}
			passEnc = &pe
		}
	} else {
		if body.Password != nil && strings.TrimSpace(*body.Password) != "" {
			pe, err := secrets.EncryptString(k, strings.TrimSpace(*body.Password))
			if err != nil {
				httpx.Error(w, http.StatusInternalServerError, "encrypt password")
				return
			}
			pwdEnc = &pe
		}
	}

	conn, err := h.Store.CreateSSHConnection(
		r.Context(),
		orgID,
		uid,
		body.Name,
		body.Host,
		body.Port,
		body.Username,
		body.AuthMethod,
		privEnc,
		passEnc,
		pwdEnc,
	)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "create ssh connection")
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"connection": conn})
}

type patchSSHConnectionBody struct {
	Name       *string `json:"name"`
	Host       *string `json:"host"`
	Port       *int    `json:"port"`
	Username   *string `json:"username"`
	PrivateKey *string `json:"private_key"`
	Passphrase *string `json:"passphrase"`
	AuthMethod *string `json:"auth_method"` // key | password
	Password   *string `json:"password"`
}

func (h *SSHConnectionsHandler) Patch(w http.ResponseWriter, r *http.Request) {
	k, err := h.key()
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.SSHConnectionsManage) {
		return
	}
	connID, err := uuid.Parse(chi.URLParam(r, "connID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid connection id")
		return
	}
	var body patchSSHConnectionBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	trim := func(p *string) *string {
		if p == nil {
			return nil
		}
		s := strings.TrimSpace(*p)
		return &s
	}
	name := trim(body.Name)
	host := trim(body.Host)
	username := trim(body.Username)

	var authMethod *string
	if body.AuthMethod != nil {
		v := strings.TrimSpace(strings.ToLower(*body.AuthMethod))
		if v != "" && v != "key" && v != "password" {
			httpx.Error(w, http.StatusBadRequest, "auth_method must be key or password")
			return
		}
		if v != "" {
			authMethod = &v
		}
	}

	var privEnc *string
	if body.PrivateKey != nil {
		pk := strings.TrimSpace(*body.PrivateKey)
		if pk != "" {
			enc, err := secrets.EncryptString(k, pk)
			if err != nil {
				httpx.Error(w, http.StatusInternalServerError, "encrypt private key")
				return
			}
			privEnc = &enc
		}
	}

	setPassphrase := body.Passphrase != nil
	var passEnc *string
	if body.Passphrase != nil {
		pp := strings.TrimSpace(*body.Passphrase)
		if pp == "" {
			passEnc = nil // clear
		} else {
			enc, err := secrets.EncryptString(k, pp)
			if err != nil {
				httpx.Error(w, http.StatusInternalServerError, "encrypt passphrase")
				return
			}
			passEnc = &enc
		}
	}

	setPassword := body.Password != nil
	var pwdEnc *string
	if body.Password != nil {
		pw := strings.TrimSpace(*body.Password)
		if pw == "" {
			pwdEnc = nil // clear
		} else {
			enc, err := secrets.EncryptString(k, pw)
			if err != nil {
				httpx.Error(w, http.StatusInternalServerError, "encrypt password")
				return
			}
			pwdEnc = &enc
		}
	}

	conn, err := h.Store.UpdateSSHConnection(
		r.Context(),
		orgID,
		uid,
		connID,
		name,
		host,
		body.Port,
		username,
		authMethod,
		privEnc,
		setPassphrase,
		passEnc,
		setPassword,
		pwdEnc,
	)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "update ssh connection")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"connection": conn})
}

func (h *SSHConnectionsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.SSHConnectionsManage) {
		return
	}
	connID, err := uuid.Parse(chi.URLParam(r, "connID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid connection id")
		return
	}
	ok2, err := h.Store.DeleteSSHConnection(r.Context(), orgID, uid, connID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "delete ssh connection")
		return
	}
	if !ok2 {
		httpx.Error(w, http.StatusNotFound, "not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

