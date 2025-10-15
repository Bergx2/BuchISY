package i18n

import (
	"encoding/json"
	"fmt"

	"github.com/bergx2/buchisy/assets"
)

// Embed translation files directly into the binary
// This ensures they're always available, even without the assets folder

// LoadEmbedded loads translations from embedded resources.
// This function is used when translation files are not found on the filesystem.
func LoadEmbedded(lang string) (*Bundle, error) {
	b := &Bundle{
		translations: make(map[string]string),
		fallback:     make(map[string]string),
	}

	// Always load German as fallback
	deData, err := assets.GetTranslationFile("de.json")
	if err != nil {
		return nil, fmt.Errorf("failed to load embedded fallback (de): %w", err)
	}
	if err := json.Unmarshal(deData, &b.fallback); err != nil {
		return nil, fmt.Errorf("failed to parse embedded fallback (de): %w", err)
	}

	// Load requested language
	switch lang {
	case "de":
		b.translations = b.fallback
	case "en":
		enData, err := assets.GetTranslationFile("en.json")
		if err != nil {
			// If file not found, use German
			b.translations = b.fallback
		} else if err := json.Unmarshal(enData, &b.translations); err != nil {
			// If parsing fails, use German
			b.translations = b.fallback
		}
	default:
		// Unknown language, use German
		b.translations = b.fallback
	}

	return b, nil
}

// AvailableLanguagesEmbedded returns the list of embedded languages.
func AvailableLanguagesEmbedded() []string {
	return []string{"de", "en"}
}
