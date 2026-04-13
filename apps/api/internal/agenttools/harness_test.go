package agenttools

import (
	"context"
	"testing"

	"hyperspeed/api/internal/files"
	"hyperspeed/api/internal/store"

	"github.com/google/uuid"
)

func TestNormalizeInvokeMode(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "ask", want: "ask"},
		{in: "plan", want: "plan"},
		{in: "agent", want: "agent"},
		{in: "", want: "agent"},
		{in: "unknown", want: "agent"},
	}
	for _, tc := range tests {
		got := NormalizeInvokeMode(tc.in)
		if got != tc.want {
			t.Fatalf("NormalizeInvokeMode(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestModeAllowsTool(t *testing.T) {
	if !modeAllowsTool("ask", "space.file.read") {
		t.Fatal("ask should allow read")
	}
	if modeAllowsTool("ask", "space.file.propose_patch") {
		t.Fatal("ask should block propose_patch")
	}
	if !modeAllowsTool("plan", "space.list_files") {
		t.Fatal("plan should allow list_files")
	}
	if modeAllowsTool("plan", "space.file.propose_patch") {
		t.Fatal("plan should block propose_patch")
	}
	if !modeAllowsTool("ask", "space.chat.read_recent") {
		t.Fatal("ask should allow space.chat.read_recent")
	}
	if !modeAllowsTool("agent", "space.file.propose_patch") {
		t.Fatal("agent should allow propose_patch")
	}
}

func TestInvokeRejectsToolByModeBeforeExecution(t *testing.T) {
	h := &Harness{
		Store: &store.Store{},
		OS:    &files.ObjectStore{},
	}
	_, err := h.Invoke(context.Background(), uuid.Nil, uuid.Nil, InvokeInput{
		Tool: "space.file.propose_patch",
		Mode: "ask",
	})
	if err == nil {
		t.Fatal("expected mode_policy error")
	}
	he, ok := IsHarnessError(err)
	if !ok {
		t.Fatalf("expected HarnessError, got %T", err)
	}
	if he.Code != "mode_policy" {
		t.Fatalf("expected mode_policy, got %q", he.Code)
	}
}

func TestInvokeBlocksCreateTextInAskMode(t *testing.T) {
	h := &Harness{
		Store: &store.Store{},
		OS:    &files.ObjectStore{},
	}
	_, err := h.Invoke(context.Background(), uuid.Nil, uuid.Nil, InvokeInput{
		Tool: "space.file.create_text",
		Mode: "ask",
	})
	if err == nil {
		t.Fatal("expected mode_policy error")
	}
	he, ok := IsHarnessError(err)
	if !ok || he.Code != "mode_policy" {
		t.Fatalf("expected mode_policy, got %v", err)
	}
}

func TestInvokeBlocksCreateFolderInAskMode(t *testing.T) {
	h := &Harness{
		Store: &store.Store{},
		OS:    &files.ObjectStore{},
	}
	_, err := h.Invoke(context.Background(), uuid.Nil, uuid.Nil, InvokeInput{
		Tool: "space.folder.create",
		Mode: "ask",
	})
	if err == nil {
		t.Fatal("expected mode_policy error")
	}
	he, ok := IsHarnessError(err)
	if !ok || he.Code != "mode_policy" {
		t.Fatalf("expected mode_policy, got %v", err)
	}
}
