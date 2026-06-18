package core

import (
	"fmt"
	"image"

	"github.com/gen2brain/go-fitz"
)

// RenderPDF renders every page of the PDF at the given DPI to an image.
// Pages are returned in document order (index 0 = page 1). Higher DPI
// values produce sharper but larger images.
func RenderPDF(path string, dpi float64) ([]image.Image, error) {
	doc, err := fitz.New(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open PDF for rendering: %w", err)
	}
	defer doc.Close()

	numPages := doc.NumPage()
	pages := make([]image.Image, 0, numPages)
	for n := 0; n < numPages; n++ {
		img, err := doc.ImageDPI(n, dpi)
		if err != nil {
			return nil, fmt.Errorf("failed to render PDF page %d: %w", n+1, err)
		}
		pages = append(pages, img)
	}
	return pages, nil
}
