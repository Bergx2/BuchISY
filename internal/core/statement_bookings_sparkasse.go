package core

import (
	"regexp"
	"strings"
)

// Sparkasse "Umsätze - Druckansicht" is the online-banking print export (not the
// classic account statement). It uses a columnar BUCHUNG/WERTSTELLUNG layout the
// generic date-prefix heuristic mis-reads: every booking spans two date-bearing
// rows (a merged "DD.MM.YYYYDD.MM.YYYY" cell plus a "DD.MM.YYYY | Wertstellung …"
// row) and the page footer carries a print timestamp — so the heuristic reports
// 2×N+1 phantom zero-amount rows instead of N bookings. The real transaction
// amount sits on the label row (e.g. "SOLIDARITAETSZUSCHLAG" / "-0,73 EUR"),
// never on a date row.

// druckansichtSignedEURRe matches a transaction amount with an explicit leading
// sign, e.g. "-0,73 EUR" or "+53,11 EUR". Balances are printed WITHOUT a sign
// ("14.577,76 EUR *"), so requiring the sign cleanly selects only transactions.
var druckansichtSignedEURRe = regexp.MustCompile(`^([+-])\s*\d{1,3}(?:\.\d{3})*,\d{2}\s*EUR`)

// isSparkasseDruckansicht reports whether the plain text looks like a Sparkasse
// "Umsätze - Druckansicht" export.
func isSparkasseDruckansicht(fullText string) bool {
	return strings.Contains(fullText, "Umsätze - Druckansicht") &&
		strings.Contains(fullText, "BUCHUNG") && strings.Contains(fullText, "WERTSTELLUNG")
}

// parseSparkasseDruckansicht turns the positioned runs of a Druckansicht export
// into one StatementBooking per transaction. Each transaction is anchored on its
// signed EUR amount; the label (transaction type) and booking date are the
// nearest left-column / date runs in the same horizontal band.
func parseSparkasseDruckansicht(pageLines [][]htmlLine) []StatementBooking {
	var out []StatementBooking
	idx := 0
	for page, lines := range pageLines {
		for _, amt := range lines {
			m := druckansichtSignedEURRe.FindStringSubmatch(amt.text)
			if m == nil {
				continue
			}
			betrag := ParseLineAmount(amt.text)
			if betrag == 0 {
				continue
			}
			idx++

			// Booking date: the first DD.MM.YYYY in a date run of the same band
			// (the merged "…2025…2026" cell or the "… | Wertstellung …" row). The
			// leading date is the Buchungstag; the trailing one is the Wertstellung.
			date := ""
			for _, ln := range lines {
				if ln.top < amt.top-6 || ln.top > amt.top+14 {
					continue
				}
				if dm := dateLineRe.FindString(ln.text); dm != "" {
					date = strings.TrimSpace(dm)
					break
				}
			}

			// Label: the left-column, non-date run on the amount's own row.
			label := ""
			for _, ln := range lines {
				if ln.left > 200 || ln.top < amt.top-8 || ln.top > amt.top+2 {
					continue
				}
				if dateLineRe.MatchString(ln.text) {
					continue
				}
				label = strings.TrimSpace(ln.text)
				break
			}

			text := label
			if date != "" {
				text = strings.TrimSpace(date + " " + label)
			}
			out = append(out, StatementBooking{
				Page:          page,
				LineIdx:       idx,
				Date:          date,
				TopPt:         amt.top,
				LeftPt:        amt.left,
				Text:          text,
				Betrag:        betrag,
				IstGutschrift: m[1] == "+",
			})
		}
	}
	return out
}
