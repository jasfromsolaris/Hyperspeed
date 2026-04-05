package spacegit

import (
	"fmt"
	"net/url"
	"strings"
)

// AuthedHTTPSURL embeds a GitHub-compatible HTTPS token into the remote URL.
// Only https:// URLs are allowed.
func AuthedHTTPSURL(rawURL, token string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	if u.Scheme != "https" {
		return "", fmt.Errorf("only https:// Git remotes are supported")
	}
	tok := strings.TrimSpace(token)
	if tok == "" {
		return u.String(), nil
	}
	u.User = url.UserPassword("x-access-token", tok)
	return u.String(), nil
}
