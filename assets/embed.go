// Package assets embeds the runtime resources (translation files, ...)
// into the binary so that the produced .exe is self-contained and does
// not need a separate assets folder next to it.
package assets

import "embed"

// Translations contains the JSON translation files under "i18n/<lang>.json".
//
//go:embed i18n
var Translations embed.FS
