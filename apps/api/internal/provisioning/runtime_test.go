package provisioning

import "testing"

func TestRuntime_SetConfigured(t *testing.T) {
	r := NewRuntime()
	if r.Configured() {
		t.Fatal("expected not configured")
	}
	r.Set("https://gw.example.com", "id1", "sec1")
	if !r.Configured() {
		t.Fatal("expected configured")
	}
	b, i, s := r.Snapshot()
	if b != "https://gw.example.com" || i != "id1" || s != "sec1" {
		t.Fatalf("snapshot: %q %q %q", b, i, s)
	}
	r.Set("", "", "")
	if r.Configured() {
		t.Fatal("expected not configured after clear")
	}
}
