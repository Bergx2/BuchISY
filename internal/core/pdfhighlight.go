package core

import (
	"fmt"
	"strings"

	"github.com/ledongthuc/pdf"
)

// Rect is a rectangle in image pixels (origin top-left).
type Rect struct {
	X, Y, W, H float32
}

// pdfBox is a rectangle in PDF points (origin bottom-left).
type pdfBox struct {
	x, y, w, h float64
}

// pdfBoxToPixel converts a PDF-point box to an image-pixel Rect.
// pageHeight is the page height in PDF points; dpi the render resolution.
// The Y axis is flipped (PDF origin bottom-left, image origin top-left).
func pdfBoxToPixel(b pdfBox, pageHeight, dpi float64) Rect {
	scale := dpi / 72.0
	return Rect{
		X: float32(b.x * scale),
		Y: float32((pageHeight - b.y - b.h) * scale),
		W: float32(b.w * scale),
		H: float32(b.h * scale),
	}
}

// valueBoxInRow searches one row of text fragments for value (verbatim,
// case-insensitive, whitespace-trimmed) and returns the PDF-point box
// enclosing the matching fragments. ok is false if value is empty or does
// not appear in the row.
func valueBoxInRow(frags []pdf.Text, value string) (box pdfBox, ok bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return pdfBox{}, false
	}

	var sb strings.Builder
	starts := make([]int, len(frags))
	ends := make([]int, len(frags))
	for i, f := range frags {
		starts[i] = sb.Len()
		sb.WriteString(f.S)
		ends[i] = sb.Len()
	}

	idx := strings.Index(strings.ToLower(sb.String()), strings.ToLower(value))
	if idx < 0 {
		return pdfBox{}, false
	}
	matchStart, matchEnd := idx, idx+len(value)

	first := true
	var minX, minY, maxX, maxY float64
	for i, f := range frags {
		if ends[i] <= matchStart || starts[i] >= matchEnd {
			continue
		}
		fh := f.FontSize
		if fh <= 0 {
			fh = 8
		}
		fx0, fy0 := f.X, f.Y
		fx1, fy1 := f.X+f.W, f.Y+fh
		if first {
			minX, minY, maxX, maxY = fx0, fy0, fx1, fy1
			first = false
			continue
		}
		if fx0 < minX {
			minX = fx0
		}
		if fy0 < minY {
			minY = fy0
		}
		if fx1 > maxX {
			maxX = fx1
		}
		if fy1 > maxY {
			maxY = fy1
		}
	}
	if first {
		return pdfBox{}, false
	}
	return pdfBox{x: minX, y: minY, w: maxX - minX, h: maxY - minY}, true
}

// HighlightRects opens the PDF and returns, per page (index 0 = page 1),
// the list of yellow highlight rectangles in image pixels for the render
// resolution dpi. dpi must match the value passed to RenderPDF. Values not
// found verbatim in a page's text produce no rectangle. A page whose text
// cannot be read yields a nil slice (no highlights, no error).
func HighlightRects(path string, values []string, dpi float64) ([][]Rect, error) {
	f, r, err := pdf.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open PDF for highlighting: %w", err)
	}
	defer f.Close()

	total := r.NumPage()
	result := make([][]Rect, 0, total)
	for pageNum := 1; pageNum <= total; pageNum++ {
		page := r.Page(pageNum)
		if page.V.IsNull() {
			result = append(result, nil)
			continue
		}
		rows, err := page.GetTextByRow()
		if err != nil {
			result = append(result, nil)
			continue
		}
		pageHeight := pageHeightPoints(page)

		var rects []Rect
		for _, row := range rows {
			frags := []pdf.Text(row.Content)
			for _, value := range values {
				if box, ok := valueBoxInRow(frags, value); ok {
					rects = append(rects, pdfBoxToPixel(box, pageHeight, dpi))
				}
			}
		}
		result = append(result, rects)
	}
	return result, nil
}

// pageHeightPoints returns the page height in PDF points from its MediaBox,
// falling back to A4 height if the MediaBox is missing or malformed.
//
// MediaBox is an inheritable attribute: it may live on the page dictionary
// itself or on an ancestor node in the page tree. This library version does
// not expose a MediaBox accessor, so the inheritance chain is walked here via
// the exported Key("Parent") method.
func pageHeightPoints(page pdf.Page) float64 {
	const a4Height = 842.0
	for v := page.V; !v.IsNull(); v = v.Key("Parent") {
		mb := v.Key("MediaBox")
		if mb.Len() < 4 {
			continue
		}
		h := mb.Index(3).Float64() - mb.Index(1).Float64()
		if h <= 0 {
			return a4Height
		}
		return h
	}
	return a4Height
}
