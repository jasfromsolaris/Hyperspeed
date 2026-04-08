package provisioning

import (
	"strings"
	"sync"
)

// Runtime holds the active provisioning gateway credentials for this process.
// It is updated after startup bootstrap and after POST /api/v1/provisioning/apply-bootstrap-token
// so handlers see provisioning_enabled without restarting the container.
type Runtime struct {
	mu     sync.RWMutex
	base   string
	id     string
	secret string
}

func NewRuntime() *Runtime {
	return &Runtime{}
}

// Set replaces gateway URL and install credentials (trimmed).
func (r *Runtime) Set(baseURL, installID, installSecret string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.base = strings.TrimSpace(baseURL)
	r.id = strings.TrimSpace(installID)
	r.secret = strings.TrimSpace(installSecret)
}

// Snapshot returns the current credentials for gateway calls.
func (r *Runtime) Snapshot() (baseURL, installID, installSecret string) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.base, r.id, r.secret
}

// Configured reports whether all three values are non-empty.
func (r *Runtime) Configured() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return strings.TrimSpace(r.base) != "" &&
		strings.TrimSpace(r.id) != "" &&
		strings.TrimSpace(r.secret) != ""
}
