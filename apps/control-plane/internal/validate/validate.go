package validate

import (
	"net"
	"regexp"
)

var dnsLabel = regexp.MustCompile(`^[a-z]([a-z0-9-]{0,61}[a-z0-9])?$`)

var reservedSlugs = map[string]struct{}{
	"www": {}, "api": {}, "mail": {}, "ftp": {}, "smtp": {}, "pop": {}, "imap": {},
	"ns1": {}, "ns2": {}, "mx": {}, "webmail": {}, "admin": {}, "app": {},
}

// Slug checks a single DNS label (subdomain part only).
func Slug(s string) bool {
	if len(s) < 1 || len(s) > 63 {
		return false
	}
	if _, bad := reservedSlugs[s]; bad {
		return false
	}
	return dnsLabel.MatchString(s)
}

// IPv4 parses and validates a public IPv4 (rejects loopback for safety in production if desired — here allow any IPv4).
func IPv4(s string) bool {
	ip := net.ParseIP(s)
	if ip == nil || ip.To4() == nil {
		return false
	}
	return true
}
