package core

import (
	"fmt"
	"strings"
	"time"

	"github.com/gen2brain/go-fitz"
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

	// PDFs sometimes encode a separator glyph (e.g. the hyphen in a receipt
	// number "MC9C7PFZ-103052") with a font mapping the extractor can't decode,
	// yielding the Unicode replacement char U+FFFD. Normalize it to a hyphen so
	// numbers like "MC9C7PFZ�103052" come through whole.
	return strings.ReplaceAll(sb.String(), "�", "-"), nil
}

// ExtractTextGuarded runs ExtractText but gives up after timeout and falls back
// to go-fitz (MuPDF). The ledongthuc/pdf parser can hang indefinitely on some
// PDFs (certain content-stream encodings) — that abandoned goroutine is left to
// finish on its own while we return text extracted the robust way. go-fitz is
// already a dependency (used for rendering) and handles those PDFs cleanly.
func (e *PDFTextExtractor) ExtractTextGuarded(path string, timeout time.Duration) (string, error) {
	type result struct {
		txt string
		err error
	}
	ch := make(chan result, 1)
	go func() {
		txt, err := e.ExtractText(path)
		ch <- result{txt, err}
	}()
	select {
	case r := <-ch:
		// Primary parser returned in time but yielded no usable text — try the
		// MuPDF fallback before declaring the PDF text-less.
		if r.err == nil && !HasText(r.txt) {
			if alt, aerr := extractTextViaFitz(path); aerr == nil && HasText(alt) {
				return alt, nil
			}
		}
		return r.txt, r.err
	case <-time.After(timeout):
		return extractTextViaFitz(path)
	}
}

// extractTextViaFitz extracts plain text from a PDF using go-fitz (MuPDF) by
// flattening each page's positioned HTML runs — the same approach used for bank
// statements, so it's known to be robust.
func extractTextViaFitz(path string) (string, error) {
	doc, err := fitz.New(path)
	if err != nil {
		return "", fmt.Errorf("fitz open: %w", err)
	}
	defer doc.Close()
	htmls := make([]string, 0, doc.NumPage())
	for i := 0; i < doc.NumPage(); i++ {
		h, herr := doc.HTML(i, false)
		if herr != nil {
			continue
		}
		htmls = append(htmls, h)
	}
	return strings.ReplaceAll(buildPlainTextFromHTML(htmls), "�", "-"), nil
}

// HasText checks if the extracted text is meaningful (not just whitespace).
func HasText(text string) bool {
	trimmed := strings.TrimSpace(text)
	return len(trimmed) > 10 // Arbitrary minimum length
}
