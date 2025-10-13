package core

import "math"

// IsDuplicate checks if a row is a duplicate of any existing row.
// A row is considered a duplicate if ALL of the following fields match:
// - Firmenname (normalized)
// - Rechnungsnummer
// - Rechnungsdatum
// - Bruttobetrag (approximately equal)
// - Teilzahlung
func IsDuplicate(existingRows []CSVRow, newRow CSVRow) bool {
	for _, existing := range existingRows {
		// All fields must match for it to be a duplicate
		if NormalizeCompanyName(existing.Firmenname) == NormalizeCompanyName(newRow.Firmenname) &&
			existing.Rechnungsnummer == newRow.Rechnungsnummer &&
			existing.Rechnungsdatum == newRow.Rechnungsdatum &&
			floatEquals(existing.Bruttobetrag, newRow.Bruttobetrag) &&
			existing.Teilzahlung == newRow.Teilzahlung {
			return true
		}
	}

	return false
}

// floatEquals checks if two floats are approximately equal (within 0.01).
func floatEquals(a, b float64) bool {
	return math.Abs(a-b) < 0.01
}

// CheckConsistency checks if Net + Tax ≈ Gross.
// Returns true if consistent, false if there's a significant discrepancy.
func CheckConsistency(meta Meta) bool {
	calculated := meta.BetragNetto + meta.SteuersatzBetrag
	return floatEquals(calculated, meta.Bruttobetrag)
}
