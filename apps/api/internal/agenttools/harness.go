package agenttools

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"hyperspeed/api/internal/files"
	"hyperspeed/api/internal/rbac"
	"hyperspeed/api/internal/store"
)

type Harness struct {
	Store *store.Store
	OS    *files.ObjectStore
}

type InvokeInput struct {
	Tool      string
	Arguments json.RawMessage
	SessionID *string
	// Mode is ask | plan | agent (default agent). Enforced before tool execution.
	Mode string
}

// NormalizeInvokeMode returns a valid mode or "agent" for empty/unknown legacy clients.
func NormalizeInvokeMode(mode string) string {
	switch mode {
	case "ask", "plan", "agent":
		return mode
	default:
		return "agent"
	}
}

func modeAllowsTool(mode, tool string) bool {
	switch mode {
	case "ask", "plan":
		return tool == "space.file.read" || tool == "space.list_files" || tool == "space.chat.read_recent"
	case "agent", "":
		return true
	default:
		return true
	}
}

func (h *Harness) Invoke(ctx context.Context, orgID, userID uuid.UUID, in InvokeInput) (any, error) {
	if h.Store == nil || h.OS == nil {
		return nil, errors.New("harness not configured")
	}
	mode := NormalizeInvokeMode(in.Mode)
	if !modeAllowsTool(mode, in.Tool) {
		return nil, errModePolicy(mode, in.Tool)
	}
	switch in.Tool {
	case "space.file.read":
		return h.toolFileRead(ctx, orgID, userID, in.Arguments)
	case "space.list_files":
		return h.toolListFiles(ctx, orgID, userID, in.Arguments)
	case "space.chat.read_recent":
		return h.toolChatReadRecent(ctx, orgID, userID, in.Arguments)
	case "space.file.propose_patch":
		return h.toolProposePatch(ctx, orgID, userID, in.Arguments)
	case "space.file.create_text":
		return h.toolCreateTextFile(ctx, orgID, userID, in.Arguments)
	case "space.automation.propose":
		return h.toolAutomationPropose(ctx, orgID, userID, in.Arguments)
	default:
		return nil, fmt.Errorf("unknown tool %q", in.Tool)
	}
}

type fileReadArgs struct {
	SpaceID uuid.UUID `json:"space_id"`
	NodeID  uuid.UUID `json:"node_id"`
}

func (h *Harness) toolFileRead(ctx context.Context, orgID, userID uuid.UUID, raw json.RawMessage) (any, error) {
	ok, err := rbac.HasPermission(ctx, h.Store, orgID, userID, rbac.FilesRead)
	if err != nil || !ok {
		return nil, errForbiddenFiles()
	}
	var a fileReadArgs
	if err := json.Unmarshal(raw, &a); err != nil {
		return nil, errBadArgs()
	}
	if a.SpaceID == uuid.Nil || a.NodeID == uuid.Nil {
		return nil, errBadArgs()
	}
	if _, err := h.Store.GetSpace(ctx, orgID, a.SpaceID); err != nil {
		if err == pgx.ErrNoRows {
			return nil, errNotFound()
		}
		return nil, err
	}
	ok, err = h.Store.UserCanAccessSpace(ctx, orgID, a.SpaceID, userID)
	if err != nil || !ok {
		return nil, errForbiddenSpace()
	}
	n, err := h.Store.FileNodeByID(ctx, a.SpaceID, a.NodeID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errNotFound()
		}
		return nil, err
	}
	if n.Kind != store.FileNodeFile || n.StorageKey == nil || *n.StorageKey == "" || n.DeletedAt != nil {
		return nil, errors.New("not a readable file")
	}
	b, err := h.OS.GetBytes(ctx, *n.StorageKey, 2<<20)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"node":    n,
		"content": string(b),
	}, nil
}

type listFilesArgs struct {
	SpaceID  uuid.UUID  `json:"space_id"`
	ParentID *uuid.UUID `json:"parent_id"`
}

func (h *Harness) toolListFiles(ctx context.Context, orgID, userID uuid.UUID, raw json.RawMessage) (any, error) {
	ok, err := rbac.HasPermission(ctx, h.Store, orgID, userID, rbac.FilesRead)
	if err != nil || !ok {
		return nil, errForbiddenFiles()
	}
	var a listFilesArgs
	if err := json.Unmarshal(raw, &a); err != nil {
		return nil, errBadArgs()
	}
	if a.SpaceID == uuid.Nil {
		return nil, errBadArgs()
	}
	if _, err := h.Store.GetSpace(ctx, orgID, a.SpaceID); err != nil {
		if err == pgx.ErrNoRows {
			return nil, errNotFound()
		}
		return nil, err
	}
	ok, err = h.Store.UserCanAccessSpace(ctx, orgID, a.SpaceID, userID)
	if err != nil || !ok {
		return nil, errForbiddenSpace()
	}
	nodes, err := h.Store.ListFileNodes(ctx, a.SpaceID, a.ParentID, "", false)
	if err != nil {
		return nil, err
	}
	if nodes == nil {
		nodes = []store.FileNode{}
	}
	return map[string]any{"nodes": nodes}, nil
}

type chatReadRecentArgs struct {
	SpaceID    uuid.UUID `json:"space_id"`
	ChatRoomID uuid.UUID `json:"chat_room_id"`
	Limit      int       `json:"limit"`
}

func (h *Harness) toolChatReadRecent(ctx context.Context, orgID, userID uuid.UUID, raw json.RawMessage) (any, error) {
	ok, err := rbac.HasPermission(ctx, h.Store, orgID, userID, rbac.ChatRead)
	if err != nil || !ok {
		return nil, errForbiddenChat()
	}
	var a chatReadRecentArgs
	if err := json.Unmarshal(raw, &a); err != nil {
		return nil, errBadArgs()
	}
	if a.SpaceID == uuid.Nil || a.ChatRoomID == uuid.Nil {
		return nil, errBadArgs()
	}
	limit := a.Limit
	if limit <= 0 {
		limit = 30
	}
	if limit > 100 {
		limit = 100
	}
	if _, err := h.Store.GetSpace(ctx, orgID, a.SpaceID); err != nil {
		if err == pgx.ErrNoRows {
			return nil, errNotFound()
		}
		return nil, err
	}
	ok, err = h.Store.UserCanAccessSpace(ctx, orgID, a.SpaceID, userID)
	if err != nil || !ok {
		return nil, errForbiddenSpace()
	}
	if _, err := h.Store.GetChatRoomInSpace(ctx, a.SpaceID, a.ChatRoomID); err != nil {
		if err == pgx.ErrNoRows {
			return nil, errNotFound()
		}
		return nil, err
	}
	msgs, err := h.Store.ListChatMessages(ctx, a.ChatRoomID, limit, nil)
	if err != nil {
		return nil, err
	}
	type row struct {
		ID        uuid.UUID  `json:"id"`
		AuthorID  *uuid.UUID `json:"author_user_id,omitempty"`
		Content   string     `json:"content"`
		CreatedAt time.Time  `json:"created_at"`
	}
	out := make([]row, 0, len(msgs))
	for _, m := range msgs {
		if m.DeletedAt != nil {
			continue
		}
		c := m.Content
		if len(c) > 4000 {
			c = c[:4000] + "…"
		}
		out = append(out, row{
			ID:        m.ID,
			AuthorID:  m.AuthorID,
			Content:   c,
			CreatedAt: m.CreatedAt,
		})
	}
	return map[string]any{"messages": out}, nil
}

type proposePatchArgs struct {
	SpaceID         uuid.UUID `json:"space_id"`
	NodeID          uuid.UUID `json:"node_id"`
	ProposedContent string    `json:"proposed_content"`
}

func (h *Harness) toolProposePatch(ctx context.Context, orgID, userID uuid.UUID, raw json.RawMessage) (any, error) {
	ok, err := rbac.HasPermission(ctx, h.Store, orgID, userID, rbac.FilesWrite)
	if err != nil || !ok {
		return nil, errForbiddenFiles()
	}
	var a proposePatchArgs
	if err := json.Unmarshal(raw, &a); err != nil {
		return nil, errBadArgs()
	}
	if a.SpaceID == uuid.Nil || a.NodeID == uuid.Nil {
		return nil, errBadArgs()
	}
	if _, err := h.Store.GetSpace(ctx, orgID, a.SpaceID); err != nil {
		if err == pgx.ErrNoRows {
			return nil, errNotFound()
		}
		return nil, err
	}
	ok, err = h.Store.UserCanAccessSpace(ctx, orgID, a.SpaceID, userID)
	if err != nil || !ok {
		return nil, errForbiddenSpace()
	}
	n, err := h.Store.FileNodeByID(ctx, a.SpaceID, a.NodeID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errNotFound()
		}
		return nil, err
	}
	if n.Kind != store.FileNodeFile || n.StorageKey == nil || *n.StorageKey == "" || n.DeletedAt != nil {
		return nil, errors.New("not a writable file")
	}
	b, err := h.OS.GetBytes(ctx, *n.StorageKey, 2<<20)
	if err != nil {
		return nil, err
	}
	base := sha256.Sum256(b)
	baseHex := hex.EncodeToString(base[:])
	p, err := h.Store.CreateFileEditProposal(ctx, orgID, a.SpaceID, a.NodeID, userID, baseHex, string(b), a.ProposedContent)
	if err != nil {
		return nil, err
	}
	return map[string]any{"proposal": p}, nil
}

type createTextFileArgs struct {
	SpaceID  uuid.UUID  `json:"space_id"`
	ParentID *uuid.UUID `json:"parent_id"`
	Name     string     `json:"name"`
	Content  string     `json:"content"`
}

func (h *Harness) toolCreateTextFile(ctx context.Context, orgID, userID uuid.UUID, raw json.RawMessage) (any, error) {
	ok, err := rbac.HasPermission(ctx, h.Store, orgID, userID, rbac.FilesWrite)
	if err != nil || !ok {
		return nil, errForbiddenFiles()
	}
	var a createTextFileArgs
	if err := json.Unmarshal(raw, &a); err != nil {
		return nil, errBadArgs()
	}
	if a.SpaceID == uuid.Nil {
		return nil, errBadArgs()
	}
	name := strings.TrimSpace(a.Name)
	if name == "" {
		return nil, errBadArgs()
	}
	if _, err := h.Store.GetSpace(ctx, orgID, a.SpaceID); err != nil {
		if err == pgx.ErrNoRows {
			return nil, errNotFound()
		}
		return nil, err
	}
	ok, err = h.Store.UserCanAccessSpace(ctx, orgID, a.SpaceID, userID)
	if err != nil || !ok {
		return nil, errForbiddenSpace()
	}
	mime := "text/plain"
	b := []byte(a.Content)
	size := int64(len(b))
	storageKey := "org/" + orgID.String() + "/project/" + a.SpaceID.String() + "/node/" + uuid.NewString() + "/" + name
	n, err := h.Store.CreateFileNode(ctx, a.SpaceID, a.ParentID, name, &mime, &size, storageKey, userID)
	if err != nil {
		return nil, err
	}
	if err := h.OS.Put(ctx, storageKey, mime, bytes.NewReader(b), &size); err != nil {
		return nil, err
	}
	return map[string]any{"node": n}, nil
}

type automationProposeArgs struct {
	SpaceID uuid.UUID       `json:"space_id"`
	Name    string          `json:"name"`
	Kind    string          `json:"kind"`
	Config  json.RawMessage `json:"config"`
	Note    string          `json:"note"`
}

func (h *Harness) toolAutomationPropose(ctx context.Context, orgID, userID uuid.UUID, raw json.RawMessage) (any, error) {
	ok, err := rbac.HasPermission(ctx, h.Store, orgID, userID, rbac.AgentToolsInvoke)
	if err != nil || !ok {
		return nil, errors.New("agent.tools.invoke permission denied")
	}
	ok, err = rbac.HasPermission(ctx, h.Store, orgID, userID, rbac.FilesWrite)
	if err != nil || !ok {
		return nil, errForbiddenFiles()
	}
	var a automationProposeArgs
	if err := json.Unmarshal(raw, &a); err != nil {
		return nil, errBadArgs()
	}
	name := strings.TrimSpace(a.Name)
	kind := strings.TrimSpace(strings.ToLower(a.Kind))
	if a.SpaceID == uuid.Nil || name == "" || kind == "" {
		return nil, errBadArgs()
	}
	if kind != "social_post" && kind != "reverse_tunnel" && kind != "scheduled" && kind != "webhook" {
		return nil, errBadArgs()
	}
	if _, err := h.Store.GetSpace(ctx, orgID, a.SpaceID); err != nil {
		if err == pgx.ErrNoRows {
			return nil, errNotFound()
		}
		return nil, err
	}
	ok, err = h.Store.UserCanAccessSpace(ctx, orgID, a.SpaceID, userID)
	if err != nil || !ok {
		return nil, errForbiddenSpace()
	}
	cfg := a.Config
	if cfg == nil {
		cfg = json.RawMessage(`{}`)
	}
	if strings.TrimSpace(a.Note) != "" {
		var m map[string]any
		_ = json.Unmarshal(cfg, &m)
		if m == nil {
			m = map[string]any{}
		}
		m["proposal_note"] = strings.TrimSpace(a.Note)
		cfg, _ = json.Marshal(m)
	}
	var cu *uuid.UUID
	var csa *uuid.UUID
	if sarow, err := h.Store.GetServiceAccountByUserInOrg(ctx, orgID, userID); err == nil {
		x := sarow.ID
		csa = &x
	} else {
		cu = &userID
	}
	auto, err := h.Store.CreateSpaceAutomation(ctx, orgID, a.SpaceID, store.CreateSpaceAutomationInput{
		Name:                      name,
		Kind:                      kind,
		Config:                    cfg,
		Status:                    "pending_approval",
		CreatedByUserID:           cu,
		CreatedByServiceAccountID: csa,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"automation": auto}, nil
}

type HarnessError struct {
	Code    string
	Message string
}

func (e *HarnessError) Error() string { return e.Message }

func errForbiddenFiles() error { return &HarnessError{Code: "forbidden", Message: "files permission denied"} }
func errForbiddenChat() error  { return &HarnessError{Code: "forbidden", Message: "chat permission denied"} }
func errForbiddenSpace() error { return &HarnessError{Code: "forbidden", Message: "no access to space"} }
func errBadArgs() error         { return &HarnessError{Code: "invalid_arguments", Message: "invalid arguments"} }
func errNotFound() error        { return &HarnessError{Code: "not_found", Message: "not found"} }
func errModePolicy(mode, tool string) error {
	return &HarnessError{
		Code:    "mode_policy",
		Message: fmt.Sprintf("tool %q is not allowed in %q mode", tool, mode),
	}
}

func classifyInvokeError(err error) string {
	if err == nil {
		return "ok"
	}
	if he, ok := IsHarnessError(err); ok {
		return he.Code
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "timeout"), strings.Contains(msg, "deadline"):
		return "timeout"
	case strings.Contains(msg, "network"), strings.Contains(msg, "connection"), strings.Contains(msg, "dial"):
		return "network"
	case strings.Contains(msg, "unauthorized"), strings.Contains(msg, "auth"):
		return "auth"
	default:
		return "internal"
	}
}

// IsHarnessError reports whether err is a typed harness error with Code.
func IsHarnessError(err error) (*HarnessError, bool) {
	var he *HarnessError
	if errors.As(err, &he) {
		return he, true
	}
	return nil, false
}

// LogInvocation writes an audit row (best-effort).
func (h *Harness) LogInvocation(ctx context.Context, orgID, userID uuid.UUID, sessionID *string, tool string, args json.RawMessage, result any, invokeErr error, start time.Time) {
	var resJSON json.RawMessage
	if result != nil {
		b, err := json.Marshal(result)
		if err == nil {
			resJSON = b
		}
	}
	var errText *string
	if invokeErr != nil {
		category := classifyInvokeError(invokeErr)
		s := fmt.Sprintf("[%s] %s", category, invokeErr.Error())
		errText = &s
	}
	ms := int(time.Since(start).Milliseconds())
	_ = h.Store.InsertAgentToolInvocation(ctx, orgID, userID, sessionID, tool, args, resJSON, errText, &ms)
}
