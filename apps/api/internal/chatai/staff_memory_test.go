package chatai

import "testing"

func TestTrimToChars(t *testing.T) {
	in := "hello world"
	if got := trimToChars(in, 5); got != "hello…" {
		t.Fatalf("unexpected trim result: %q", got)
	}
	if got := trimToChars(in, 50); got != in {
		t.Fatalf("expected unchanged string, got %q", got)
	}
}

func TestParseJSONPayload(t *testing.T) {
	raw := "```json\n{\"episodes\":[],\"facts\":[]}\n```"
	got := string(parseJSONPayload(raw))
	want := "{\"episodes\":[],\"facts\":[]}"
	if got != want {
		t.Fatalf("parseJSONPayload mismatch: got %q want %q", got, want)
	}
}

