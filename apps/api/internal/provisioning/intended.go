package provisioning

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// GiftedSubdomainApex is the only product hostname for gifted DNS (see docs).
const GiftedSubdomainApex = "hyperspeedapp.com"

var dnsLabel = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// GiftedTeamWWWPrefix is the hostname prefix for canonical gifted team URLs (www.{slug}.apex).
const GiftedTeamWWWPrefix = "www."

// GiftedSubdomainFromIntendedURL returns the team DNS label if intended is
// https://www.{label}.hyperspeedapp.com (canonical), or legacy https://{label}.hyperspeedapp.com.
// Path is ignored.
func GiftedSubdomainFromIntendedURL(intended string) (slug string, err error) {
	raw := strings.TrimSpace(intended)
	if raw == "" {
		return "", fmt.Errorf("empty intended url")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if strings.ToLower(u.Scheme) != "https" {
		return "", fmt.Errorf("intended_public_url must use https for gifted DNS")
	}
	host := strings.ToLower(u.Hostname())
	suffix := "." + GiftedSubdomainApex
	if !strings.HasSuffix(host, suffix) {
		return "", fmt.Errorf("intended host must be a subdomain of %s", GiftedSubdomainApex)
	}
	withoutApex := strings.TrimSuffix(host, suffix)
	if withoutApex == "" {
		return "", fmt.Errorf("invalid subdomain label")
	}
	if strings.HasPrefix(withoutApex, GiftedTeamWWWPrefix) {
		inner := strings.TrimPrefix(withoutApex, GiftedTeamWWWPrefix)
		if inner == "" || strings.Contains(inner, ".") {
			return "", fmt.Errorf("invalid subdomain label")
		}
		if !dnsLabel.MatchString(inner) {
			return "", fmt.Errorf("invalid subdomain label")
		}
		return inner, nil
	}
	if strings.Contains(withoutApex, ".") {
		return "", fmt.Errorf("invalid subdomain label")
	}
	if !dnsLabel.MatchString(withoutApex) {
		return "", fmt.Errorf("invalid subdomain label")
	}
	return withoutApex, nil
}
