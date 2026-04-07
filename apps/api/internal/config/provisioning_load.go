package config

import (
	"encoding/json"
	"os"
	"strings"
)

// provisioningStateFile is persisted by bootstrap exchange (JSON on disk).
type provisioningStateFile struct {
	ProvisioningBaseURL       string `json:"provisioning_base_url"`
	ProvisioningInstallID     string `json:"provisioning_install_id"`
	ProvisioningInstallSecret string `json:"provisioning_install_secret"`
}

func tryLoadProvisioningStateFile(path string) (provisioningStateFile, bool) {
	path = strings.TrimSpace(path)
	if path == "" {
		return provisioningStateFile{}, false
	}
	b, err := os.ReadFile(path)
	if err != nil || len(b) == 0 {
		return provisioningStateFile{}, false
	}
	var st provisioningStateFile
	if err := json.Unmarshal(b, &st); err != nil {
		return provisioningStateFile{}, false
	}
	st.ProvisioningBaseURL = strings.TrimSpace(st.ProvisioningBaseURL)
	st.ProvisioningInstallID = strings.TrimSpace(st.ProvisioningInstallID)
	st.ProvisioningInstallSecret = strings.TrimSpace(st.ProvisioningInstallSecret)
	if st.ProvisioningBaseURL == "" || st.ProvisioningInstallID == "" || st.ProvisioningInstallSecret == "" {
		return provisioningStateFile{}, false
	}
	return st, true
}

func loadProvisioningFromEnvFilesAndState() (baseURL, installID, installSecret string) {
	statePath := strings.TrimSpace(getEnv("PROVISIONING_STATE_PATH", DefaultProvisioningStatePath))
	if st, ok := tryLoadProvisioningStateFile(statePath); ok {
		return st.ProvisioningBaseURL, st.ProvisioningInstallID, st.ProvisioningInstallSecret
	}
	baseURL = strings.TrimSpace(getEnv("PROVISIONING_BASE_URL", ""))
	installID = getEnvOrFile("PROVISIONING_INSTALL_ID", "PROVISIONING_INSTALL_ID_FILE", defaultProvisioningInstallIDPath)
	installSecret = getEnvOrFile("PROVISIONING_INSTALL_SECRET", "PROVISIONING_INSTALL_SECRET_FILE", defaultProvisioningInstallSecretPath)
	if baseURL == "" && installID != "" && installSecret != "" {
		baseURL = DefaultProvisioningGatewayURL
	}
	return baseURL, installID, installSecret
}
