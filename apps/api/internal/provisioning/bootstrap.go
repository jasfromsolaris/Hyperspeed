package provisioning

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"hyperspeed/api/internal/config"
)

// bootstrapResponse matches the provisioning gateway POST /v1/bootstrap JSON body.
type bootstrapResponse struct {
	ProvisioningBaseURL       string `json:"provisioning_base_url"`
	ProvisioningInstallID     string `json:"provisioning_install_id"`
	ProvisioningInstallSecret string `json:"provisioning_install_secret"`
}

func resolveBootstrapGatewayURL() string {
	gw := strings.TrimSpace(config.DefaultProvisioningGatewayURL)
	if v := strings.TrimSpace(os.Getenv("PROVISIONING_BOOTSTRAP_GATEWAY_URL")); v != "" {
		gw = strings.TrimSuffix(v, "/")
	}
	return gw
}

// PerformBootstrapExchange calls POST /v1/bootstrap on the gateway, writes statePath atomically,
// and returns install-scoped credentials. Used at startup (env token) and from ApplyBootstrapToken (UI).
func PerformBootstrapExchange(ctx context.Context, token string, statePath string) (base, installID, installSecret string, err error) {
	tok := strings.TrimSpace(token)
	if tok == "" {
		return "", "", "", fmt.Errorf("bootstrap token is empty")
	}
	gw := resolveBootstrapGatewayURL()
	url := strings.TrimSuffix(gw, "/") + "/v1/bootstrap"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader([]byte("{}")))
	if err != nil {
		return "", "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", "", fmt.Errorf("bootstrap request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", "", "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", "", fmt.Errorf("bootstrap failed: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var out bootstrapResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return "", "", "", fmt.Errorf("bootstrap response json: %w", err)
	}
	out.ProvisioningBaseURL = strings.TrimSpace(out.ProvisioningBaseURL)
	out.ProvisioningInstallID = strings.TrimSpace(out.ProvisioningInstallID)
	out.ProvisioningInstallSecret = strings.TrimSpace(out.ProvisioningInstallSecret)
	if out.ProvisioningBaseURL == "" || out.ProvisioningInstallID == "" || out.ProvisioningInstallSecret == "" {
		return "", "", "", fmt.Errorf("bootstrap response missing provisioning fields")
	}

	statePath = strings.TrimSpace(statePath)
	if statePath == "" {
		statePath = config.DefaultProvisioningStatePath
	}
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		return "", "", "", fmt.Errorf("provisioning state dir: %w", err)
	}
	payload, err := json.MarshalIndent(struct {
		ProvisioningBaseURL       string `json:"provisioning_base_url"`
		ProvisioningInstallID     string `json:"provisioning_install_id"`
		ProvisioningInstallSecret string `json:"provisioning_install_secret"`
	}{
		ProvisioningBaseURL:       out.ProvisioningBaseURL,
		ProvisioningInstallID:     out.ProvisioningInstallID,
		ProvisioningInstallSecret: out.ProvisioningInstallSecret,
	}, "", "  ")
	if err != nil {
		return "", "", "", err
	}
	tmp := statePath + ".tmp"
	if err := os.WriteFile(tmp, payload, 0o600); err != nil {
		return "", "", "", err
	}
	if err := os.Rename(tmp, statePath); err != nil {
		_ = os.Remove(tmp)
		return "", "", "", err
	}

	return out.ProvisioningBaseURL, out.ProvisioningInstallID, out.ProvisioningInstallSecret, nil
}

// ExchangeBootstrapIfNeeded exchanges PROVISIONING_BOOTSTRAP_TOKEN for install credentials and writes
// the state file when the API has no provisioning credentials yet.
func ExchangeBootstrapIfNeeded(ctx context.Context, cfg *config.Config) error {
	tok := strings.TrimSpace(cfg.ProvisioningBootstrapToken)
	if tok == "" {
		return nil
	}
	if cfg.ProvisioningBaseURL != "" && cfg.ProvisioningInstallID != "" && cfg.ProvisioningInstallSecret != "" {
		return nil
	}
	if cfg.ProvisioningInstallID != "" || cfg.ProvisioningInstallSecret != "" {
		return fmt.Errorf("PROVISIONING_BOOTSTRAP_TOKEN is set but PROVISIONING_INSTALL_ID/SECRET are partially set; unset the token or provide full install credentials")
	}

	statePath := strings.TrimSpace(cfg.ProvisioningStatePath)
	if statePath == "" {
		statePath = config.DefaultProvisioningStatePath
	}
	base, id, secret, err := PerformBootstrapExchange(ctx, tok, statePath)
	if err != nil {
		return err
	}
	cfg.ProvisioningBaseURL = base
	cfg.ProvisioningInstallID = id
	cfg.ProvisioningInstallSecret = secret
	return nil
}
