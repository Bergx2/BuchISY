package core

import (
	"regexp"
	"strconv"
	"strings"
)

// qontoPeriodRe extracts the year from the Qonto period header line.
// Example: "Vom 01/04/2026 bis zum 30/04/2026"
var qontoPeriodRe = regexp.MustCompile(`Vom\s+\d{2}/\d{2}/(\d{4})`)

// qontoDateLineRe matches lines that start a new Qonto transaction.
// Example: "01/04 Qonto" or "23/04 SW Operations GmbH"
var qontoDateLineRe = regexp.MustCompile(`^(\d{2})/(\d{2})\b(.*)`)

// qontoAmountRe matches a standalone EUR amount line like "- 108.00 EUR" or "+ 75404.05 EUR".
// The amount uses dot-decimal (plain English format from Qonto).
var qontoAmountRe = regexp.MustCompile(`^\s*([-+])\s*([\d.,]+)\s+EUR\s*$`)

// qontoUSDLineRe matches lines containing a USD amount (to be skipped).
// Matches both "- 100.00 USD" and "1.15220647540039 USD = 1.00 EUR" style lines.
var qontoUSDLineRe = regexp.MustCompile(`USD`)

// qontoSkipLineRe matches header/footer lines that should be ignored entirely.
var qontoSkipLineRe = regexp.MustCompile(`^(Kontostand|Eingänge|Ausgänge|Abrechnungstag|Kontoauszüge)`)

// parseQontoStatement parses the plain-text content of a Qonto PDF bank statement
// and returns a slice of StatementBooking. Each transaction starts with a DD/MM date
// line; the first standalone EUR amount line sets the Betrag. USD lines are ignored.
func parseQontoStatement(text string) []StatementBooking {
	// Extract the year from the period header.
	year := ""
	if m := qontoPeriodRe.FindStringSubmatch(text); m != nil {
		year = m[1]
	}

	lines := strings.Split(text, "\n")

	type pending struct {
		day    string
		month  string
		text   string
		betrag float64
		credit bool
		hasAmt bool
		lineIdx int
	}

	var result []StatementBooking
	var cur *pending
	lineIdx := 0

	finalize := func() {
		if cur != nil && cur.hasAmt {
			date := cur.day + "." + cur.month + "."
			if year != "" {
				date = cur.day + "." + cur.month + "." + year
			}
			result = append(result, StatementBooking{
				Page:          0,
				LineIdx:       cur.lineIdx,
				Date:          date,
				Text:          cur.text,
				Betrag:        cur.betrag,
				IstGutschrift: cur.credit,
			})
		}
		cur = nil
	}

	for _, rawLine := range lines {
		line := strings.TrimRight(rawLine, "\r")

		// Skip header/summary lines.
		if qontoSkipLineRe.MatchString(strings.TrimSpace(line)) {
			continue
		}

		// Check if this is a new transaction date line.
		if m := qontoDateLineRe.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			finalize()
			lineIdx++
			desc := strings.TrimSpace(m[3])
			cur = &pending{
				day:     m[1],
				month:   m[2],
				text:    desc,
				lineIdx: lineIdx,
			}
			continue
		}

		// If no transaction is open, nothing else to do.
		if cur == nil {
			continue
		}

		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Skip USD-related lines (raw USD amounts and exchange rate lines).
		if qontoUSDLineRe.MatchString(trimmed) {
			continue
		}

		// Check if this is a standalone EUR amount line.
		if m := qontoAmountRe.FindStringSubmatch(trimmed); m != nil {
			if !cur.hasAmt {
				// Parse the amount — Qonto uses dot-decimal (e.g. "75404.05")
				// but also possibly German thousands (e.g. "1.793,68").
				amt := parseQontoAmount(m[2])
				cur.betrag = amt
				cur.credit = m[1] == "+"
				cur.hasAmt = true
			}
			continue
		}

		// Append remaining non-empty, non-USD, non-amount lines as description.
		if cur.text == "" {
			cur.text = trimmed
		} else {
			cur.text += " / " + trimmed
		}
	}

	finalize()
	return result
}

// parseQontoAmount parses a numeric string that may be either:
//   - plain dot-decimal: "75404.05", "108.00"
//   - German format with dot-thousands and comma-decimal: "1.793,68"
func parseQontoAmount(s string) float64 {
	// If there's a comma, treat as German format: remove dot thousands, replace comma with dot.
	if strings.Contains(s, ",") {
		s = strings.ReplaceAll(s, ".", "")
		s = strings.ReplaceAll(s, ",", ".")
	}
	// Otherwise it's already dot-decimal; parse directly.
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}
