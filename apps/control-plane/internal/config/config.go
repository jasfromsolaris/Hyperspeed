package config

import (
	"fmt"
	"os"
	"strings"
)

type Config struct {
	HTTPAddr            string
	BearerToken         string
	CloudflareAPIToken  string
	CloudflareZoneID    string
	BaseDomain          string // e.g. hyperspeedapp.com (apex zone)
	AuditDBPath         string
	Proxied             bool // Cloudflare orange cloud; default false for customer TLS
	WorkerAdminURL      string // e.g. https://provision-gw.hyperspeedapp.com
	WorkerAdminToken    string // token accepted by worker /v1/admin/bootstrap-token
	ProvisioningBaseURL string // returned to customer API for PROVISIONING_BASE_URL
}

func Load() Config {
	httpAddr := strings.TrimSpace(os.Getenv("HTTP_ADDR"))
	if httpAddr == "" {
		if p := strings.TrimSpace(os.Getenv("PORT")); p != "" {
			// Render, Fly, Railway, etc. inject PORT; bind on all interfaces.
			httpAddr = ":" + p
		} else {
			httpAddr = ":8787"
		}
	}
	return Config{
		HTTPAddr:           httpAddr,
		BearerToken:        strings.TrimSpace(os.Getenv("CONTROL_PLANE_BEARER_TOKEN")),
		CloudflareAPIToken: strings.TrimSpace(os.Getenv("CLOUDFLARE_API_TOKEN")),
		CloudflareZoneID:   strings.TrimSpace(os.Getenv("CLOUDFLARE_ZONE_ID")),
		BaseDomain:         strings.TrimSpace(getEnv("BASE_DOMAIN", "hyperspeedapp.com")),
		AuditDBPath:        strings.TrimSpace(getEnv("AUDIT_DB_PATH", "./data/control-plane.sqlite")),
		Proxied:            strings.EqualFold(strings.TrimSpace(os.Getenv("CLOUDFLARE_PROXIED")), "true"),
		WorkerAdminURL:      strings.TrimSpace(getEnv("WORKER_ADMIN_URL", "https://provision-gw.hyperspeedapp.com")),
		WorkerAdminToken:    strings.TrimSpace(os.Getenv("WORKER_ADMIN_TOKEN")),
		ProvisioningBaseURL: strings.TrimSpace(getEnv("PROVISIONING_BASE_URL", "https://provision-gw.hyperspeedapp.com")),
	}
}

func (c Config) Validate() error {
	var errs []string
	if c.BearerToken == "" {
		errs = append(errs, "CONTROL_PLANE_BEARER_TOKEN is required")
	}
	if c.CloudflareAPIToken == "" {
		errs = append(errs, "CLOUDFLARE_API_TOKEN is required")
	}
	if c.CloudflareZoneID == "" {
		errs = append(errs, "CLOUDFLARE_ZONE_ID is required")
	}
	if c.BaseDomain == "" {
		errs = append(errs, "BASE_DOMAIN is required")
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("invalid config:\n- %s", strings.Join(errs, "\n- "))
}

func getEnv(key, def string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	return v
}
