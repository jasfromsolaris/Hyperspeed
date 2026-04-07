package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadProvisioningFromEnvFilesAndState_PrefersStateFile(t *testing.T) {
	dir := t.TempDir()
	state := filepath.Join(dir, "state.json")
	payload := `{"provisioning_base_url":"https://gw.test","provisioning_install_id":"a","provisioning_install_secret":"b"}`
	if err := os.WriteFile(state, []byte(payload), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PROVISIONING_STATE_PATH", state)
	t.Setenv("PROVISIONING_BASE_URL", "https://ignored")
	t.Setenv("PROVISIONING_INSTALL_ID", "ignored")
	t.Setenv("PROVISIONING_INSTALL_SECRET", "ignored")

	b, id, sec := loadProvisioningFromEnvFilesAndState()
	if b != "https://gw.test" || id != "a" || sec != "b" {
		t.Fatalf("got %q %q %q", b, id, sec)
	}
}

func TestLoadProvisioningFromEnvFilesAndState_DefaultBaseURL(t *testing.T) {
	t.Setenv("PROVISIONING_STATE_PATH", filepath.Join(t.TempDir(), "missing.json"))
	t.Setenv("PROVISIONING_BASE_URL", "")
	t.Setenv("PROVISIONING_INSTALL_ID", "x")
	t.Setenv("PROVISIONING_INSTALL_SECRET", "y")
	t.Setenv("PROVISIONING_INSTALL_ID_FILE", "")
	t.Setenv("PROVISIONING_INSTALL_SECRET_FILE", "")

	b, id, sec := loadProvisioningFromEnvFilesAndState()
	if b != DefaultProvisioningGatewayURL || id != "x" || sec != "y" {
		t.Fatalf("got %q %q %q", b, id, sec)
	}
}
