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

// ExtractTextGuarded extracts a PDF's text using go-fitz (MuPDF), which is fast
// and robust and does not hang.
//
// It deliberately does NOT use the pure-Go ledongthuc/pdf parser (ExtractText):
// on certain PDFs — e.g. some digitally-produced invoices — that parser enters
// an unbounded-allocation loop, allocating ~500 MB/s with no end. Because a Go
// goroutine cannot be killed, guarding it with a timeout could only *abandon*
// the goroutine, which then kept eating memory until the whole machine froze.
// go-fitz is already used everywhere else in the app (rendering, bank-statement
// parsing) and reads those same PDFs cleanly in milliseconds.
//
// The timeout parameter is retained for API compatibility; go-fitz returns
// effectively instantly, so it is unused.
func (e *PDFTextExtractor) ExtractTextGuarded(path string, timeout time.Duration) (string, error) {
	_ = timeout
	return extractTextViaFitz(path)
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
