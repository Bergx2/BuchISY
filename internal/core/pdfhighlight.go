package core

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/ledongthuc/pdf"
)

// amountRe matches a money amount with exactly two decimal places, allowing
// thousands separators in the integer part: "1234.56", "1.234,56", "573,15".
var amountRe = regexp.MustCompile(`^(\d[\d.,]*)[.,](\d{2})$`)

// numTokenRe finds numeric tokens (digits with . , separators) within a row.
var numTokenRe = regexp.MustCompile(`\d[\d.,]*\d|\d`)

// nonDigitRe strips everything that is not a digit.
var nonDigitRe = regexp.MustCompile(`\D`)

// amountDigits returns the digit signature of a 2-decimal money amount
// (integer digits + the two cents digits), independent of thousands separators
// or which decimal mark is used. It returns ok=false for anything that is not a
// 2-decimal amount (e.g. invoice numbers, dates, plain integers), so tolerant
// matching never confuses an amount with a same-digit identifier.
func amountDigits(s string) (string, bool) {
	s = strings.TrimSpace(s)
	m := amountRe.FindStringSubmatch(s)
	if m == nil {
		return "", false
	}
	intPart := nonDigitRe.ReplaceAllString(m[1], "")
	if intPart == "" {
		return "", false
	}
	return intPart + m[2], true
}

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

// pdfBoxToPixelTopOrigin maps a box whose Y is TOP-origin in a coordinate space
// of height coordHeight (which differs from the MediaBox) onto the rendered
// image. Some PDFs — e.g. bank statements generated from HTML — report text in
// a top-origin space scaled relative to the page, so text Y runs past the
// MediaBox height and the normal bottom-origin flip lands the rect off-page.
// Only Y/H need be accurate for the full-width statement frame; X/W are
// overridden by the caller's fullWidth handling.
func pdfBoxToPixelTopOrigin(b pdfBox, pageHeight, coordHeight, dpi float64) Rect {
	s := (dpi / 72.0) * (pageHeight / coordHeight)
	return Rect{
		X: float32(b.x * s),
		Y: float32(b.y * s),
		W: float32(b.w * s),
		H: float32(b.h * s),
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

	full := sb.String()
	var matchStart, matchEnd int
	matched := false
	// If the search value is a money amount, match it tolerantly: any numeric
	// token in the row with the same digit signature (so "1234.56" matches
	// "1.234,56", "573,15 €", etc.). Non-amounts fall back to a literal search.
	if vd, isAmt := amountDigits(value); isAmt {
		for _, loc := range numTokenRe.FindAllStringIndex(full, -1) {
			if td, ok := amountDigits(full[loc[0]:loc[1]]); ok && td == vd {
				matchStart, matchEnd = loc[0], loc[1]
				matched = true
				break
			}
		}
	}
	if !matched {
		idx := strings.Index(strings.ToLower(full), strings.ToLower(value))
		if idx < 0 {
			return pdfBox{}, false
		}
		matchStart, matchEnd = idx, idx+len(value)
	}

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

		// Detect a top-origin / scaled coordinate space: if the lowest text runs
		// past the MediaBox height, the text coords don't match the bottom-origin
		// MediaBox (common for HTML-generated statements). Map proportionally.
		maxY := 0.0
		for _, row := range rows {
			for _, f := range []pdf.Text(row.Content) {
				fh := f.FontSize
				if fh <= 0 {
					fh = 8
				}
				if y := f.Y + fh; y > maxY {
					maxY = y
				}
			}
		}
		topOrigin := maxY > pageHeight*1.02

		var rects []Rect
		for _, row := range rows {
			frags := []pdf.Text(row.Content)
			for _, value := range values {
				if box, ok := valueBoxInRow(frags, value); ok {
					if topOrigin {
						rects = append(rects, pdfBoxToPixelTopOrigin(box, pageHeight, maxY, dpi))
					} else {
						rects = append(rects, pdfBoxToPixel(box, pageHeight, dpi))
					}
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
