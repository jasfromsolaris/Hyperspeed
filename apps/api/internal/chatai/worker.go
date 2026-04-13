package chatai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"

	"hyperspeed/api/internal/agenttools"
	"hyperspeed/api/internal/chatmentions"
	"hyperspeed/api/internal/cursor"
	"hyperspeed/api/internal/cursor/agents"
	"hyperspeed/api/internal/events"
	"hyperspeed/api/internal/openrouter"
	"hyperspeed/api/internal/store"
)

var (
	reMentionTokens       = regexp.MustCompile(`<@&?[0-9a-fA-F-]{36}>`)
	reChatFileRefTagged   = regexp.MustCompile(`<#([0-9a-fA-F-]{36})>`)
)

// stripChatMarkupTokens removes user/role and file reference tokens from raw message text.
func stripChatMarkupTokens(s string) string {
	s = reMentionTokens.ReplaceAllString(s, "")
	s = reChatFileRefTagged.ReplaceAllString(s, "")
	return s
}

// cleanUserMessageSnippet is used for Cursor agent prompts and tests: strips mention/file tokens and wraps for readability.
func cleanUserMessageSnippet(raw string) string {
	s := strings.TrimSpace(stripChatMarkupTokens(raw))
	return "About your message:\n" + s
}

type Worker struct {
	Store         *store.Store
	Rdb           *redis.Client
	Bus           *events.Bus
	EncryptKeyB64 string
	OpenRouter    *openrouter.Client
	Agents        *agents.Client
	Harness       *agenttools.Harness
	ORTooling     OpenRouterChatTooling
	// Debug logs which OpenRouter model is used per mention (no silent fallback in this path).
	Debug bool
}

func (w *Worker) Start(ctx context.Context) error {
	if w == nil || w.Store == nil || w.Rdb == nil || w.Bus == nil {
		return nil
	}
	pubsub := w.Rdb.PSubscribe(ctx, events.ChannelPrefix+"*")
	defer pubsub.Close()
	if _, err := pubsub.Receive(ctx); err != nil {
		return err
	}
	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return nil
		case msg, ok := <-ch:
			if !ok || msg == nil {
				return nil
			}
			env, err := events.Parse([]byte(msg.Payload))
			if err != nil || env.Type != events.ChatAIMentionRequested {
				continue
			}
			var req events.ChatAIMentionRequestedPayload
			if err := events.UnmarshalPayload(env, &req); err != nil {
				slog.Warn("chat ai mention payload unmarshal", "err", err)
				continue
			}
			if req.OrganizationID == uuid.Nil {
				req.OrganizationID = env.OrganizationID
			}
			if req.OrganizationID == uuid.Nil || req.SpaceID == uuid.Nil || req.ChatRoomID == uuid.Nil || req.SourceMessageID == uuid.Nil || req.AIUserID == uuid.Nil || req.RequestedByUserID == uuid.Nil {
				continue
			}
			runCtx, cancel := context.WithTimeout(ctx, 25*time.Minute)
			if err := w.handle(runCtx, req); err != nil {
				slog.Error("chat ai mention worker", "err", err)
			}
			cancel()
		}
	}
}

func (w *Worker) logSkipMention(req events.ChatAIMentionRequestedPayload, reason string) {
	slog.Info("chat ai mention skipped",
		"reason", reason,
		"org_id", req.OrganizationID,
		"space_id", req.SpaceID,
		"chat_room_id", req.ChatRoomID,
		"source_message_id", req.SourceMessageID,
		"ai_user_id", req.AIUserID,
	)
}

func (w *Worker) handle(ctx context.Context, req events.ChatAIMentionRequestedPayload) error {
	inserted, _, err := w.Store.CreateChatAIMentionReplyRecord(ctx, req.OrganizationID, req.SpaceID, req.ChatRoomID, req.SourceMessageID, req.AIUserID, req.RequestedByUserID)
	if err != nil {
		return err
	}
	if !inserted {
		w.logSkipMention(req, "dedupe_duplicate_or_already_queued")
		return nil
	}
	src, err := w.Store.GetChatMessageByID(ctx, req.SpaceID, req.ChatRoomID, req.SourceMessageID)
	if err == pgx.ErrNoRows {
		w.logSkipMention(req, "source_message_not_found")
		return nil
	}
	if err != nil {
		return err
	}
	if src.DeletedAt != nil || src.AuthorID == nil {
		w.logSkipMention(req, "source_deleted_or_no_author")
		return nil
	}
	if *src.AuthorID == req.AIUserID {
		w.logSkipMention(req, "source_author_is_target_ai")
		return nil
	}
	sourceIsSA, err := w.Store.ServiceAccountInOrg(ctx, req.OrganizationID, *src.AuthorID)
	if err != nil {
		return err
	}
	if sourceIsSA {
		w.logSkipMention(req, "source_author_is_service_account")
		return nil
	}
	isAIStaff, err := w.Store.ServiceAccountInOrg(ctx, req.OrganizationID, req.AIUserID)
	if err != nil {
		return err
	}
	if !isAIStaff {
		w.logSkipMention(req, "target_user_not_ai_staff")
		return nil
	}
	askerOK, err := w.Store.UserCanAccessSpace(ctx, req.OrganizationID, req.SpaceID, req.RequestedByUserID)
	if err != nil {
		return err
	}
	if !askerOK {
		w.logSkipMention(req, "requester_no_space_access")
		return nil
	}

	history, err := w.Store.ListChatMessages(ctx, req.ChatRoomID, maxChatHistory, nil)
	if err != nil {
		return err
	}

	sa, err := w.Store.GetServiceAccountByUserInOrg(ctx, req.OrganizationID, req.AIUserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return w.failWithAssistantText(ctx, req, "This AI staff member is not configured for this organization.")
		}
		return err
	}

	slog.Info("chat ai mention processing",
		"source_message_id", req.SourceMessageID,
		"ai_user_id", req.AIUserID,
		"provider", sa.Provider,
		"service_account_id", sa.ID,
	)

	switch sa.Provider {
	case store.ProviderCursor:
		return w.handleCursorAgent(ctx, req, src, history, sa)
	default:
		return w.handleOpenRouter(ctx, req, src, history, sa)
	}
}

func openRouterStaffChatOpts(t OpenRouterChatTooling) *openrouter.ChatCompletionOpts {
	hasReasoning := strings.TrimSpace(string(t.Reasoning)) != ""
	if !hasReasoning && t.MaxTokens == nil {
		return nil
	}
	var r json.RawMessage
	if hasReasoning {
		r = t.Reasoning
	}
	return &openrouter.ChatCompletionOpts{Reasoning: r, MaxTokens: t.MaxTokens}
}

func (w *Worker) failWithAssistantText(ctx context.Context, req events.ChatAIMentionRequestedPayload, reply string) error {
	msg, err := w.Store.CreateChatMessage(ctx, req.SpaceID, req.ChatRoomID, req.AIUserID, reply)
	if err != nil {
		return err
	}
	if err := w.Store.MarkChatAIMentionReplyResponded(ctx, req.SourceMessageID, req.AIUserID, msg.ID, nil); err != nil {
		return err
	}
	return w.publishChatMessage(ctx, req, msg)
}

func (w *Worker) publishChatMessage(ctx context.Context, req events.ChatAIMentionRequestedPayload, msg store.ChatMessage) error {
	out, err := events.Marshal(events.ChatMessageCreated, req.OrganizationID, &req.SpaceID, map[string]any{
		"chat_room_id": req.ChatRoomID,
		"payload": map[string]any{
			"message":     msg,
			"attachments": []store.ChatMessageAttachment{},
		},
	})
	if err != nil {
		return err
	}
	if err := w.Bus.Publish(ctx, req.OrganizationID, out); err != nil {
		return err
	}
	authorID := req.AIUserID
	if msg.AuthorID != nil {
		authorID = *msg.AuthorID
	}
	if err := chatmentions.NotifyMentionRecipients(ctx, w.Store, w.Bus, req.OrganizationID, req.SpaceID, req.ChatRoomID, msg.ID, authorID, msg.Content); err != nil {
		slog.Warn("chat mention notifications after ai message", "err", err)
	}
	return nil
}

func (w *Worker) handleOpenRouter(ctx context.Context, req events.ChatAIMentionRequestedPayload, src store.ChatMessage, history []store.ChatMessage, sa store.ServiceAccount) error {
	key32, err := decodeSecretsKey(w.EncryptKeyB64)
	if err != nil || len(key32) != 32 {
		return w.failWithAssistantText(ctx, req, encryptionKeyMissingReply())
	}
	apiKey, err := w.Store.DecryptedOrgOpenRouterAPIKey(ctx, req.OrganizationID, key32)
	if err != nil {
		slog.Error("decrypt org openrouter key", "err", err)
		return w.failWithAssistantText(ctx, req, "I couldn’t load the workspace OpenRouter API key (decrypt error). An org admin should re-save the key in settings or clear it and try again.")
	}
	if strings.TrimSpace(apiKey) == "" {
		return w.failWithAssistantText(ctx, req, "OpenRouter-backed replies are not enabled for this workspace yet. An org admin must add an OpenRouter API key under **Workspace settings** (OpenRouter integration).")
	}
	if w.OpenRouter == nil {
		return w.failWithAssistantText(ctx, req, "The OpenRouter client is not configured on the server. Contact your Hyperspeed operator.")
	}
	model := ""
	if sa.OpenRouterModel != nil {
		model = strings.TrimSpace(*sa.OpenRouterModel)
	}
	if model == "" {
		return w.failWithAssistantText(ctx, req, "This AI staff member has no **openrouter_model** configured. An org admin should set it in service account settings.")
	}
	if w.Debug {
		slog.Debug("chatai openrouter using model from service_accounts row",
			"model", model,
			"org_id", req.OrganizationID,
			"ai_user_id", req.AIUserID,
			"service_account_id", sa.ID,
			"service_account_name", sa.Name,
		)
	}

	useTools := w.ORTooling.Enabled && w.Harness != nil && w.Harness.Store != nil && w.Harness.OS != nil
	var fileProposalMeta []map[string]any
	seenProposalIDs := make(map[uuid.UUID]struct{})
	var msgs []cursor.Message
	if useTools {
		msgs, err = w.BuildStaffMessagesWithOptions(ctx, req, src, history, StaffMessageOptions{
			ToolsEnabled:                 true,
			SkipPreread:                  w.ORTooling.SkipPreread,
			IncludeStaffProfileAndMemory: true,
		})
	} else {
		msgs, err = w.BuildStaffMessagesWithOptions(ctx, req, src, history, StaffMessageOptions{
			IncludeStaffProfileAndMemory: true,
		})
	}
	if err != nil {
		slog.Error("build staff context", "err", err)
		if he, ok := agenttools.IsHarnessError(err); ok {
			return w.failWithAssistantText(ctx, req, fmt.Sprintf("I couldn’t read space context (%s). Try again or narrow the request.", he.Message))
		}
		return w.failWithAssistantText(ctx, req, "I couldn’t build context for this reply. Please try again.")
	}

	var runDetailForMark []byte
	var text string
	var tools []json.RawMessage
	if useTools {
		var terr error
		tools, terr = w.ORTooling.BuildRequestTools()
		if terr != nil {
			slog.Error("openrouter tools payload", "err", terr)
			return w.failWithAssistantText(ctx, req, "Tool configuration failed on the server. Ask an operator to check logs.")
		}
		if len(tools) == 0 {
			useTools = false
		}
	}
	if useTools {
		seed := openrouter.ChatMessagesFromCursor(msgs)
		sessionID := "or-staff-" + req.SourceMessageID.String()
		allow := make(map[string]struct{}, 8)
		for _, n := range agenttools.OpenRouterInvokableToolNames() {
			allow[n] = struct{}{}
		}
		trace := &openrouter.ToolLoopTrace{}
		opts := openrouter.ToolLoopOptions{
			MaxIterations: w.ORTooling.MaxIterations,
			StepTimeout:   w.ORTooling.StepTimeout,
			Reasoning:     w.ORTooling.Reasoning,
			MaxTokens:     w.ORTooling.MaxTokens,
			Trace:         trace,
		}
		exec := func(callCtx context.Context, name string, args json.RawMessage) (string, error) {
			if _, ok := allow[name]; !ok {
				b, _ := json.Marshal(map[string]string{"error": "unknown tool: " + name})
				return string(b), nil
			}
			sid := sessionID
			start := time.Now()
			result, ierr := w.Harness.Invoke(callCtx, req.OrganizationID, req.AIUserID, agenttools.InvokeInput{
				Tool: name, Arguments: args, Mode: "agent", SessionID: &sid,
			})
			w.Harness.LogInvocation(callCtx, req.OrganizationID, req.AIUserID, &sid, name, args, result, ierr, start)
			if ierr != nil {
				if he, ok := agenttools.IsHarnessError(ierr); ok {
					b, _ := json.Marshal(map[string]string{"error": he.Message, "code": he.Code})
					return string(b), nil
				}
				return "", ierr
			}
			b, mErr := json.Marshal(result)
			if mErr != nil {
				return "", mErr
			}
			if name == "space.file.propose_patch" {
				var parsed struct {
					Proposal *struct {
						ID     uuid.UUID `json:"id"`
						NodeID uuid.UUID `json:"node_id"`
					} `json:"proposal"`
				}
				if json.Unmarshal(b, &parsed) == nil && parsed.Proposal != nil && parsed.Proposal.ID != uuid.Nil {
					if _, dup := seenProposalIDs[parsed.Proposal.ID]; !dup {
						seenProposalIDs[parsed.Proposal.ID] = struct{}{}
						fn := ""
						if n, ferr := w.Store.FileNodeByID(callCtx, req.SpaceID, parsed.Proposal.NodeID); ferr == nil {
							fn = n.Name
						}
						fileProposalMeta = append(fileProposalMeta, map[string]any{
							"proposal_id": parsed.Proposal.ID.String(),
							"node_id":     parsed.Proposal.NodeID.String(),
							"file_name":   fn,
						})
					}
				}
			}
			return string(b), nil
		}
		var usage json.RawMessage
		var fallbackNote string
		text, usage, err = w.OpenRouter.ChatCompletionWithToolLoop(ctx, apiKey, model, seed, tools, w.ORTooling.PluginsRaw, opts, exec)
		if w.Debug && len(usage) > 0 {
			slog.Debug("openrouter staff usage", "usage", string(usage))
		}
		if err != nil {
			slog.Warn("openrouter tool loop failed, falling back to plain completion", "err", err)
			fallbackNote = err.Error()
			text, err = w.OpenRouter.ChatCompletion(ctx, apiKey, model, msgs, openRouterStaffChatOpts(w.ORTooling))
		}
		if err == nil {
			if rd := buildOpenRouterPeekDetail(model, usage, trace, fileProposalMeta, fallbackNote); len(rd) > 0 {
				runDetailForMark = rd
			}
		}
	} else {
		text, err = w.OpenRouter.ChatCompletion(ctx, apiKey, model, msgs, openRouterStaffChatOpts(w.ORTooling))
	}
	if err != nil {
		return w.failWithAssistantText(ctx, req, formatOpenRouterFailure(err))
	}
	if strings.TrimSpace(text) == "" {
		return w.failWithAssistantText(ctx, req, "The AI returned an empty reply. Please try again.")
	}
	text = w.withProfileFooter(ctx, req.OrganizationID, req.AIUserID, text)

	var msg store.ChatMessage
	if len(fileProposalMeta) > 0 {
		metaBytes, jerr := json.Marshal(map[string]any{"file_edit_proposals": fileProposalMeta})
		if jerr != nil {
			slog.Error("marshal chat file proposal metadata", "err", jerr)
			msg, err = w.Store.CreateChatMessage(ctx, req.SpaceID, req.ChatRoomID, req.AIUserID, text)
		} else {
			msg, err = w.Store.CreateChatMessageWithMetadata(ctx, req.SpaceID, req.ChatRoomID, req.AIUserID, text, metaBytes)
		}
	} else {
		msg, err = w.Store.CreateChatMessage(ctx, req.SpaceID, req.ChatRoomID, req.AIUserID, text)
	}
	if err != nil {
		return err
	}
	if err := w.Store.MarkChatAIMentionReplyResponded(ctx, req.SourceMessageID, req.AIUserID, msg.ID, runDetailForMark); err != nil {
		return err
	}
	w.spawnOpenRouterMemoryPersist(req, sa, model, apiKey, src, msg)
	return w.publishChatMessage(ctx, req, msg)
}

func (w *Worker) handleCursorAgent(ctx context.Context, req events.ChatAIMentionRequestedPayload, src store.ChatMessage, history []store.ChatMessage, sa store.ServiceAccount) error {
	key32, err := decodeSecretsKey(w.EncryptKeyB64)
	if err != nil || len(key32) != 32 {
		return w.failWithAssistantText(ctx, req, encryptionKeyMissingReply())
	}
	apiKey, err := w.Store.DecryptedOrgCursorAPIKey(ctx, req.OrganizationID, key32)
	if err != nil {
		slog.Error("decrypt org cursor key", "err", err)
		return w.failWithAssistantText(ctx, req, "I couldn’t load the workspace Cursor API key (decrypt error). An org admin should re-save the key in settings or clear it and try again.")
	}
	if strings.TrimSpace(apiKey) == "" {
		return w.failWithAssistantText(ctx, req, "Cursor Cloud Agent runs require a Cursor API key. An org admin must add one under **Workspace settings** (Cursor integration).")
	}
	repoURL, ref, err := ResolveCursorRepoForLaunch(ctx, w.Store, req.SpaceID, sa)
	if err != nil {
		if errors.Is(err, ErrNoCursorRepoForLaunch) {
			return w.failWithAssistantText(ctx, req, "No **repository URL** for this run: set `cursor_default_repo_url` on the Cursor staff profile, or configure a Git remote for this space in the IDE Source Control panel.")
		}
		slog.Error("resolve cursor repo for launch", "err", err)
		return w.failWithAssistantText(ctx, req, "Couldn’t load repository settings for Cursor Cloud Agent. Try again.")
	}
	if w.Agents == nil {
		return w.failWithAssistantText(ctx, req, "The Cursor Cloud Agents client is not configured on the server. Contact your Hyperspeed operator.")
	}

	msgs, err := w.BuildStaffMessages(ctx, req, src, history)
	if err != nil {
		slog.Error("build staff context", "err", err)
		if he, ok := agenttools.IsHarnessError(err); ok {
			return w.failWithAssistantText(ctx, req, fmt.Sprintf("I couldn’t read space context (%s). Try again or narrow the request.", he.Message))
		}
		return w.failWithAssistantText(ctx, req, "I couldn’t build context for this reply. Please try again.")
	}
	prompt := messagesToAgentPrompt(msgs)

	launch, err := w.Agents.Launch(ctx, apiKey, agents.LaunchInput{
		Prompt:        prompt,
		RepositoryURL: repoURL,
		Ref:           ref,
	})
	if err != nil {
		return w.failWithAssistantText(ctx, req, formatCursorAgentsFailure(err))
	}
	extID := strings.TrimSpace(launch.ID)
	if extID == "" {
		return w.failWithAssistantText(ctx, req, "The Cursor API returned an agent response without an id. Try again or check the integration.")
	}

	meta, _ := json.Marshal(map[string]any{
		"ai_agent_run": map[string]any{
			"provider":     "cursor",
			"external_id":  extID,
			"status":       strings.TrimSpace(launch.Status),
			"url":          strings.TrimSpace(launch.URL),
			"display_name": "Cloud Agent run",
		},
	})
	startContent := "Starting **Cursor Cloud Agent** run…"
	if strings.TrimSpace(launch.URL) != "" {
		startContent += "\n\n" + launch.URL
	}
	startMsg, err := w.Store.CreateChatMessageWithMetadata(ctx, req.SpaceID, req.ChatRoomID, req.AIUserID, startContent, meta)
	if err != nil {
		return err
	}
	if err := w.publishChatMessage(ctx, req, startMsg); err != nil {
		return err
	}

	final, err := w.Agents.PollUntilTerminal(ctx, apiKey, extID, 4*time.Second)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return w.failWithAssistantText(ctx, req, "The Cloud Agent run timed out before completion. Try a smaller request or try again later.")
		}
		return w.failWithAssistantText(ctx, req, formatCursorAgentsFailure(err))
	}

	conv, err := w.Agents.GetConversation(ctx, apiKey, extID)
	if err != nil {
		slog.Error("cursor agent conversation", "err", err)
	}
	var cursorRunDetail []byte
	if rd := buildCursorPeekDetail(extID, strings.TrimSpace(launch.URL), final, conv); len(rd) > 0 {
		cursorRunDetail = rd
	}
	summary := agents.SummarizeConversation(conv)
	if strings.TrimSpace(summary) == "" {
		st := strings.TrimSpace(final.Status)
		if st == "" {
			st = "completed"
		}
		summary = "Cloud Agent finished with status **" + st + "**."
		if u := final.EffectiveURL(); u != "" {
			summary += "\n\n" + u
		}
	}
	summary = w.withProfileFooter(ctx, req.OrganizationID, req.AIUserID, summary)

	finalMsg, err := w.Store.CreateChatMessage(ctx, req.SpaceID, req.ChatRoomID, req.AIUserID, summary)
	if err != nil {
		return err
	}
	if err := w.Store.MarkChatAIMentionReplyResponded(ctx, req.SourceMessageID, req.AIUserID, finalMsg.ID, cursorRunDetail); err != nil {
		return err
	}
	return w.publishChatMessage(ctx, req, finalMsg)
}

func messagesToAgentPrompt(msgs []cursor.Message) string {
	var b strings.Builder
	for _, m := range msgs {
		role := strings.TrimSpace(m.Role)
		if role == "" {
			role = "user"
		}
		b.WriteString(strings.ToUpper(role))
		b.WriteString("\n")
		b.WriteString(strings.TrimSpace(m.Content))
		b.WriteString("\n\n")
	}
	return strings.TrimSpace(b.String())
}

func encryptionKeyMissingReply() string {
	return "I can’t reach the AI service: the server encryption key (HS_SSH_ENCRYPTION_KEY) is not configured. Ask an org admin to set it, then configure org API keys in workspace settings."
}

func (w *Worker) withProfileFooter(ctx context.Context, orgID, aiUserID uuid.UUID, text string) string {
	var profileLine string
	if ident, err := w.Store.ServiceAccountIdentityByUser(ctx, aiUserID); err == nil && ident != nil && ident.OrganizationID == orgID {
		if prof, err := w.Store.LatestServiceAccountProfile(ctx, ident.ServiceAccountID); err == nil {
			profileLine = strings.TrimSpace(firstNonEmptyLine(prof.ContentMD))
		}
	}
	if profileLine != "" {
		return text + "\n\n—\n*" + profileLine + "*"
	}
	return text
}

func formatOpenRouterFailure(err error) string {
	switch {
	case errors.Is(err, cursor.ErrAuth):
		return "OpenRouter rejected this workspace’s API key (auth error). An org admin should verify or rotate the key under workspace settings."
	case errors.Is(err, cursor.ErrRateLimit):
		return "OpenRouter rate-limited this workspace. Wait a bit and try your mention again."
	case errors.Is(err, cursor.ErrTimeout):
		return "OpenRouter timed out. Try again with a shorter question or try later."
	default:
		var up *cursor.ErrUpstream
		if errors.As(err, &up) && up.Msg != "" {
			return "OpenRouter returned an error: " + strings.TrimSpace(up.Msg)
		}
		return "Something went wrong calling OpenRouter. Try again later or ask an org admin to check the integration."
	}
}

func formatCursorAgentsFailure(err error) string {
	switch {
	case errors.Is(err, cursor.ErrAuth):
		return "The Cursor API rejected this workspace’s API key (auth error). An org admin should verify or rotate the key under workspace settings."
	case errors.Is(err, cursor.ErrRateLimit):
		return "The Cursor API rate-limited this workspace. Wait a bit and try your mention again."
	case errors.Is(err, cursor.ErrTimeout):
		return "The Cursor API timed out. Try again later."
	default:
		var up *cursor.ErrUpstream
		if errors.As(err, &up) && up.Msg != "" {
			return "The Cursor Cloud Agents API returned an error: " + strings.TrimSpace(up.Msg)
		}
		return "Something went wrong calling the Cursor Cloud Agents API. Try again later or ask an org admin to check the integration."
	}
}

func buildOpenRouterPeekDetail(model string, usage json.RawMessage, trace *openrouter.ToolLoopTrace, fileProposalMeta []map[string]any, fallbackNote string) []byte {
	hasTrace := trace != nil && len(trace.Steps) > 0
	if !hasTrace && fallbackNote == "" && len(fileProposalMeta) == 0 {
		return nil
	}
	m := map[string]any{"provider": "openrouter", "model": model}
	if len(usage) > 0 {
		m["usage"] = json.RawMessage(usage)
	}
	if hasTrace {
		m["trace"] = trace
	}
	if len(fileProposalMeta) > 0 {
		m["file_edit_proposals"] = fileProposalMeta
	}
	if fallbackNote != "" {
		m["fallback_note"] = fallbackNote
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil
	}
	return b
}

func buildCursorPeekDetail(externalID, launchURL string, final agents.AgentStatusResponse, conv []agents.ConversationMessage) []byte {
	m := map[string]any{
		"provider":     "cursor",
		"external_id":  externalID,
		"final_status": strings.TrimSpace(final.Status),
	}
	if launchURL != "" {
		m["launch_url"] = launchURL
	}
	if u := final.EffectiveURL(); u != "" {
		m["result_url"] = u
	}
	if len(conv) > 0 {
		const maxMsg = 8000
		msgs := make([]map[string]any, 0, len(conv))
		for _, row := range conv {
			c := strings.TrimSpace(row.Content)
			trunc := false
			if len(c) > maxMsg {
				c = c[:maxMsg] + "…"
				trunc = true
			}
			entry := map[string]any{"role": row.Role, "content": c}
			if trunc {
				entry["content_truncated"] = true
			}
			msgs = append(msgs, entry)
		}
		m["conversation"] = msgs
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil
	}
	return b
}

func firstNonEmptyLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		t := strings.TrimSpace(strings.TrimPrefix(line, "#"))
		if t != "" {
			return t
		}
	}
	return ""
}
