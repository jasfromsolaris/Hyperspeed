package rest

import "hyperspeed/api/internal/slug"

// Slugify turns a display name into a DNS-safe slug fragment.
func Slugify(name string) string {
	return slug.Slugify(name)
}
