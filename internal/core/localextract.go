package core

import (
	"fmt"
	"math"
	"regexp"
	"strings"
)

// LocalExtractor performs heuristic-based extraction from invoice text.
type LocalExtractor struct{}

// NewLocalExtractor creates a new local extractor.
func NewLocalExtractor() *LocalExtractor {
	return &LocalExtractor{}
}

// Extract extracts invoice metadata from text using heuristics.
// Returns Meta, confidence score (0-1), and error.
func (e *LocalExtractor) Extract(text string) (Meta, float64, error) {
	meta := Meta{
		Waehrung: "EUR", // default
	}

	confidence := 0.0
	matched := 0
	total := 0

	// Extract company name (first substantial line, often at top)
	if company := e.extractCompany(text); company != "" {
		meta.Firmenname = company
		matched++
	}
	total++

	// Extract invoice number
	if invoiceNum := e.extractInvoiceNumber(text); invoiceNum != "" {
		meta.Rechnungsnummer = invoiceNum
		matched++
	}
	total++

	// Extract invoice date
	if date := e.extractDate(text); date != "" {
		meta.Rechnungsdatum = date
		meta.DatumDeutsch = FormatGermanDate(date)
		parts := strings.Split(date, "-")
		if len(parts) == 3 {
			meta.Jahr = parts[0]
			meta.Monat = parts[1]
		}
		matched++
	}
	total++

	// Extract amounts
	if gross, net, vat, vatPercent := e.extractAmounts(text); gross > 0 {
		meta.Bruttobetrag = gross
		meta.BetragNetto = net
		meta.SteuersatzBetrag = vat
		meta.SteuersatzProzent = vatPercent
		matched++
	}
	total++

	// Extract currency
	if currency := e.extractCurrency(text); currency != "" {
		meta.Waehrung = currency
	}

	// Generate short description
	meta.Kurzbezeichnung = e.generateShortDesc(meta, text)

	// Calculate confidence
	if total > 0 {
		confidence = float64(matched) / float64(total)
	}

	return meta, confidence, nil
}

// extractCompany extracts the company name (vendor/issuer).
// Looks for lines near the top that look like company names.
func (e *LocalExtractor) extractCompany(text string) string {
	lines := strings.Split(text, "\n")
	for i := 0; i < min(10, len(lines)); i++ {
		line := strings.TrimSpace(lines[i])
		if len(line) > 5 && len(line) < 100 {
			// Check if it looks like a company name (has letters, possibly GmbH, etc.)
			if matched, _ := regexp.MatchString(`(?i)(gmbh|ag|kg|ltd|inc|corp)`, line); matched {
				return line
			}
		}
	}

	// Fallback: first substantial line
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if len(line) > 5 && len(line) < 100 {
			return line
		}
	}

	return ""
}

// extractInvoiceNumber extracts the invoice number.
func (e *LocalExtractor) extractInvoiceNumber(text string) string {
	patterns := []string{
		`(?i)rechnungsnr[.:\s]+([A-Z0-9\-/]+)`,
		`(?i)rechnung\s+nr[.:\s]+([A-Z0-9\-/]+)`,
		`(?i)invoice\s+no[.:\s]+([A-Z0-9\-/]+)`,
		`(?i)invoice\s+number[.:\s]+([A-Z0-9\-/]+)`,
		`(?i)re[-\s]?([0-9]{4,})`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		if matches := re.FindStringSubmatch(text); len(matches) > 1 {
			return strings.TrimSpace(matches[1])
		}
	}

	return ""
}

// extractDate extracts the invoice date and returns it in YYYY-MM-DD format.
func (e *LocalExtractor) extractDate(text string) string {
	// Patterns for German dates (dd.MM.yyyy or dd.MM.yy)
	germanDatePatterns := []string{
		`(?i)rechnungsdatum[:\s]+(\d{1,2})\.(\d{1,2})\.(\d{4})`,
		`(?i)datum[:\s]+(\d{1,2})\.(\d{1,2})\.(\d{4})`,
		`(\d{1,2})\.(\d{1,2})\.(\d{4})`,
	}

	for _, pattern := range germanDatePatterns {
		re := regexp.MustCompile(pattern)
		if matches := re.FindStringSubmatch(text); len(matches) > 3 {
			day := pad(matches[1], 2)
			month := pad(matches[2], 2)
			year := matches[3]
			return year + "-" + month + "-" + day
		}
	}

	// ISO date (YYYY-MM-DD)
	isoPattern := `(\d{4})-(\d{2})-(\d{2})`
	re := regexp.MustCompile(isoPattern)
	if matches := re.FindStringSubmatch(text); len(matches) > 3 {
		return matches[0]
	}

	return ""
}

// extractAmounts extracts gross, net, vat amount, and vat percent.
func (e *LocalExtractor) extractAmounts(text string) (gross, net, vat, vatPercent float64) {
	// Look for gross amount (Gesamt, Brutto, Total)
	grossPatterns := []string{
		`(?i)gesamt[:\s]+([\d.,]+)`,
		`(?i)brutto[:\s]+([\d.,]+)`,
		`(?i)total[:\s]+([\d.,]+)`,
		`(?i)rechnungsbetrag[:\s]+([\d.,]+)`,
	}

	for _, pattern := range grossPatterns {
		re := regexp.MustCompile(pattern)
		if matches := re.FindStringSubmatch(text); len(matches) > 1 {
			gross = parseAmount(matches[1])
			break
		}
	}

	// Look for net amount (Netto)
	netPatterns := []string{
		`(?i)netto[:\s]+([\d.,]+)`,
		`(?i)net[:\s]+([\d.,]+)`,
		`(?i)summe\s+netto[:\s]+([\d.,]+)`,
	}

	for _, pattern := range netPatterns {
		re := regexp.MustCompile(pattern)
		if matches := re.FindStringSubmatch(text); len(matches) > 1 {
			net = parseAmount(matches[1])
			break
		}
	}

	// Look for VAT (USt, MwSt)
	vatPatterns := []string{
		`(?i)ust\s+(\d+)%[:\s]+([\d.,]+)`,
		`(?i)mwst\s+(\d+)%[:\s]+([\d.,]+)`,
		`(?i)vat\s+(\d+)%[:\s]+([\d.,]+)`,
		`(\d+)%[:\s]+([\d.,]+)`,
	}

	for _, pattern := range vatPatterns {
		re := regexp.MustCompile(pattern)
		if matches := re.FindStringSubmatch(text); len(matches) > 2 {
			vatPercent = parseAmount(matches[1])
			vat = parseAmount(matches[2])
			break
		}
	}

	// If we have gross and vat but not net, calculate it
	if gross > 0 && vat > 0 && net == 0 {
		net = gross - vat
	}

	// If we have net and vat but not gross, calculate it
	if net > 0 && vat > 0 && gross == 0 {
		gross = net + vat
	}

	// If we have net and gross but not vat, calculate it
	if net > 0 && gross > 0 && vat == 0 {
		vat = gross - net
		if net > 0 {
			vatPercent = (vat / net) * 100
		}
	}

	return
}

// extractCurrency extracts the currency from the text.
func (e *LocalExtractor) extractCurrency(text string) string {
	if strings.Contains(text, "€") || strings.Contains(text, "EUR") {
		return "EUR"
	}
	if strings.Contains(text, "$") || strings.Contains(text, "USD") {
		return "USD"
	}
	return "EUR" // default
}

// generateShortDesc generates a short description based on available data.
func (e *LocalExtractor) generateShortDesc(meta Meta, text string) string {
	// Try to find keywords like "Wartung", "Lizenz", "Abo", etc.
	keywords := []string{"wartung", "lizenz", "abo", "subscription", "service", "beratung"}
	lowerText := strings.ToLower(text)

	for _, kw := range keywords {
		if strings.Contains(lowerText, kw) {
			// Create description with keyword and month
			if meta.Monat != "" {
				monthName := getMonthName(meta.Monat)
				return strings.Title(kw) + " " + monthName + " " + meta.Jahr
			}
			return strings.Title(kw)
		}
	}

	// Fallback: use company name or "Rechnung"
	if meta.Firmenname != "" {
		return "Rechnung " + meta.Firmenname
	}

	return "Rechnung"
}

// parseAmount parses a string amount (handles both , and . as decimal separator).
func parseAmount(s string) float64 {
	s = strings.TrimSpace(s)
	// Remove thousands separators
	s = strings.ReplaceAll(s, "'", "")
	s = strings.ReplaceAll(s, " ", "")

	// Determine decimal separator
	// If both , and . are present, the last one is the decimal separator
	commaIdx := strings.LastIndex(s, ",")
	dotIdx := strings.LastIndex(s, ".")

	if commaIdx > dotIdx {
		// Comma is decimal separator
		s = strings.ReplaceAll(s, ".", "")  // Remove thousands separator
		s = strings.Replace(s, ",", ".", 1) // Replace decimal separator
	} else {
		// Dot is decimal separator (or no separator)
		s = strings.ReplaceAll(s, ",", "") // Remove thousands separator
	}

	// Parse
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return math.Round(f*100) / 100 // Round to 2 decimals
}

// pad pads a string with leading zeros.
func pad(s string, length int) string {
	for len(s) < length {
		s = "0" + s
	}
	return s
}

// min returns the minimum of two integers.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// getMonthName returns the German month name for a month number (01-12).
func getMonthName(month string) string {
	monthMap := map[string]string{
		"01": "Januar", "02": "Februar", "03": "März",
		"04": "April", "05": "Mai", "06": "Juni",
		"07": "Juli", "08": "August", "09": "September",
		"10": "Oktober", "11": "November", "12": "Dezember",
	}
	if name, ok := monthMap[month]; ok {
		return name
	}
	return month
}
