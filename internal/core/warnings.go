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

	// §13b reverse-charge: a 0%-VAT expense from a likely foreign supplier should
	// have a §13b booking (Vorsteuer-RC 1577/1407, Umsatzsteuer-RC 1787/3837).
	// Fire when: incoming invoice, 0% VAT, positive amount, no §13b account in
	// the booking, AND a foreign-supplier signal (non-EUR currency OR non-DE EU
	// VAT-ID on the supplier). Suppressed for tax-exempt financial services
	// (§ 4 Nr. 8 UStG, e.g. foreign bank/card fees) — there §13b does not apply;
	// the signal is the expense being booked to a bank-charges account.
	if !row.Ausgangsrechnung && row.SteuersatzBetrag == 0 && row.Bruttobetrag > 0 &&
		!isFinancialChargeBooking(row) {
		has13b := false
		for _, e := range row.Buchung.Entries {
			if e.Konto == 1577 || e.Konto == 1787 || e.Konto == 1407 || e.Konto == 3837 {
				has13b = true
				break
			}
		}
		if !has13b {
			foreignSignal := (row.Waehrung != "" && row.Waehrung != "EUR") ||
				(IsEUVatID(row.VATID) && !strings.HasPrefix(strings.ToUpper(strings.TrimSpace(row.VATID)), "DE"))
			if foreignSignal {
				w = append(w, "0 % USt ohne Reverse-Charge — bei ausländischem Lieferant §13b (Kz 46/47) prüfen")
			}
		}
	}

	// Bewirtung (entertainment) booked without the 70/30 split: an entry on the
	// deductible Bewirtung account (SKR03 4650 / SKR04 6640) with NO matching
	// non-deductible entry (4654 / 6644) means the cost was treated as 100 %
	// deductible — but § 4 Abs. 5 Nr. 2 EStG allows only 70 %.
	hasBewAbz, hasBewNicht := false, false
	for _, e := range row.Buchung.Entries {
		switch e.Konto {
		case 4650, 6640:
			hasBewAbz = true
		case 4654, 6644:
			hasBewNicht = true
		}
	}
	if hasBewAbz && !hasBewNicht {
		w = append(w, "Bewirtung ohne 70/30-Aufteilung — Kategorie \"Bewirtung\" wählen (nur 70 % abziehbar, § 4 Abs. 5 EStG)")
	}

	// Bewirtung needs Anlass + Teilnehmer (§ 4 Abs. 5 Nr. 2 EStG). Accept either
	// electronic entry of BOTH fields OR the flag that they are handwritten on
	// the receipt/attachment. Only nag for an actual Bewirtung booking.
	if hasBewAbz && !row.BewirtungAngabenAufBeleg &&
		(strings.TrimSpace(row.BewirtungAnlass) == "" || strings.TrimSpace(row.BewirtungTeilnehmer) == "") {
		w = append(w, "Bewirtung: Anlass und Teilnehmer fehlen (§ 4 Abs. 5 EStG) — elektronisch eintragen oder als \"handschriftlich auf Beleg\" markieren")
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

// isFinancialChargeBooking reports whether an expense is booked as a financial
// service / bank charge — "Nebenkosten des Geldverkehrs" (SKR03 4970, SKR04
// 6855). Such services are VAT-exempt under § 4 Nr. 8 UStG, so the §13b
// reverse-charge nudge would be a false positive for a foreign bank's fees.
// Checks the Gegenkonto and any Soll entry of the booking.
func isFinancialChargeBooking(row CSVRow) bool {
	isCharge := func(konto int) bool { return konto == 4970 || konto == 6855 }
	if isCharge(row.Gegenkonto) {
		return true
	}
	for _, e := range row.Buchung.Entries {
		if e.Soll && isCharge(e.Konto) {
			return true
		}
	}
	return false
}

// InvoiceWarnings returns advisory (non-blocking) plausibility warnings for an
// invoice row.
func InvoiceWarnings(row CSVRow) []string {
	return InvoiceWarningsAsOf(row, time.Now())
}
