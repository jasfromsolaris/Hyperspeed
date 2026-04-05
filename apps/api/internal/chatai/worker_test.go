package chatai

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestFirstNonEmptyLine(t *testing.T) {
	got := firstNonEmptyLine("\n# Persona\n\nHelpful and concise\n")
	if got != "Persona" {
		t.Fatalf("firstNonEmptyLine got %q", got)
	}
}

func TestCleanUserMessageSnippet(t *testing.T) {
	u := uuid.New().String()
	raw := "<@" + u + "> @Jarvis do you see the file?"
	got := cleanUserMessageSnippet(raw)
	if !strings.Contains(got, "About your message:") || strings.Contains(got, u) {
		t.Fatalf("unexpected snippet: %q", got)
	}
	if !strings.Contains(got, "do you see the file") {
		t.Fatalf("expected preserved text: %q", got)
	}
}
