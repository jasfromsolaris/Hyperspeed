package chatai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"hyperspeed/api/internal/agenttools"
	"hyperspeed/api/internal/cursor"
	"hyperspeed/api/internal/events"
	"hyperspeed/api/internal/store"
)

const maxFileContextBytes = 24000
const maxFilesToRead = 15
const maxChatHistory = 40

const (
	maxUserReferencedFiles      = 12
	maxUserRefFileContentBytes  = 12000
	maxUserReferencedTotalBytes = 40000
)

// StaffMessageOptions tweaks staff system prompts and optional pre-read context.
type StaffMessageOptions struct {
	// ToolsEnabled: model may call Hyperspeed tools (system prompt reflects this).
	ToolsEnabled bool
	// SkipPreread skips bulk file injection (saves tokens when the model can call space.file.read).
	SkipPreread bool
}

func decodeSecretsKey(b64 string) ([]byte, error) {
	k := strings.TrimSpace(b64)
	if k == "" {
		return nil, nil
	}
	return base64.StdEncoding.DecodeString(k)
}

// BuildStaffMessages builds OpenAI-shaped messages for OpenRouter or for assembling a Cloud Agent prompt.
func (w *Worker) BuildStaffMessages(ctx context.Context, req events.ChatAIMentionRequestedPayload, src store.ChatMessage, history []store.ChatMessage) ([]cursor.Message, error) {
	return w.BuildStaffMessagesWithOptions(ctx, req, src, history, StaffMessageOptions{})
}

// BuildStaffMessagesWithOptions is like BuildStaffMessages but supports OpenRouter tool mode.
func (w *Worker) BuildStaffMessagesWithOptions(ctx context.Context, req events.ChatAIMentionRequestedPayload, src store.ChatMessage, history []store.ChatMessage, opt StaffMessageOptions) ([]cursor.Message, error) {
	var fileBlock string
	var err error
	if !opt.SkipPreread {
		fileBlock, err = w.readOnlyFileContext(ctx, req.OrganizationID, req.SpaceID, req.AIUserID)
		if err != nil {
			return nil, err
		}
	}

	refIDs := parseChatFileRefUUIDsOrderedUnique(src.Content)
	refBlock, refNames, err := w.userReferencedFileContext(ctx, req.OrganizationID, req.SpaceID, req.AIUserID, refIDs)
	if err != nil {
		return nil, err
	}

	var hist strings.Builder
	hist.WriteString("Recent chat (oldest first):\n")
	for _, m := range history {
		if m.DeletedAt != nil {
			continue
		}
		role := "user"
		if m.AuthorID != nil && *m.AuthorID == req.AIUserID {
			role = "assistant"
		}
		line := strings.TrimSpace(stripChatMarkupTokens(m.Content))
		line = strings.Join(strings.Fields(line), " ")
		if len(line) > 1500 {
			line = line[:1500] + "…"
		}
		if line == "" {
			continue
		}
		fmt.Fprintf(&hist, "[%s] %s\n", role, line)
	}

	srcLine := strings.TrimSpace(replaceChatFileRefsWithLabels(src.Content, refNames))
	srcLine = strings.TrimSpace(reMentionTokens.ReplaceAllString(srcLine, ""))
	srcLine = strings.Join(strings.Fields(srcLine), " ")
	if len(srcLine) > 2000 {
		srcLine = srcLine[:2000] + "…"
	}

	sys := strings.Builder{}
	sys.WriteString("You are the designated AI staff member in a Hyperspeed organization chat. ")
	sys.WriteString("Reply helpfully and concisely in Markdown. ")
	if opt.ToolsEnabled {
		sys.WriteString("You have tools: read/list space files, read recent chat, create new text files, and propose edits to existing files (proposals require human acceptance in the web UI). ")
		sys.WriteString("Prefer space.file.propose_patch for changing existing files; use space.file.create_text for new files. After a successful propose_patch, briefly confirm the change; the UI shows an inline diff card in chat for accept/reject. ")
		sys.WriteString("Web search and datetime may be available via OpenRouter when relevant.\n")
	} else {
		sys.WriteString("You only have read-only file excerpts from this space; you cannot run tools from here.\n")
	}
	fmt.Fprintf(&sys, "Organization ID: %s\nSpace ID: %s\nChat room ID: %s\n", req.OrganizationID, req.SpaceID, req.ChatRoomID)
	fmt.Fprintf(&sys, "When calling tools, use these exact UUIDs for space_id and chat_room_id as appropriate.\n")
	if refBlock != "" {
		sys.WriteString("\n--- User-referenced files (prioritize these) ---\n")
		sys.WriteString(refBlock)
	}
	if fileBlock != "" {
		sys.WriteString("\n--- Read-only file context ---\n")
		sys.WriteString(fileBlock)
	}

	msgs := []cursor.Message{
		{Role: "system", Content: sys.String()},
		{Role: "user", Content: hist.String()},
		{Role: "user", Content: fmt.Sprintf("The user <@%s> mentioned you. Their message:\n%s\n\nReply addressing them.", req.RequestedByUserID.String(), srcLine)},
	}
	return msgs, nil
}

func (w *Worker) readOnlyFileContext(ctx context.Context, orgID, spaceID, aiUserID uuid.UUID) (string, error) {
	if w.Harness == nil {
		return "", nil
	}
	listArgs, _ := json.Marshal(map[string]string{"space_id": spaceID.String()})
	out, err := w.Harness.Invoke(ctx, orgID, aiUserID, agenttools.InvokeInput{
		Tool: "space.list_files", Arguments: listArgs, Mode: "agent",
	})
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	var wrap struct {
		Nodes []store.FileNode `json:"nodes"`
	}
	if err := json.Unmarshal(b, &wrap); err != nil {
		return "", err
	}

	var sb strings.Builder
	used := 0
	nread := 0
	for _, n := range wrap.Nodes {
		if n.Kind != store.FileNodeFile || n.DeletedAt != nil {
			continue
		}
		if nread >= maxFilesToRead || used >= maxFileContextBytes {
			break
		}
		readArgs, _ := json.Marshal(map[string]string{
			"space_id": spaceID.String(),
			"node_id":  n.ID.String(),
		})
		ro, err := w.Harness.Invoke(ctx, orgID, aiUserID, agenttools.InvokeInput{
			Tool: "space.file.read", Arguments: readArgs, Mode: "agent",
		})
		if err != nil {
			continue
		}
		rb, err := json.Marshal(ro)
		if err != nil {
			continue
		}
		var fr struct {
			Content string `json:"content"`
		}
		if err := json.Unmarshal(rb, &fr); err != nil {
			continue
		}
		c := fr.Content
		if len(c) > 8000 {
			c = c[:8000] + "\n…(truncated)"
		}
		chunk := fmt.Sprintf("--- file: %s (%s) ---\n%s\n", n.Name, n.ID.String(), c)
		if used+len(chunk) > maxFileContextBytes {
			break
		}
		sb.WriteString(chunk)
		used += len(chunk)
		nread++
	}
	return sb.String(), nil
}

func parseChatFileRefUUIDsOrderedUnique(content string) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{})
	var out []uuid.UUID
	for _, sub := range reChatFileRefTagged.FindAllStringSubmatch(content, -1) {
		if len(sub) < 2 {
			continue
		}
		id, err := uuid.Parse(sub[1])
		if err != nil {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func replaceChatFileRefsWithLabels(content string, labels map[uuid.UUID]string) string {
	return reChatFileRefTagged.ReplaceAllStringFunc(content, func(full string) string {
		sub := reChatFileRefTagged.FindStringSubmatch(full)
		if len(sub) < 2 {
			return full
		}
		id, err := uuid.Parse(sub[1])
		if err != nil {
			return full
		}
		if labels != nil {
			if name, ok := labels[id]; ok && strings.TrimSpace(name) != "" {
				return "[file: " + name + "]"
			}
		}
		return "[file]"
	})
}

// userReferencedFileContext loads content for <#uuid> file tokens in the triggering message.
// nameByID maps node id -> file name for prompt labeling (even when read fails, name may be absent).
func (w *Worker) userReferencedFileContext(ctx context.Context, orgID, spaceID, aiUserID uuid.UUID, ids []uuid.UUID) (block string, nameByID map[uuid.UUID]string, err error) {
	nameByID = make(map[uuid.UUID]string)
	if len(ids) == 0 || w.Harness == nil {
		return "", nameByID, nil
	}
	var sb strings.Builder
	used := 0
	nread := 0
	for _, nodeID := range ids {
		if nread >= maxUserReferencedFiles || used >= maxUserReferencedTotalBytes {
			break
		}
		readArgs, _ := json.Marshal(map[string]string{
			"space_id": spaceID.String(),
			"node_id":  nodeID.String(),
		})
		ro, err := w.Harness.Invoke(ctx, orgID, aiUserID, agenttools.InvokeInput{
			Tool: "space.file.read", Arguments: readArgs, Mode: "agent",
		})
		if err != nil {
			continue
		}
		rb, err := json.Marshal(ro)
		if err != nil {
			continue
		}
		var fr struct {
			Node    store.FileNode `json:"node"`
			Content string         `json:"content"`
		}
		if err := json.Unmarshal(rb, &fr); err != nil {
			continue
		}
		nameByID[nodeID] = fr.Node.Name
		c := fr.Content
		if len(c) > maxUserRefFileContentBytes {
			c = c[:maxUserRefFileContentBytes] + "\n…(truncated)"
		}
		chunk := fmt.Sprintf("--- file: %s (%s) ---\n%s\n", fr.Node.Name, nodeID.String(), c)
		if used+len(chunk) > maxUserReferencedTotalBytes {
			break
		}
		sb.WriteString(chunk)
		used += len(chunk)
		nread++
	}
	return sb.String(), nameByID, nil
}
