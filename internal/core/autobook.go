package core

import "math"

// AutobookPlausible reports whether a receipt is safe to book without manual
// review. All of the following must hold:
//   - at least one tax line is present
//   - Gegenkonto > 0
//   - Bruttobetrag > 0
//   - |Bruttobetrag − (SumNetto + SumMwSt + Trinkgeld)| ≤ 0.02
//   - NOT (foreign currency with Wechselkurs ≤ 0)
func AutobookPlausible(m Meta) bool {
	if len(m.TaxLines) == 0 {
		return false
	}
	if m.Gegenkonto <= 0 {
		return false
	}
	if m.Bruttobetrag <= 0 {
		return false
	}
	expected := SumNetto(m.TaxLines) + SumMwSt(m.TaxLines) + m.Trinkgeld
	if math.Abs(m.Bruttobetrag-expected) > 0.02 {
		return false
	}
	// Foreign currency without a valid exchange rate is not safe to auto-book.
	if m.Waehrung != "" && m.Waehrung != "EUR" && m.Wechselkurs <= 0 {
		return false
	}
	return true
}

// MatchAutobookRule returns the booking template for company iff one exists in
// the store AND its Autobook flag is true. Returns (zero, false) otherwise.
func MatchAutobookRule(company string, store *BookingTemplateStore) (BookingTemplate, bool) {
	tpl, ok := store.Get(company)
	if !ok || !tpl.Autobook {
		return BookingTemplate{}, false
	}
	return tpl, true
}
