package rest

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPublicBaseURL(t *testing.T) {
	t.Parallel()
	r := httptest.NewRequest(http.MethodGet, "http://localhost:8080/x", nil)
	if got := publicBaseURL(r, "https://api.example.com"); got != "https://api.example.com" {
		t.Fatalf("expected configured base, got %q", got)
	}
	r2 := httptest.NewRequest(http.MethodGet, "http://localhost:8080/x", nil)
	if got := publicBaseURL(r2, ""); got != "http://localhost:8080" {
		t.Fatalf("expected derived base, got %q", got)
	}
}
