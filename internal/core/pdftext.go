package core

import (
	"fmt"
	"strings"

	"github.com/ledongthuc/pdf"
)

// PDFTextExtractor extracts text from PDF files.
type PDFTextExtractor struct{}

// NewPDFTextExtractor creates a new PDF text extractor.
func NewPDFTextExtractor() *PDFTextExtractor {
	return &PDFTextExtractor{}
}

// ExtractText extracts text from a PDF file.
func (e *PDFTextExtractor) ExtractText(path string) (string, error) {
	f, r, err := pdf.Open(path)
	if err != nil {
		return "", fmt.Errorf("failed to open PDF: %w", err)
	}
	defer func() { _ = f.Close() }()

	var sb strings.Builder
	totalPages := r.NumPage()

	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		page := r.Page(pageNum)
		if page.V.IsNull() {
			continue
		}

		text, err := page.GetPlainText(nil)
		if err != nil {
			// Try to continue with other pages
			continue
		}

		sb.WriteString(text)
		sb.WriteString("\n")
	}

	return sb.String(), nil
}

// HasText checks if the extracted text is meaningful (not just whitespace).
func HasText(text string) bool {
	trimmed := strings.TrimSpace(text)
	return len(trimmed) > 10 // Arbitrary minimum length
}
