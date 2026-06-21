// Package assets embeds the runtime resources (translation files, ...)
// into the binary so that the produced .exe is self-contained and does
// not need a separate assets folder next to it.
package assets

import "embed"

// Translations contains the JSON translation files under "i18n/<lang>.json".
//
//go:embed i18n
var Translations embed.FS

// EmbeddedFiles contains all embedded asset files
//
//go:embed i18n/*.json
var EmbeddedFiles embed.FS

// SKR04JSON is the bundled starter SKR04 chart of accounts (extend via import).
//
//go:embed skr04.json
var SKR04JSON []byte

// GetTranslationFile returns the content of a translation file
func GetTranslationFile(filename string) ([]byte, error) {
	return EmbeddedFiles.ReadFile("i18n/" + filename)
}
