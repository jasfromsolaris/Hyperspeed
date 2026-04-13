package chatai

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"hyperspeed/api/internal/cursor"
	"hyperspeed/api/internal/events"
	"hyperspeed/api/internal/store"
)

type memoryExtractionResult struct {
	Episodes []struct {
		Summary    string  `json:"summary"`
		Details    string  `json:"details"`
		Importance float64 `json:"importance"`
	} `json:"episodes"`
	Facts []struct {
		Statement  string  `json:"statement"`
		Confidence float64 `json:"confidence"`
	} `json:"facts"`
	Procedures []struct {
		Name  string   `json:"name"`
		Steps []string `json:"steps"`
	} `json:"procedures"`
	ProposedAppendMD string `json:"proposed_append_md"`
}

func parseJSONPayload(s string) []byte {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	start := strings.IndexByte(s, '{')
	end := strings.LastIndexByte(s, '}')
	if start < 0 || end <= start {
		return nil
	}
	return []byte(s[start : end+1])
}

func (w *Worker) spawnOpenRouterMemoryPersist(req events.ChatAIMentionRequestedPayload, sa store.ServiceAccount, model, apiKey string, src store.ChatMessage, reply store.ChatMessage) {
	if w == nil || w.Store == nil || w.OpenRouter == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		claimed, err := w.Store.TryClaimStaffMemoryRun(ctx, req.OrganizationID, sa.ID, req.SourceMessageID, reply.ID)
		if err != nil {
			slog.Warn("staff memory claim run", "err", err)
			return
		}
		if !claimed {
			return
		}
		episode, err := w.Store.CreateStaffMemoryEpisode(ctx, store.CreateStaffMemoryEpisodeInput{
			OrganizationID:   req.OrganizationID,
			ServiceAccountID: sa.ID,
			SpaceID:          &req.SpaceID,
			ChatRoomID:       &req.ChatRoomID,
			SourceMessageID:  &req.SourceMessageID,
			ReplyMessageID:   &reply.ID,
			Summary:          trimToChars(stripChatMarkupTokens(reply.Content), 300),
			Details:          "User: " + trimToChars(stripChatMarkupTokens(src.Content), 700),
			Importance:       0.5,
		})
		if err != nil {
			slog.Warn("staff memory create episode", "err", err)
			return
		}
		w.writeWorkingMemory(ctx, req.OrganizationID, sa.ID, req.SourceMessageID, stripChatMarkupTokens(src.Content), stripChatMarkupTokens(reply.Content))

		sys := "Extract durable memory for an AI staff member. Return strict JSON only."
		user := "From this exchange, return JSON with keys episodes, facts, procedures, proposed_append_md.\n" +
			"episodes: [{summary, details, importance 0..1}] (0-2 rows)\n" +
			"facts: [{statement, confidence 0..1}] (0-6 rows)\n" +
			"procedures: [{name, steps:[]}] (0-3 rows)\n" +
			"proposed_append_md: optional short markdown block to append to staff profile only if very durable preference or instruction emerged.\n\n" +
			"User message:\n" + src.Content + "\n\nAssistant reply:\n" + reply.Content

		out, err := w.OpenRouter.ChatCompletion(ctx, apiKey, model, []cursor.Message{
			{Role: "system", Content: sys},
			{Role: "user", Content: user},
		}, nil)
		if err != nil {
			slog.Warn("staff memory extract completion", "err", err)
			return
		}
		payload := parseJSONPayload(out)
		if len(payload) == 0 {
			return
		}
		var parsed memoryExtractionResult
		if err := json.Unmarshal(payload, &parsed); err != nil {
			slog.Warn("staff memory extract parse", "err", err)
			return
		}

		for _, f := range parsed.Facts {
			if _, err := w.Store.UpsertStaffMemoryFact(ctx, req.OrganizationID, sa.ID, &episode.ID, &req.SourceMessageID, f.Statement, f.Confidence); err != nil {
				slog.Warn("staff memory upsert fact", "err", err)
			}
		}
		for _, p := range parsed.Procedures {
			steps, _ := json.Marshal(p.Steps)
			if _, err := w.Store.UpsertStaffMemoryProcedure(ctx, req.OrganizationID, sa.ID, &episode.ID, p.Name, steps); err != nil {
				slog.Warn("staff memory upsert procedure", "err", err)
			}
		}
		for _, e := range parsed.Episodes {
			if strings.TrimSpace(e.Summary) == "" {
				continue
			}
			if _, err := w.Store.CreateStaffMemoryEpisode(ctx, store.CreateStaffMemoryEpisodeInput{
				OrganizationID:   req.OrganizationID,
				ServiceAccountID: sa.ID,
				SpaceID:          &req.SpaceID,
				ChatRoomID:       &req.ChatRoomID,
				SourceMessageID:  &req.SourceMessageID,
				ReplyMessageID:   &reply.ID,
				Summary:          e.Summary,
				Details:          e.Details,
				Importance:       e.Importance,
			}); err != nil {
				slog.Warn("staff memory create extracted episode", "err", err)
			}
		}
		appendMD := strings.TrimSpace(parsed.ProposedAppendMD)
		if appendMD != "" {
			sourceID := req.SourceMessageID
			if _, err := w.Store.CreateServiceAccountProfileProposal(ctx, req.OrganizationID, sa.ID, &sourceID, nil, appendMD); err != nil {
				slog.Warn("staff profile proposal create", "err", err)
			}
		}
	}()
}

