package core

import "strings"

// MissingConfigHints returns i18n keys for unmet setup preconditions, shown as
// dismissible banners. hasAPIKey reports whether a Claude API key is stored.
func MissingConfigHints(s Settings, hasAPIKey bool) []string {
	var hints []string
	if s.ProcessingMode == "claude" && !hasAPIKey {
		hints = append(hints, "hint.no_api_key")
	}
	if strings.TrimSpace(s.StorageRoot) == "" {
		hints = append(hints, "hint.no_storage")
	}
	return hints
}
