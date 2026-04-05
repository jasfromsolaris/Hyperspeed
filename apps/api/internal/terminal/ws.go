package terminal

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/ssh"

	"hyperspeed/api/internal/auth"
	"hyperspeed/api/internal/ctxkey"
	"hyperspeed/api/internal/httpx"
	"hyperspeed/api/internal/rbac"
	"hyperspeed/api/internal/secrets"
	"hyperspeed/api/internal/store"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type WSHandler struct {
	Auth         *auth.Service
	Store        *store.Store
	EncryptKeyB64 string
	encryptKeyOnce []byte
}

func (h *WSHandler) key() ([]byte, error) {
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

type clientMsg struct {
	Type string `json:"type"`
	Data string `json:"data,omitempty"`
	Cols int    `json:"cols,omitempty"`
	Rows int    `json:"rows,omitempty"`
	Method   string `json:"method,omitempty"`   // password
	Password string `json:"password,omitempty"` // sent over WS, never via query string
}

type serverMsg struct {
	Type         string    `json:"type"`
	Data         string    `json:"data,omitempty"`
	Message      string    `json:"message,omitempty"`
	ConnectionID uuid.UUID `json:"connection_id,omitempty"`
}

func (h *WSHandler) ServeWS(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if strings.TrimSpace(token) == "" {
		httpx.Error(w, http.StatusUnauthorized, "token required")
		return
	}
	claims, err := h.Auth.ParseAccess(token)
	if err != nil {
		httpx.Error(w, http.StatusUnauthorized, "invalid token")
		return
	}
	ctx := ctxkey.WithUserID(r.Context(), claims.UserID)

	orgID, err := uuid.Parse(chi.URLParam(r, "orgID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid organization id")
		return
	}
	spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid space id")
		return
	}
	if _, err := h.Store.MemberRole(ctx, orgID, claims.UserID); err != nil {
		httpx.Error(w, http.StatusForbidden, "forbidden")
		return
	}

	ok, err := rbac.HasPermission(ctx, h.Store, orgID, claims.UserID, rbac.TerminalUse)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "permissions")
		return
	}
	if !ok {
		httpx.Error(w, http.StatusForbidden, "forbidden")
		return
	}

	if _, err := h.Store.SpaceMemberRole(ctx, spaceID, claims.UserID); err != nil {
		if err == pgx.ErrNoRows {
			// Org-level override for admins/managers (mirrors middleware.RequireProjectMember).
			if ok, _ := rbac.HasPermission(ctx, h.Store, orgID, claims.UserID, rbac.OrgManage); ok {
				// allow
			} else if ok, _ := rbac.HasPermission(ctx, h.Store, orgID, claims.UserID, rbac.SpaceMembersManage); ok {
				// allow
			} else {
			httpx.Error(w, http.StatusForbidden, "not a member of this space")
			return
			}
		} else {
			httpx.Error(w, http.StatusInternalServerError, "membership")
			return
		}
	}

	connIDStr := r.URL.Query().Get("connectionId")
	connID, err := uuid.Parse(connIDStr)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "connectionId required")
		return
	}

	k, err := h.key()
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	wsConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer wsConn.Close()

	writeMu := &sync.Mutex{}
	write := func(m serverMsg) {
		b, _ := json.Marshal(m)
		writeMu.Lock()
		_ = wsConn.WriteMessage(websocket.TextMessage, b)
		writeMu.Unlock()
	}

	sshConn, err := h.Store.GetSSHConnectionSecret(ctx, orgID, claims.UserID, connID)
	if err != nil {
		if err == pgx.ErrNoRows {
			write(serverMsg{Type: "error", Message: "connection not found"})
			return
		}
		write(serverMsg{Type: "error", Message: "load connection failed"})
		return
	}

	var authMethods []ssh.AuthMethod
	switch sshConn.AuthMethod {
	case "password":
		// Use stored password if present, else wait for client to send {type:"auth", method:"password", password:"..."}.
		var pw string
		if sshConn.PasswordEnc != nil && strings.TrimSpace(*sshConn.PasswordEnc) != "" {
			pp, err := secrets.DecryptString(k, *sshConn.PasswordEnc)
			if err != nil {
				write(serverMsg{Type: "error", Message: "decrypt password failed"})
				return
			}
			pw = pp
		} else {
			_ = wsConn.SetReadDeadline(time.Now().Add(60 * time.Second))
			for {
				_, data, err := wsConn.ReadMessage()
				if err != nil {
					write(serverMsg{Type: "error", Message: "password required"})
					return
				}
				var m clientMsg
				if err := json.Unmarshal(data, &m); err != nil {
					continue
				}
				if m.Type == "auth" && strings.ToLower(strings.TrimSpace(m.Method)) == "password" {
					pw = m.Password
					break
				}
			}
			_ = wsConn.SetReadDeadline(time.Time{})
		}
		if strings.TrimSpace(pw) == "" {
			write(serverMsg{Type: "error", Message: "password required"})
			return
		}
		authMethods = []ssh.AuthMethod{ssh.Password(pw)}
	default:
		// key auth
		if sshConn.PrivateKeyEnc == nil || strings.TrimSpace(*sshConn.PrivateKeyEnc) == "" {
			write(serverMsg{Type: "error", Message: "private key required"})
			return
		}
		privateKeyPEM, err := secrets.DecryptString(k, *sshConn.PrivateKeyEnc)
		if err != nil {
			write(serverMsg{Type: "error", Message: "decrypt private key failed"})
			return
		}
		var passphrase string
		if sshConn.PassphraseEnc != nil && strings.TrimSpace(*sshConn.PassphraseEnc) != "" {
			pp, err := secrets.DecryptString(k, *sshConn.PassphraseEnc)
			if err != nil {
				write(serverMsg{Type: "error", Message: "decrypt passphrase failed"})
				return
			}
			passphrase = pp
		}
		var signer ssh.Signer
		if passphrase != "" {
			s, err := ssh.ParsePrivateKeyWithPassphrase([]byte(privateKeyPEM), []byte(passphrase))
			if err != nil {
				write(serverMsg{Type: "error", Message: "invalid private key or passphrase"})
				return
			}
			signer = s
		} else {
			s, err := ssh.ParsePrivateKey([]byte(privateKeyPEM))
			if err != nil {
				write(serverMsg{Type: "error", Message: "invalid private key"})
				return
			}
			signer = s
		}
		authMethods = []ssh.AuthMethod{ssh.PublicKeys(signer)}
	}

	addr := net.JoinHostPort(sshConn.Host, fmt.Sprintf("%d", sshConn.Port))
	client, err := ssh.Dial("tcp", addr, &ssh.ClientConfig{
		User:            sshConn.Username,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	})
	if err != nil {
		write(serverMsg{Type: "error", Message: "ssh dial failed"})
		return
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		write(serverMsg{Type: "error", Message: "ssh session failed"})
		return
	}
	defer session.Close()

	stdin, err := session.StdinPipe()
	if err != nil {
		write(serverMsg{Type: "error", Message: "stdin failed"})
		return
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		write(serverMsg{Type: "error", Message: "stdout failed"})
		return
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		write(serverMsg{Type: "error", Message: "stderr failed"})
		return
	}

	cols, rows := 80, 24
	if err := session.RequestPty("xterm-256color", rows, cols, ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}); err != nil {
		write(serverMsg{Type: "error", Message: "pty failed"})
		return
	}
	if err := session.Shell(); err != nil {
		write(serverMsg{Type: "error", Message: "shell failed"})
		return
	}
	write(serverMsg{Type: "connected", ConnectionID: sshConn.ID})

	ctx2, cancel := context.WithCancel(ctx)
	defer cancel()

	// Server -> client output
	pump := func(r io.Reader) {
		buf := make([]byte, 32*1024)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				write(serverMsg{Type: "output", Data: string(buf[:n])})
			}
			if err != nil {
				cancel()
				return
			}
		}
	}
	go pump(stdout)
	go pump(stderr)

	// Client -> server input
	go func() {
		for {
			_, data, err := wsConn.ReadMessage()
			if err != nil {
				cancel()
				return
			}
			var m clientMsg
			if err := json.Unmarshal(data, &m); err != nil {
				continue
			}
			switch m.Type {
			case "auth":
				// Only used during initial handshake for password auth.
				continue
			case "input":
				if m.Data != "" {
					_, _ = io.WriteString(stdin, m.Data)
				}
			case "resize":
				if m.Cols > 0 && m.Rows > 0 {
					_ = session.WindowChange(m.Rows, m.Cols)
				}
			case "ping":
				// no-op
			}
		}
	}()

	select {
	case <-ctx2.Done():
		return
	case <-r.Context().Done():
		return
	}
}

