package core

import (
	"regexp"
	"strings"
)

// SanitizeFilename removes or replaces characters that are unsafe for filenames.
// It preserves umlauts but removes/replaces other problematic characters.
func SanitizeFilename(name string) string {
	// Replace slashes and backslashes with hyphens
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.ReplaceAll(name, "\\", "-")

	// Remove or replace other unsafe characters
	// Keep: letters, digits, dots, hyphens, underscores, umlauts
	unsafeChars := regexp.MustCompile(`[<>:"|?*\x00-\x1f]`)
	name = unsafeChars.ReplaceAllString(name, "")

	// Replace multiple spaces with a single space
	name = regexp.MustCompile(`\s+`).ReplaceAllString(name, " ")

	// Trim spaces from ends
	name = strings.TrimSpace(name)

	// Replace spaces with hyphens or underscores (optional, keep spaces for readability)
	// Uncomment if you prefer no spaces:
	// name = strings.ReplaceAll(name, " ", "-")

	return name
}

// NormalizeCompanyName normalizes a company name for stable mapping keys.
// It lowercases, trims, and removes common spacing variants.
func NormalizeCompanyName(name string) string {
	// Lowercase
	name = strings.ToLower(name)

	// Trim spaces
	name = strings.TrimSpace(name)

	// Remove multiple spaces
	name = regexp.MustCompile(`\s+`).ReplaceAllString(name, " ")

	// Remove common legal suffixes for better matching
	suffixes := []string{" gmbh", " ag", " kg", " ohg", " gbr", " ug", " e.k.", " ltd", " inc", " corp"}
	for _, suffix := range suffixes {
		if strings.HasSuffix(name, suffix) {
			name = strings.TrimSuffix(name, suffix)
			name = strings.TrimSpace(name)
			break
		}
	}

	return name
}
