package core

import (
	"fmt"
	"strings"
)

// BuildLexwareCSV renders the bookings of rows as a simple Lexware import CSV
// (semicolon-separated). Returns the bytes, rows written, invoices skipped.
func BuildLexwareCSV(rows []CSVRow) ([]byte, int, int) {
	var b strings.Builder
	b.WriteString("Datum;Belegnr;Buchungstext;Betrag;Sollkonto;Habenkonto\r\n")
	exported, skipped := 0, 0
	for _, r := range rows {
		pay, ok := r.Buchung.PaymentEntry()
		if !r.Buchung.Balanced() || !ok {
			skipped++
			continue
		}
		text := lexClean(strings.TrimSpace(r.Auftraggeber + " " + r.Verwendungszweck))
		// Prefer the internal sequential receipt number; fall back to the supplier
		// invoice number for rows that predate the Belegnummer feature.
		belegRef := r.Belegnummer
		if belegRef == "" {
			belegRef = r.Rechnungsnummer
		}
		beleg := lexClean(belegRef)
		for _, e := range r.Buchung.DebitEntries() {
			amount := strings.Replace(fmt.Sprintf("%.2f", e.Betrag), ".", ",", 1)
			b.WriteString(fmt.Sprintf("%s;%s;%s;%s;%d;%d\r\n",
				r.Rechnungsdatum, beleg, text, amount, e.Konto, pay.Konto))
			exported++
		}
	}
	return []byte(b.String()), exported, skipped
}

// lexClean strips the field separator and newlines from a free-text field.
func lexClean(s string) string {
	s = strings.ReplaceAll(s, ";", ",")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}
