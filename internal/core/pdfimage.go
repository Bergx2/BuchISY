package core

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/gen2brain/go-fitz"
)

// PDFToImageBase64 converts the first page of a PDF to PNG and returns base64 + media type.
// On macOS ARM64, uses external commands to avoid signal handling issues with go-fitz.
// On other platforms (Windows, Linux, macOS Intel), uses go-fitz which works reliably.
func PDFToImageBase64(path string) (string, string, error) {
	// On macOS ARM64, use external command to avoid signal handling crashes
	if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
		return pdfToImageExternal(path)
	}

	// For all other platforms (Windows, Linux, macOS Intel), use go-fitz
	return pdfToImageGoFitz(path)
}

// pdfToImageGoFitz uses go-fitz (MuPDF) to render PDF as image.
// This works well on Windows, Linux, and macOS Intel.
func pdfToImageGoFitz(path string) (string, string, error) {
	// Recover from potential crashes (shouldn't happen on non-ARM64)
	defer func() {
		if r := recover(); r != nil {
			// Log but don't crash the whole app
		}
	}()

	// Open PDF document
	doc, err := fitz.New(path)
	if err != nil {
		return "", "", fmt.Errorf("failed to open PDF: %w", err)
	}
	defer func() { _ = doc.Close() }()

	// Check if document has pages
	if doc.NumPage() < 1 {
		return "", "", fmt.Errorf("PDF has no pages")
	}

	// Render first page to image at 2x resolution (DPI=144)
	// Higher DPI = better quality for Claude to read
	img, err := doc.Image(0)
	if err != nil {
		return "", "", fmt.Errorf("failed to render PDF page: %w", err)
	}

	// Encode image as PNG
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", "", fmt.Errorf("failed to encode image as PNG: %w", err)
	}

	// Convert to base64
	encoded := base64.StdEncoding.EncodeToString(buf.Bytes())

	return encoded, "image/png", nil
}

// pdfToImageExternal uses external commands to convert PDF to image on macOS.
// This avoids the signal handling issues with go-fitz on ARM64.
func pdfToImageExternal(path string) (string, string, error) {
	// Create temporary file for the PNG output
	tempDir := os.TempDir()
	tempFile := filepath.Join(tempDir, fmt.Sprintf("buchisy_%d.png", os.Getpid()))
	defer os.Remove(tempFile) // Clean up temp file

	// Try using sips first (built into macOS)
	// sips can convert PDF to PNG directly
	cmd := exec.Command("sips", "-s", "format", "png", "--resampleHeightWidthMax", "2400", path, "--out", tempFile)
	output, err := cmd.CombinedOutput()

	if err != nil {
		// If sips fails, try using ImageMagick's convert command if available
		// convert -density 200 input.pdf[0] output.png
		cmd = exec.Command("convert", "-density", "200", "-quality", "90", path+"[0]", tempFile)
		output, err = cmd.CombinedOutput()

		if err != nil {
			// If both fail, try Ghostscript as last resort
			cmd = exec.Command("gs", "-dNOPAUSE", "-dBATCH", "-sDEVICE=png16m",
				"-r200", "-dFirstPage=1", "-dLastPage=1",
				fmt.Sprintf("-sOutputFile=%s", tempFile), path)
			output, err = cmd.CombinedOutput()

			if err != nil {
				return "", "", fmt.Errorf("failed to convert PDF to image (tried sips, convert, gs): %v, output: %s", err, output)
			}
		}
	}

	// Read the generated PNG file
	pngData, err := os.ReadFile(tempFile)
	if err != nil {
		return "", "", fmt.Errorf("failed to read generated PNG: %w", err)
	}

	// Validate it's a valid PNG by trying to decode it
	_, err = png.Decode(bytes.NewReader(pngData))
	if err != nil {
		return "", "", fmt.Errorf("generated file is not a valid PNG: %w", err)
	}

	// Convert to base64
	encoded := base64.StdEncoding.EncodeToString(pngData)

	return encoded, "image/png", nil
}
