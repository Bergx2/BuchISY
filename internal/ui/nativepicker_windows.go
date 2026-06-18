//go:build windows

package ui

import (
	"path/filepath"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Win32-native "Open File" dialog (GetOpenFileNameW). Fyne's built-in
// dialog is known to crash on Windows when users switch its view mode
// or sort order; the OS dialog is rock-solid and has all those modes
// built in.

var (
	comdlg32             = windows.NewLazySystemDLL("comdlg32.dll")
	procGetOpenFileNameW = comdlg32.NewProc("GetOpenFileNameW")
)

const (
	ofnPathMustExist    = 0x00000800
	ofnFileMustExist    = 0x00001000
	ofnExplorer         = 0x00080000
	ofnNoChangeDir      = 0x00000008
	ofnAllowMultiSelect = 0x00000200
)

type openFileNameW struct {
	StructSize        uint32
	HwndOwner         uintptr
	HInstance         uintptr
	LpstrFilter       *uint16
	LpstrCustomFilter *uint16
	NMaxCustFilter    uint32
	NFilterIndex      uint32
	LpstrFile         *uint16
	NMaxFile          uint32
	LpstrFileTitle    *uint16
	NMaxFileTitle     uint32
	LpstrInitialDir   *uint16
	LpstrTitle        *uint16
	Flags             uint32
	NFileOffset       uint16
	NFileExtension    uint16
	LpstrDefExt       *uint16
	LCustData         uintptr
	LpfnHook          uintptr
	LpTemplateName    *uint16
	PvReserved        uintptr
	DwReserved        uint32
	FlagsEx           uint32
}

// pickFileNative opens the OS file-open dialog and returns the picked
// path. The bool is false when the user cancels or the dialog can't
// be opened.
func pickFileNative(initialDir, title string) (string, bool) {
	// Buffer must be large enough for long paths.
	buf := make([]uint16, 32768)

	filter := buildFilter("Alle Dateien (*.*)", "*.*")

	var initDirPtr *uint16
	if initialDir != "" {
		if p, err := syscall.UTF16PtrFromString(initialDir); err == nil {
			initDirPtr = p
		}
	}
	var titlePtr *uint16
	if title != "" {
		if p, err := syscall.UTF16PtrFromString(title); err == nil {
			titlePtr = p
		}
	}

	ofn := openFileNameW{
		StructSize:      uint32(unsafe.Sizeof(openFileNameW{})),
		LpstrFilter:     &filter[0],
		LpstrFile:       &buf[0],
		NMaxFile:        uint32(len(buf)),
		LpstrInitialDir: initDirPtr,
		LpstrTitle:      titlePtr,
		Flags:           ofnPathMustExist | ofnFileMustExist | ofnExplorer | ofnNoChangeDir,
	}

	ret, _, _ := procGetOpenFileNameW.Call(uintptr(unsafe.Pointer(&ofn)))
	if ret == 0 {
		return "", false
	}
	return syscall.UTF16ToString(buf), true
}

// pickFilesNative opens the OS dialog in multi-select mode. Result
// buffer with OFN_ALLOWMULTISELECT + OFN_EXPLORER is:
//   - one full path (single selection), OR
//   - "directory\0file1\0file2\0…\0\0" (multi).
// Returns the full paths.
func pickFilesNative(initialDir, title string) ([]string, bool) {
	buf := make([]uint16, 65536) // bigger buffer for multi-select

	filter := buildFilter("Alle Dateien (*.*)", "*.*")

	var initDirPtr *uint16
	if initialDir != "" {
		if p, err := syscall.UTF16PtrFromString(initialDir); err == nil {
			initDirPtr = p
		}
	}
	var titlePtr *uint16
	if title != "" {
		if p, err := syscall.UTF16PtrFromString(title); err == nil {
			titlePtr = p
		}
	}

	ofn := openFileNameW{
		StructSize:      uint32(unsafe.Sizeof(openFileNameW{})),
		LpstrFilter:     &filter[0],
		LpstrFile:       &buf[0],
		NMaxFile:        uint32(len(buf)),
		LpstrInitialDir: initDirPtr,
		LpstrTitle:      titlePtr,
		Flags: ofnPathMustExist | ofnFileMustExist | ofnExplorer |
			ofnNoChangeDir | ofnAllowMultiSelect,
	}

	ret, _, _ := procGetOpenFileNameW.Call(uintptr(unsafe.Pointer(&ofn)))
	if ret == 0 {
		return nil, false
	}

	parts := splitNullSeparated(buf)
	switch len(parts) {
	case 0:
		return nil, false
	case 1:
		// Single file → full path is in parts[0].
		return []string{parts[0]}, true
	}
	// Multi → first part is the directory, rest are filenames.
	dir := parts[0]
	out := make([]string, 0, len(parts)-1)
	for _, name := range parts[1:] {
		out = append(out, filepath.Join(dir, name))
	}
	return out, true
}

// splitNullSeparated decodes a Win32 \0-separated, \0\0-terminated
// UTF-16 buffer into Go strings.
func splitNullSeparated(buf []uint16) []string {
	var out []string
	start := 0
	for i := 0; i < len(buf); i++ {
		if buf[i] == 0 {
			if i == start {
				break // double null = end of list
			}
			out = append(out, syscall.UTF16ToString(buf[start:i]))
			start = i + 1
		}
	}
	return out
}

// nativePickerAvailable reports that we should use the native dialog
// on this platform.
func nativePickerAvailable() bool { return true }

// buildFilter constructs the double-null-terminated UTF-16 buffer the
// Win32 dialog expects for its filter pairs: "label\0pattern\0…\0\0".
func buildFilter(pairs ...string) []uint16 {
	var out []uint16
	for _, p := range pairs {
		u, _ := syscall.UTF16FromString(p) // includes trailing null
		out = append(out, u...)
	}
	out = append(out, 0) // final double-null terminator
	return out
}
