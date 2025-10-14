package assets

import (
	"embed"
)

// EmbeddedFiles contains all embedded asset files
//
//go:embed i18n/*.json
var EmbeddedFiles embed.FS

// GetTranslationFile returns the content of a translation file
func GetTranslationFile(filename string) ([]byte, error) {
	return EmbeddedFiles.ReadFile("i18n/" + filename)
}