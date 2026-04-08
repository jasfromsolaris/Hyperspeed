package provisioning

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"hyperspeed/api/internal/config"
)

func TestExchangeBootstrapIfNeeded_WritesState(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/bootstrap" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			http.Error(w, "auth", http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"provisioning_base_url":       "https://gw.example.com",
			"provisioning_install_id":     "inst1",
			"provisioning_install_secret": "sec1",
		})
	}))
	defer srv.Close()

	t.Setenv("PROVISIONING_BOOTSTRAP_GATEWAY_URL", srv.URL)
	t.Cleanup(func() { _ = os.Unsetenv("PROVISIONING_BOOTSTRAP_GATEWAY_URL") })

	cfg := config.Config{
		ProvisioningBootstrapToken: "test-token",
		ProvisioningStatePath:      statePath,
	}
	if err := ExchangeBootstrapIfNeeded(context.Background(), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.ProvisioningBaseURL != "https://gw.example.com" || cfg.ProvisioningInstallID != "inst1" || cfg.ProvisioningInstallSecret != "sec1" {
		t.Fatalf("cfg not updated: %+v", cfg)
	}
	b, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	var st struct {
		ProvisioningBaseURL       string `json:"provisioning_base_url"`
		ProvisioningInstallID     string `json:"provisioning_install_id"`
		ProvisioningInstallSecret string `json:"provisioning_install_secret"`
	}
	if err := json.Unmarshal(b, &st); err != nil {
		t.Fatal(err)
	}
	if st.ProvisioningInstallID != "inst1" {
		t.Fatalf("state file: %+v", st)
	}
}

func TestPerformBootstrapExchange_WritesState(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/bootstrap" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		auth := r.Header.Get("Authorization")
		if auth != "Bearer tok2" {
			http.Error(w, "auth", http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"provisioning_base_url":       "https://gw2.example.com",
			"provisioning_install_id":     "inst2",
			"provisioning_install_secret": "sec2",
		})
	}))
	defer srv.Close()

	t.Setenv("PROVISIONING_BOOTSTRAP_GATEWAY_URL", srv.URL)
	t.Cleanup(func() { _ = os.Unsetenv("PROVISIONING_BOOTSTRAP_GATEWAY_URL") })

	base, id, sec, err := PerformBootstrapExchange(context.Background(), "tok2", statePath)
	if err != nil {
		t.Fatal(err)
	}
	if base != "https://gw2.example.com" || id != "inst2" || sec != "sec2" {
		t.Fatalf("returns: %s %s %s", base, id, sec)
	}
}

func TestExchangeBootstrapIfNeeded_NoOpWhenProvisioned(t *testing.T) {
	cfg := config.Config{
		ProvisioningBootstrapToken: "x",
		ProvisioningBaseURL:        "https://a",
		ProvisioningInstallID:      "i",
		ProvisioningInstallSecret:  "s",
	}
	if err := ExchangeBootstrapIfNeeded(context.Background(), &cfg); err != nil {
		t.Fatal(err)
	}
}
