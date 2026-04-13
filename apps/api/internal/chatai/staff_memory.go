package chatai

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	maxStaffProfileChars = 8000
	maxStaffMemoryChars  = 5000
	maxMemoryEpisodeRows = 6
	maxMemoryFactRows    = 8
	maxMemoryProcRows    = 3
)

type workingMemorySnapshot struct {
	SourceMessageID string `json:"source_message_id"`
	User            string `json:"user,omitempty"`
	Assistant       string `json:"assistant,omitempty"`
	CreatedAt       string `json:"created_at"`
}

func workingMemoryKey(orgID, serviceAccountID uuid.UUID) string {
	return "staffmem:wm:" + orgID.String() + ":" + serviceAccountID.String() + ":latest"
}

func trimToChars(s string, maxChars int) string {
	s = strings.TrimSpace(s)
	if maxChars <= 0 || len(s) <= maxChars {
		return s
	}
	return strings.TrimSpace(s[:maxChars]) + "…"
}

func (w *Worker) profileAndMemoryContext(ctx context.Context, orgID, aiUserID uuid.UUID, query string) (string, string) {
	if w == nil || w.Store == nil {
		return "", ""
	}
	ident, err := w.Store.ServiceAccountIdentityByUser(ctx, aiUserID)
	if err != nil || ident == nil || ident.OrganizationID != orgID {
		return "", ""
	}
	profileBlock := ""
	if prof, err := w.Store.LatestServiceAccountProfile(ctx, ident.ServiceAccountID); err == nil {
		profileBlock = trimToChars(prof.ContentMD, maxStaffProfileChars)
	}

	episodes, facts, procedures, err := w.Store.SearchStaffMemoryForPrompt(ctx, orgID, ident.ServiceAccountID, query, maxMemoryEpisodeRows, maxMemoryFactRows, maxMemoryProcRows)
	if err != nil {
		slog.Warn("staff memory retrieval", "err", err)
	}
	memory := strings.Builder{}
	if wm := w.readWorkingMemory(ctx, orgID, ident.ServiceAccountID); wm != "" {
		fmt.Fprintf(&memory, "Recent working memory: %s\n", wm)
	}
	for _, e := range episodes {
		if e.DeletedAt != nil {
			continue
		}
		fmt.Fprintf(&memory, "- Episode: %s", strings.TrimSpace(e.Summary))
		if d := strings.TrimSpace(e.Details); d != "" {
			fmt.Fprintf(&memory, " (%s)", trimToChars(d, 220))
		}
		memory.WriteString("\n")
	}
	for _, f := range facts {
		if f.InvalidatedAt != nil {
			continue
		}
		fmt.Fprintf(&memory, "- Fact: %s\n", strings.TrimSpace(f.Statement))
	}
	for _, p := range procedures {
		if strings.TrimSpace(p.Name) == "" {
			continue
		}
		fmt.Fprintf(&memory, "- Procedure: %s\n", strings.TrimSpace(p.Name))
	}
	memoryBlock := trimToChars(memory.String(), maxStaffMemoryChars)
	return profileBlock, memoryBlock
}

func (w *Worker) readWorkingMemory(ctx context.Context, orgID, serviceAccountID uuid.UUID) string {
	if w == nil || w.Rdb == nil {
		return ""
	}
	b, err := w.Rdb.Get(ctx, workingMemoryKey(orgID, serviceAccountID)).Bytes()
	if err != nil || len(b) == 0 {
		return ""
	}
	var snap workingMemorySnapshot
	if err := json.Unmarshal(b, &snap); err != nil {
		return ""
	}
	parts := make([]string, 0, 2)
	if u := strings.TrimSpace(snap.User); u != "" {
		parts = append(parts, "user="+trimToChars(u, 160))
	}
	if a := strings.TrimSpace(snap.Assistant); a != "" {
		parts = append(parts, "assistant="+trimToChars(a, 160))
	}
	return strings.Join(parts, "; ")
}

func (w *Worker) writeWorkingMemory(ctx context.Context, orgID, serviceAccountID, sourceMessageID uuid.UUID, userText, assistantText string) {
	if w == nil || w.Rdb == nil {
		return
	}
	snap := workingMemorySnapshot{
		SourceMessageID: sourceMessageID.String(),
		User:            trimToChars(userText, 500),
		Assistant:       trimToChars(assistantText, 500),
		CreatedAt:       time.Now().UTC().Format(time.RFC3339),
	}
	b, err := json.Marshal(snap)
	if err != nil {
		return
	}
	if err := w.Rdb.Set(ctx, workingMemoryKey(orgID, serviceAccountID), b, 15*time.Minute).Err(); err != nil {
		slog.Warn("write working memory", "err", err)
	}
}

