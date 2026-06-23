package core

import (
	"math"
	"strings"
)

// InvoiceWarnings returns advisory (non-blocking) plausibility warnings for an
// invoice row.
func InvoiceWarnings(row CSVRow) []string {
	var w []string
	if row.Bruttobetrag > 0 {
		expected := row.BetragNetto + row.SteuersatzBetrag + row.Trinkgeld
		if math.Abs(row.Bruttobetrag-expected) > 0.02 {
			w = append(w, "Brutto stimmt nicht mit Netto + MwSt + Trinkgeld überein")
		}
	}
	if row.Gegenkonto == 0 {
		w = append(w, "Kein Gegenkonto gewählt")
	}
	if row.Waehrung != "" && row.Waehrung != "EUR" && row.Wechselkurs <= 0 {
		w = append(w, "Fremdwährung ohne Wechselkurs")
	}
	// Outgoing invoice with no VAT and no customer VAT-ID: an intra-EU
	// reverse-charge supply needs the customer's USt-IdNr for the
	// Zusammenfassende Meldung (and Kz 21). Harmless for genuine third-country
	// (Drittland) supplies — hence advisory.
	if row.Ausgangsrechnung && row.SteuersatzBetrag == 0 && strings.TrimSpace(row.VATID) == "" {
		w = append(w, "Ausgangsrechnung ohne USt und ohne Kunden-USt-IdNr — bei EU-Kunden fehlt sonst der ZM-Eintrag (bei Drittland/Schweiz ok)")
	}
	return w
}
