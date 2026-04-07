package config

import (
	"os"
	"strings"
)

// DefaultProvisioningGatewayURL is the production provisioning gateway origin (no trailing path).
const DefaultProvisioningGatewayURL = "https://provision-gw.hyperspeedapp.com"

const (
	defaultProvisioningInstallSecretPath = "/run/secrets/provisioning_install_secret"
	defaultProvisioningInstallIDPath     = "/run/secrets/provisioning_install_id"
)

// DefaultProvisioningStatePath is the default path for persisted provisioning credentials (Phase 2 bootstrap).
const DefaultProvisioningStatePath = "/var/lib/hyperspeed/provisioning/state.json"

// readSecretFile reads a file and trims whitespace (Docker secret files often end with \n).
func readSecretFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

// getEnvOrFile returns the env value if set; otherwise reads from filePathEnv (e.g. PROVISIONING_INSTALL_SECRET_FILE);
// if that is unset or the file is missing, tries defaultPath when non-empty (e.g. Docker /run/secrets/...).
func getEnvOrFile(envKey, filePathEnvKey, defaultPath string) string {
	if v := strings.TrimSpace(os.Getenv(envKey)); v != "" {
		return v
	}
	if fp := strings.TrimSpace(os.Getenv(filePathEnvKey)); fp != "" {
		if s, err := readSecretFile(fp); err == nil && s != "" {
			return s
		}
	}
	if defaultPath != "" {
		if s, err := readSecretFile(defaultPath); err == nil && s != "" {
			return s
		}
	}
	return ""
}
