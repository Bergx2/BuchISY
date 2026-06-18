//go:build windows

package ui

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Win32 clipboard reading. Fyne's own Clipboard API only handles text,
// so we walk the WinAPI directly to grab two things:
//   - file paths (CF_HDROP, e.g. files copied in Explorer)
//   - bitmap images (CF_DIB / CF_DIBV5, e.g. screenshots from the
//     Snipping Tool, image viewers, browsers)

var (
	user32           = windows.NewLazySystemDLL("user32.dll")
	shell32          = windows.NewLazySystemDLL("shell32.dll")
	kernel32         = windows.NewLazySystemDLL("kernel32.dll")
	procOpenClip     = user32.NewProc("OpenClipboard")
	procGetClipData  = user32.NewProc("GetClipboardData")
	procCloseClip    = user32.NewProc("CloseClipboard")
	procEnumFmt      = user32.NewProc("EnumClipboardFormats")
	procGetFmtName   = user32.NewProc("GetClipboardFormatNameW")
	procDragQueryW   = shell32.NewProc("DragQueryFileW")
	procGlobalLock   = kernel32.NewProc("GlobalLock")
	procGlobalUnlock = kernel32.NewProc("GlobalUnlock")
	procGlobalSize   = kernel32.NewProc("GlobalSize")
)

const (
	cfBitmap = 2
	cfDIB    = 8
	cfHDrop  = 15
	cfDIBV5  = 17
)

// openClipboardRetry opens the clipboard, retrying briefly because
// other apps (Explorer, browsers) sometimes hold the lock for a few
// milliseconds after a copy.
func openClipboardRetry() bool {
	for i := 0; i < 10; i++ {
		r, _, _ := procOpenClip.Call(0)
		if r != 0 {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

// clipboardFiles returns the file paths currently on the Windows
// clipboard. Empty when the clipboard holds something other than a
// file list, or when the clipboard can't be opened.
func clipboardFiles() []string {
	if !openClipboardRetry() {
		return nil
	}
	defer procCloseClip.Call()

	hDrop, _, _ := procGetClipData.Call(uintptr(cfHDrop))
	if hDrop == 0 {
		return nil
	}

	// DragQueryFile(h, 0xFFFFFFFF, …) returns the file count.
	n, _, _ := procDragQueryW.Call(hDrop, ^uintptr(0), 0, 0)
	if n == 0 {
		return nil
	}

	files := make([]string, 0, n)
	buf := make([]uint16, 32768) // generous: handles long paths
	for i := uintptr(0); i < n; i++ {
		l, _, _ := procDragQueryW.Call(
			hDrop, i,
			uintptr(unsafe.Pointer(&buf[0])),
			uintptr(len(buf)),
		)
		if l == 0 {
			continue
		}
		files = append(files, syscall.UTF16ToString(buf[:l]))
	}
	return files
}

// clipboardImage returns the bitmap currently on the Windows clipboard.
// Tries CF_DIB then CF_DIBV5 directly (Windows synthesizes one from the
// other but IsClipboardFormatAvailable doesn't reliably report
// synthesized formats — calling GetClipboardData blindly is safer).
// Returns nil when no image format yields decodable bytes.
func clipboardImage() image.Image {
	if !openClipboardRetry() {
		return nil
	}
	defer procCloseClip.Call()

	for _, fmt := range []uintptr{cfDIB, cfDIBV5} {
		if img, _ := readDIBFormat(fmt); img != nil {
			return img
		}
	}
	return nil
}

// clipboardImageDiagnostic is like clipboardImage but also returns a
// human-readable trace of each attempt's failure reason. Used to log
// why a paste came up empty.
func clipboardImageDiagnostic() (image.Image, string) {
	if !openClipboardRetry() {
		return nil, "OpenClipboard failed after retries"
	}
	defer procCloseClip.Call()

	var notes []string
	for _, fmt := range []uintptr{cfDIB, cfDIBV5} {
		img, why := readDIBFormat(fmt)
		if img != nil {
			return img, ""
		}
		notes = append(notes, fmt2str(fmt)+": "+why)
	}
	return nil, strings.Join(notes, "; ")
}

// clipboardFormatsDiagnostic returns a human-readable list of every
// format currently on the clipboard. Used to log what's available when
// the paste path comes up empty.
func clipboardFormatsDiagnostic() string {
	if !openClipboardRetry() {
		return "(clipboard locked by another app)"
	}
	defer procCloseClip.Call()

	var parts []string
	var fmtID uintptr = 0
	for {
		next, _, _ := procEnumFmt.Call(fmtID)
		if next == 0 {
			break
		}
		parts = append(parts, fmt2str(next))
		fmtID = next
	}
	if len(parts) == 0 {
		return "(empty)"
	}
	return strings.Join(parts, ", ")
}

func fmt2str(id uintptr) string {
	switch id {
	case 1:
		return "CF_TEXT"
	case cfBitmap:
		return "CF_BITMAP"
	case 7:
		return "CF_OEMTEXT"
	case cfDIB:
		return "CF_DIB"
	case 13:
		return "CF_UNICODETEXT"
	case cfHDrop:
		return "CF_HDROP"
	case cfDIBV5:
		return "CF_DIBV5"
	}
	buf := make([]uint16, 256)
	n, _, _ := procGetFmtName.Call(id,
		uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n > 0 {
		return syscall.UTF16ToString(buf[:n])
	}
	return fmt.Sprintf("CF_%d", id)
}

// readDIBFormat fetches one DIB-style clipboard format and turns its
// raw bytes into an image.Image directly (no detour through BMP file
// decoding, which rejects V4/V5 headers).
func readDIBFormat(format uintptr) (image.Image, string) {
	hData, _, _ := procGetClipData.Call(format)
	if hData == 0 {
		return nil, "GetClipboardData returned 0"
	}
	ptr, _, _ := procGlobalLock.Call(hData)
	if ptr == 0 {
		return nil, "GlobalLock returned 0"
	}
	defer procGlobalUnlock.Call(hData)

	size, _, _ := procGlobalSize.Call(hData)
	if size < 40 {
		return nil, fmt.Sprintf("GlobalSize=%d (too small)", size)
	}

	dib := make([]byte, int(size))
	copy(dib, unsafe.Slice((*byte)(unsafe.Pointer(ptr)), int(size)))

	img, err := dibToImage(dib)
	if err != nil {
		return nil, "dibToImage: " + err.Error()
	}
	return img, ""
}

// dibToImage decodes a packed DIB (BITMAPINFOHEADER / V4 / V5 header
// followed by optional bitfield masks and pixel data) into an NRGBA
// image. Supports 24- and 32-bit BGR(A) sources, which covers every
// screenshot path on modern Windows.
func dibToImage(dib []byte) (image.Image, error) {
	if len(dib) < 40 {
		return nil, fmt.Errorf("DIB too short (%d bytes)", len(dib))
	}

	biSize := binary.LittleEndian.Uint32(dib[0:4])
	biWidth := int32(binary.LittleEndian.Uint32(dib[4:8]))
	biHeight := int32(binary.LittleEndian.Uint32(dib[8:12]))
	biBitCount := binary.LittleEndian.Uint16(dib[14:16])
	biCompression := binary.LittleEndian.Uint32(dib[16:20])

	if biBitCount != 24 && biBitCount != 32 {
		return nil, fmt.Errorf("unsupported biBitCount=%d", biBitCount)
	}
	if biWidth <= 0 {
		return nil, fmt.Errorf("invalid biWidth=%d", biWidth)
	}

	// Bitfield masks: V4/V5 (biSize >= 108) carry them inside the
	// header; V1 (40) appends them after for BI_BITFIELDS / BI_ALPHA-
	// BITFIELDS.
	pixelOffset := uint32(biSize)
	if biSize == 40 {
		switch biCompression {
		case 3:
			pixelOffset += 12
		case 6:
			pixelOffset += 16
		}
	}
	if uint32(len(dib)) <= pixelOffset {
		return nil, fmt.Errorf("no pixel data (offset %d >= len %d)",
			pixelOffset, len(dib))
	}
	pixels := dib[pixelOffset:]

	width := int(biWidth)
	flipY := biHeight > 0 // positive height = bottom-up rows
	height := int(biHeight)
	if height < 0 {
		height = -height
	}

	bytesPerPixel := int(biBitCount) / 8
	rowSize := ((width*int(biBitCount) + 31) / 32) * 4 // DWORD-padded

	if rowSize*height > len(pixels) {
		return nil, fmt.Errorf("truncated pixel data (need %d have %d)",
			rowSize*height, len(pixels))
	}

	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		srcRow := y
		if flipY {
			srcRow = height - 1 - y
		}
		base := srcRow * rowSize
		for x := 0; x < width; x++ {
			i := base + x*bytesPerPixel
			b, g, r := pixels[i], pixels[i+1], pixels[i+2]
			var a uint8 = 255
			if bytesPerPixel == 4 {
				a = pixels[i+3]
				// Screenshots usually leave the alpha byte at 0 even
				// though the intent is opaque; treat 0 as opaque so
				// we don't render a fully transparent image.
				if a == 0 {
					a = 255
				}
			}
			img.SetNRGBA(x, y, color.NRGBA{R: r, G: g, B: b, A: a})
		}
	}
	return img, nil
}
