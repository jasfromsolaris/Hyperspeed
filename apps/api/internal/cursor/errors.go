package cursor

import (
	"errors"
	"fmt"
)

var (
	ErrAuth      = errors.New("cursor api: authentication failed")
	ErrRateLimit = errors.New("cursor api: rate limited")
	ErrTimeout   = errors.New("cursor api: request timed out")
)

// ErrUpstream wraps a non-auth HTTP or parse failure from the Cursor (or compatible) API.
type ErrUpstream struct {
	Status int
	Msg    string
}

func (e *ErrUpstream) Error() string {
	if e.Status > 0 {
		return fmt.Sprintf("cursor api: upstream status %d: %s", e.Status, e.Msg)
	}
	return "cursor api: " + e.Msg
}

func (e *ErrUpstream) Unwrap() error { return nil }
