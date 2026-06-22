package core

import "testing"

func TestConvertForeignPayment(t *testing.T) {
	// USD 89.18 gross at 1.1583 USD/EUR → 76.99 EUR; 2% fee → 1.54; total 78.53.
	c := ConvertForeignPayment(89.18, 74.94, 1.1583, 2)
	if !almost(c.BruttoEUR, 76.99) {
		t.Errorf("BruttoEUR = %v, want 76.99", c.BruttoEUR)
	}
	if !almost(c.GebuehrEUR, 1.54) {
		t.Errorf("GebuehrEUR = %v, want 1.54", c.GebuehrEUR)
	}
	if !almost(c.GesamtEUR, 78.53) {
		t.Errorf("GesamtEUR = %v, want 78.53", c.GesamtEUR)
	}
	// kurs 0 → no divide-by-zero, all zero
	if z := ConvertForeignPayment(89.18, 74.94, 0, 2); z.BruttoEUR != 0 || z.GesamtEUR != 0 {
		t.Errorf("kurs 0 should yield zero EUR: %+v", z)
	}
}
