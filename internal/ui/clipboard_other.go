//go:build !windows

package ui

import "image"

// clipboardFiles/clipboardImage/clipboardFormatsDiagnostic are no-ops
// on non-Windows platforms; Fyne's text-only Clipboard API and the
// lack of a portable file-clipboard format mean we just disable the
// feature gracefully. Picking files via the "…" button keeps working
// on every platform.
func clipboardFiles() []string                { return nil }
func clipboardImage() image.Image             { return nil }
func clipboardFormatsDiagnostic() string      { return "(not supported)" }
