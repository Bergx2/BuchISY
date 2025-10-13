package core

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image/png"

	"github.com/gen2brain/go-fitz"
)

// PDFToImageBase64 converts the first page of a PDF to PNG and returns base64 + media type.
// Uses go-fitz (MuPDF) to render the PDF page as an image.
func PDFToImageBase64(path string) (string, string, error) {
	// Open PDF document
	doc, err := fitz.New(path)
	if err != nil {
		return "", "", fmt.Errorf("failed to open PDF: %w", err)
	}
	defer doc.Close()

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
