// Package version holds build metadata injected with -ldflags at compile time.
//
// Example:
//
//	go build -ldflags "-X hyperspeed/api/internal/version.Version=1.2.3 -X hyperspeed/api/internal/version.GitSHA=abc123" ./cmd/server
package version

// Version is a semver or release tag (e.g. 1.2.3). Default for dev builds.
var Version = "dev"

// GitSHA is the VCS revision, optional.
var GitSHA = ""
