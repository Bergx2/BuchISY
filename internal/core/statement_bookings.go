package core

import (
	"fmt"
	"html"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/gen2brain/go-fitz"
)

// lineAmountRe matches German money amounts like "1.234,56" or "78,53".
var lineAmountRe = regexp.MustCompile(`\d{1,3}(?:\.\d{3})*,\d{2}`)

// creditKeywordRe matches clear credit (Haben / incoming) signals on a line.
var creditKeywordRe = regexp.MustCompile(`(?i)gutschrift|zahlungseingang|geldeingang|überweisungseingang|lohn|gehalt`)

// trailingCreditRe matches an amount followed by a credit marker (" H" or "+").
var trailingCreditRe = regexp.MustCompile(`\d,\d{2}\s*([H+])\b|\d,\d{2}\s*\+`)

// dateLineRe matches a transaction line's leading date. Accepts both
// "DD.MM." (Sparkasse-style short date) and full "DD.MM.YYYY".
var dateLineRe = regexp.MustCompile(`^\s*([0-3]?\d)\.([01]?\d)\.(\d{4}|\d{2}|)`)

// pAttrTopRe / pAttrLeftRe pull the absolute pt coordinates out of a
// <p style="top:Npt;left:Mpt;...">. mupdf's HTML output uses these.
var (
	pAttrTopRe  = regexp.MustCompile(`top:\s*([\d.]+)pt`)
	pAttrLeftRe = regexp.MustCompile(`left:\s*([\d.]+)pt`)
)

// tagStripRe removes any HTML tag (we don't need attributes inside the
// visible text — just the user-facing characters).
var tagStripRe = regexp.MustCompile(`<[^>]+>`)

// ParseLineAmount returns the absolute value of the LAST money token in a
// statement line's text (the transaction amount sits at the end of the line),
// or 0 when none is present.
func ParseLineAmount(text string) float64 {
	matches := lineAmountRe.FindAllString(text, -1)
	if len(matches) == 0 {
		return 0
	}
	last := matches[len(matches)-1]
	last = strings.ReplaceAll(last, ".", "")
	last = strings.ReplaceAll(last, ",", ".")
	v, err := strconv.ParseFloat(last, 64)
	if err != nil {
		return 0
	}
	return v
}

// ParseLineIsCredit reports whether a statement line is CLEARLY an incoming
// credit (Haben). Ambiguous lines return false (treated as a debit) so a real
// expense match is never silently dropped.
func ParseLineIsCredit(text string) bool {
	if creditKeywordRe.MatchString(text) {
		return true
	}
	if trailingCreditRe.MatchString(text) {
		return true
	}
	return false
}

// ParseStatementBookings scans a bank statement file and returns
// transaction lines. For CAMT.053 XML and MT940 text files the
// structured parsers are used; for all other formats (PDF) the
// page-by-page MuPDF heuristic runs as before.
//
// The heuristic intentionally only checks the date prefix; "Kontostand
// am 02.01.2026" rows (which carry a date in the middle) are correctly
// skipped because they don't start with one.
func ParseStatementBookings(path string) ([]StatementBooking, error) {
	// E20.6: detect structured bank-statement formats before attempting PDF parse.
	data, err := os.ReadFile(path)
	if err == nil {
		switch DetectBankFormat(data) {
		case "camt":
			return ParseCAMT053(data)
		case "mt940":
			return ParseMT940(data)
		}
	}
	// Fall through to PDF parsing (go-fitz).
	return parseStatementPDF(path)
}

// parseStatementPDF is the original PDF-only implementation, factored out
// so the routing above can fall through cleanly.
func parseStatementPDF(path string) ([]StatementBooking, error) {
	doc, err := fitz.New(path)
	if err != nil {
		return nil, fmt.Errorf("open statement PDF: %w", err)
	}
	defer doc.Close()

	// Collect per-page HTML for both Qonto detection and the existing heuristic.
	pageHTMLs := make([]string, doc.NumPage())
	for page := 0; page < doc.NumPage(); page++ {
		htmlStr, err := doc.HTML(page, false)
		if err != nil {
			return nil, fmt.Errorf("extract page %d html: %w", page+1, err)
		}
		pageHTMLs[page] = htmlStr
	}

	// Build plain text from the run-extraction in bookingsFromPageHTML so we
	// can detect Qonto statements without needing a separate doc.Text() method.
	fullText := buildPlainTextFromHTML(pageHTMLs)

	// Qonto detection: if the document carries Qonto's columnar layout markers,
	// use the specialised parser instead of the generic heuristic. The positioned
	// variant records each booking's page + Y position so the UI can frame the
	// exact booking line on the rendered statement.
	if strings.Contains(fullText, "Qonto") && strings.Contains(fullText, "Abrechnungstag") {
		year := ""
		if m := qontoPeriodRe.FindStringSubmatch(fullText); m != nil {
			year = m[1]
		}
		var lines []qLine
		for page, htmlStr := range pageHTMLs {
			for _, ln := range extractHTMLLines(htmlStr) {
				lines = append(lines, qLine{text: ln.text, page: page, top: ln.top})
			}
		}
		if bookings := parseQontoCore(lines, year); len(bookings) >= 1 {
			return bookings, nil
		}
	}

	// Sparkasse "Umsätze - Druckansicht": a columnar online-banking export whose
	// two-row-per-booking layout the generic date-prefix heuristic mis-reads
	// (2×N+1 phantom rows). Parse it from positioned runs instead.
	if isSparkasseDruckansicht(fullText) {
		pageLines := make([][]htmlLine, len(pageHTMLs))
		for page, htmlStr := range pageHTMLs {
			pageLines[page] = extractHTMLLines(htmlStr)
		}
		if bookings := parseSparkasseDruckansicht(pageLines); len(bookings) >= 1 {
			return bookings, nil
		}
	}

	// Fall through to the existing page-by-page HTML heuristic.
	var out []StatementBooking
	for page, htmlStr := range pageHTMLs {
		pageBookings := bookingsFromPageHTML(htmlStr, page)
		out = append(out, pageBookings...)
	}
	return out, nil
}

// htmlLine is one positioned text run from a MuPDF HTML page.
type htmlLine struct {
	top, left float64
	text      string
}

// extractHTMLLines pulls the positioned <p> runs out of one MuPDF HTML page,
// sorted top→bottom then left→right (document order). Shared by the positioned
// Qonto parser so bookings keep their page + Y position.
func extractHTMLLines(pageHTML string) []htmlLine {
	var lines []htmlLine
	for _, chunk := range splitPTags(pageHTML) {
		topMatch := pAttrTopRe.FindStringSubmatch(chunk)
		leftMatch := pAttrLeftRe.FindStringSubmatch(chunk)
		if topMatch == nil || leftMatch == nil {
			continue
		}
		gt := strings.Index(chunk, ">")
		end := strings.LastIndex(chunk, "</p>")
		if gt < 0 || end < 0 || end <= gt {
			continue
		}
		text := chunk[gt+1 : end]
		text = tagStripRe.ReplaceAllString(text, "")
		text = html.UnescapeString(text)
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		top, _ := strconv.ParseFloat(topMatch[1], 64)
		left, _ := strconv.ParseFloat(leftMatch[1], 64)
		lines = append(lines, htmlLine{top: top, left: left, text: text})
	}
	sort.Slice(lines, func(i, j int) bool {
		if lines[i].top != lines[j].top {
			return lines[i].top < lines[j].top
		}
		return lines[i].left < lines[j].left
	})
	return lines
}

// buildPlainTextFromHTML converts a slice of MuPDF HTML pages into a single
// plain-text string, one run per line, sorted by vertical then horizontal
// position within each page. This reuses the same <p> extraction logic as
// bookingsFromPageHTML so no additional PDF rendering is required.
func buildPlainTextFromHTML(pageHTMLs []string) string {
	var sb strings.Builder
	for _, htmlStr := range pageHTMLs {
		type rawLine struct {
			top, left float64
			text      string
		}
		var lines []rawLine
		for _, chunk := range splitPTags(htmlStr) {
			topMatch := pAttrTopRe.FindStringSubmatch(chunk)
			leftMatch := pAttrLeftRe.FindStringSubmatch(chunk)
			if topMatch == nil || leftMatch == nil {
				continue
			}
			gt := strings.Index(chunk, ">")
			end := strings.LastIndex(chunk, "</p>")
			if gt < 0 || end < 0 || end <= gt {
				continue
			}
			text := chunk[gt+1 : end]
			text = tagStripRe.ReplaceAllString(text, "")
			text = html.UnescapeString(text)
			text = strings.TrimSpace(text)
			if text == "" {
				continue
			}
			top, _ := strconv.ParseFloat(topMatch[1], 64)
			left, _ := strconv.ParseFloat(leftMatch[1], 64)
			lines = append(lines, rawLine{top: top, left: left, text: text})
		}
		sort.Slice(lines, func(i, j int) bool {
			if lines[i].top != lines[j].top {
				return lines[i].top < lines[j].top
			}
			return lines[i].left < lines[j].left
		})
		for _, ln := range lines {
			sb.WriteString(ln.text)
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// bookingsFromPageHTML extracts numbered transaction lines from a
// single page's HTML. Exposed for testing without a real PDF.
func bookingsFromPageHTML(pageHTML string, page int) []StatementBooking {
	type rawLine struct {
		top, left float64
		text      string
	}
	var lines []rawLine

	// MuPDF emits one <p> per text run, one per source line. We
	// iterate <p ...>...</p> blocks via a simple state walk.
	for _, chunk := range splitPTags(pageHTML) {
		topMatch := pAttrTopRe.FindStringSubmatch(chunk)
		leftMatch := pAttrLeftRe.FindStringSubmatch(chunk)
		if topMatch == nil || leftMatch == nil {
			continue
		}
		// Visible text = everything between the first '>' and the closing </p>.
		gt := strings.Index(chunk, ">")
		end := strings.LastIndex(chunk, "</p>")
		if gt < 0 || end < 0 || end <= gt {
			continue
		}
		text := chunk[gt+1 : end]
		text = tagStripRe.ReplaceAllString(text, "")
		text = html.UnescapeString(text)
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		top, _ := strconv.ParseFloat(topMatch[1], 64)
		left, _ := strconv.ParseFloat(leftMatch[1], 64)
		lines = append(lines, rawLine{top: top, left: left, text: text})
	}

	// Sort by vertical position then horizontal, so document order is
	// preserved even if MuPDF emitted runs out of order.
	sort.Slice(lines, func(i, j int) bool {
		if lines[i].top != lines[j].top {
			return lines[i].top < lines[j].top
		}
		return lines[i].left < lines[j].left
	})

	var bookings []StatementBooking
	idx := 0
	for _, ln := range lines {
		m := dateLineRe.FindStringSubmatch(ln.text)
		if m == nil {
			continue
		}
		idx++
		date := m[0]
		// Trim trailing whitespace from the matched date prefix.
		date = strings.TrimSpace(date)
		// If year is missing (e.g. "05.01."), keep as-is; UI can
		// resolve to full year using statement period.
		bookings = append(bookings, StatementBooking{
			Page:          page,
			LineIdx:       idx,
			Date:          date,
			TopPt:         ln.top,
			LeftPt:        ln.left,
			Text:          ln.text,
			Betrag:        ParseLineAmount(ln.text),
			IstGutschrift: ParseLineIsCredit(ln.text),
		})
	}
	return bookings
}

// splitPTags returns each "<p ...>...</p>" substring as a separate
// chunk. We split deliberately rather than using a full HTML parser:
// mupdf's output is mechanical, and a 0-dependency string walk avoids
// pulling in golang.org/x/net.
func splitPTags(s string) []string {
	var out []string
	for {
		open := strings.Index(s, "<p ")
		if open < 0 {
			return out
		}
		close := strings.Index(s[open:], "</p>")
		if close < 0 {
			return out
		}
		end := open + close + len("</p>")
		out = append(out, s[open:end])
		s = s[end:]
	}
}
