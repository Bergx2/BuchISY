package core

// isForeign reports whether a row represents a foreign-currency invoice.
// A row is foreign iff Waehrung is non-empty and not "EUR".
func isForeign(waehrung string) bool {
	return waehrung != "" && waehrung != "EUR"
}

// RowEUR returns a copy of row with all money fields converted to EUR
// (Waehrung "EUR", Wechselkurs 0). rateMissing is true for a foreign row
// without a valid rate (Wechselkurs <= 0): amounts are left at face value.
// EUR rows (or rows with no currency) are returned unchanged.
func RowEUR(row CSVRow) (eur CSVRow, rateMissing bool) {
	if !isForeign(row.Waehrung) {
		// Already EUR (or blank currency — treat as EUR)
		return row, false
	}

	if row.Wechselkurs <= 0 {
		// Foreign, but rate is missing — pass through at face value
		return row, true
	}

	kurs := row.Wechselkurs
	eur = row
	// Invoice amounts are in the foreign currency → convert to EUR.
	eur.BetragNetto = round2(row.BetragNetto / kurs)
	eur.SteuersatzBetrag = round2(row.SteuersatzBetrag / kurs)
	eur.Bruttobetrag = round2(row.Bruttobetrag / kurs)
	eur.Trinkgeld = round2(row.Trinkgeld / kurs)
	eur.Rabatt = round2(row.Rabatt / kurs)
	// Tax lines drive UStVA/ZM — must be EUR too (own slice, original untouched).
	eur.TaxLines = TaxLinesEUR(row.TaxLines, row.Waehrung, kurs)
	// BetragNetto_EUR IS the EUR net (already EUR) → equals the converted net.
	eur.BetragNetto_EUR = eur.BetragNetto
	// Gebuehr (bank/CC FX fee) is already booked in EUR → keep as-is (NOT divided).
	eur.Waehrung = "EUR"
	eur.Wechselkurs = 0
	return eur, false
}

// RowsEUR maps RowEUR over a slice. Rate-missing rows pass through at face value.
func RowsEUR(rows []CSVRow) []CSVRow {
	result := make([]CSVRow, len(rows))
	for i, r := range rows {
		converted, _ := RowEUR(r)
		result[i] = converted
	}
	return result
}

// TaxLinesEUR converts each line's Netto + MwStBetrag to EUR (round2) for a
// foreign currency with a rate. Returns lines unchanged for EUR / empty currency /
// no rate (kurs <= 0). SatzProzent is never modified.
func TaxLinesEUR(lines []TaxLine, waehrung string, kurs float64) []TaxLine {
	if !isForeign(waehrung) || kurs <= 0 {
		return lines
	}

	result := make([]TaxLine, len(lines))
	for i, l := range lines {
		result[i] = TaxLine{
			Netto:       round2(l.Netto / kurs),
			SatzProzent: l.SatzProzent,
			MwStBetrag:  round2(l.MwStBetrag / kurs),
		}
	}
	return result
}
