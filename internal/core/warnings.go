package core

import (
	"math"
	"regexp"
	"strings"
	"time"
)

// vatIDRegex validates VAT-ID format: 2-letter country code + 6-14 alphanumeric chars.
var vatIDRegex = regexp.MustCompile(`^[A-Z]{2}[0-9A-Za-z]{6,14}$`)

// InvoiceWarningsAsOf returns advisory (non-blocking) plausibility warnings for an
// invoice row as of a given date.
func InvoiceWarningsAsOf(row CSVRow, today time.Time) []string {
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

	// Future date check
	if row.Rechnungsdatum != "" {
		if d, err := time.Parse("02.01.2006", row.Rechnungsdatum); err == nil && d.After(today) {
			w = append(w, "Rechnungsdatum liegt in der Zukunft")
		}
	}

	// Zero amount check
	if row.Bruttobetrag <= 0 {
		w = append(w, "Bruttobetrag fehlt oder ist 0")
	}

	// GWG account (Sofortabschreibung GWG: SKR03 4855, SKR04 6260) but net > 800 €:
	// over the GWG limit, so it is NOT a geringwertiges Wirtschaftsgut — it must be
	// capitalised as a fixed asset and depreciated (AfA), not written off at once.
	if (row.Gegenkonto == 4855 || row.Gegenkonto == 6260) && row.BetragNetto > 800.0 {
		w = append(w, "GWG-Konto, aber Netto > 800 € — kein GWG: als Anlagegut aktivieren und abschreiben (AfA)")
	}

	// VAT-ID format check
	if vatID := strings.TrimSpace(row.VATID); vatID != "" {
		// Normalize: remove spaces, uppercase
		normalized := strings.ToUpper(strings.ReplaceAll(vatID, " ", ""))
		if !vatIDRegex.MatchString(normalized) {
			w = append(w, "USt-IdNr hat ungültiges Format")
		}
	}

	return w
}

// InvoiceWarnings returns advisory (non-blocking) plausibility warnings for an
// invoice row.
func InvoiceWarnings(row CSVRow) []string {
	return InvoiceWarningsAsOf(row, time.Now())
}
