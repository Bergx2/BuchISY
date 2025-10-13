// Package i18n provides internationalization support with JSON resource bundles.
package i18n

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Bundle holds translations for a specific language.
type Bundle struct {
	translations map[string]string
	fallback     map[string]string
}

// Load loads translations from the assets directory for the given language.
// It always loads "de" as fallback and then the requested language if different.
func Load(assetsDir, lang string) (*Bundle, error) {
	b := &Bundle{
		translations: make(map[string]string),
		fallback:     make(map[string]string),
	}

	// Always load German as fallback
	fallbackPath := filepath.Join(assetsDir, "i18n", "de.json")
	if err := b.loadFile(fallbackPath, b.fallback); err != nil {
		return nil, fmt.Errorf("failed to load fallback (de): %w", err)
	}

	// If requested language is not German, load it too
	if lang != "de" {
		langPath := filepath.Join(assetsDir, "i18n", fmt.Sprintf("%s.json", lang))
		if err := b.loadFile(langPath, b.translations); err != nil {
			// If language not found, use German as main
			b.translations = b.fallback
		}
	} else {
		b.translations = b.fallback
	}

	return b, nil
}

// loadFile loads a JSON translation file into the given map.
func (b *Bundle) loadFile(path string, target map[string]string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, &target)
}

// T returns the translation for the given key.
// If the key is not found, it tries the fallback.
// If still not found, it returns the key itself.
func (b *Bundle) T(key string, args ...interface{}) string {
	// Try main translations
	if val, ok := b.translations[key]; ok {
		if len(args) > 0 {
			return fmt.Sprintf(val, args...)
		}
		return val
	}

	// Try fallback
	if val, ok := b.fallback[key]; ok {
		if len(args) > 0 {
			return fmt.Sprintf(val, args...)
		}
		return val
	}

	// Return key if not found
	return key
}

// AvailableLanguages returns the list of available languages by scanning the assets directory.
func AvailableLanguages(assetsDir string) []string {
	i18nDir := filepath.Join(assetsDir, "i18n")
	entries, err := os.ReadDir(i18nDir)
	if err != nil {
		return []string{"de"} // fallback
	}

	var langs []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".json") {
			lang := strings.TrimSuffix(name, ".json")
			langs = append(langs, lang)
		}
	}

	if len(langs) == 0 {
		return []string{"de"}
	}

	return langs
}
