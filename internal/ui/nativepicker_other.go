//go:build !windows

package ui

// pickFileNative is a no-op on non-Windows platforms; the caller should
// fall back to Fyne's own file dialog.
func pickFileNative(_, _ string) (string, bool)    { return "", false }
func pickFilesNative(_, _ string) ([]string, bool) { return nil, false }

func nativePickerAvailable() bool { return false }
