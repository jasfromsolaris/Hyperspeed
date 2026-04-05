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
}

func Load() Config {
	return Config{
		HTTPAddr:           strings.TrimSpace(getEnv("HTTP_ADDR", ":8787")),
		BearerToken:        strings.TrimSpace(os.Getenv("CONTROL_PLANE_BEARER_TOKEN")),
		CloudflareAPIToken: strings.TrimSpace(os.Getenv("CLOUDFLARE_API_TOKEN")),
		CloudflareZoneID:   strings.TrimSpace(os.Getenv("CLOUDFLARE_ZONE_ID")),
		BaseDomain:         strings.TrimSpace(getEnv("BASE_DOMAIN", "hyperspeedapp.com")),
		AuditDBPath:        strings.TrimSpace(getEnv("AUDIT_DB_PATH", "./data/control-plane.sqlite")),
		Proxied:            strings.EqualFold(strings.TrimSpace(os.Getenv("CLOUDFLARE_PROXIED")), "true"),
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
