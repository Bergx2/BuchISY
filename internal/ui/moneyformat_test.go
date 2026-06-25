package ui

import "testing"

func TestFormatMoney(t *testing.T) {
	cases := []struct {
		amount   float64
		currency string
		sep      string
		want     string
	}{
		{170.65, "EUR", ",", "EUR 170,65"},
		{200.00, "USD", ",", "USD 200,00"},
		{1234.5, "EUR", ",", "EUR 1.234,50"}, // formatDecimal adds German thousands grouping
		{170.65, "", ",", "EUR 170,65"},   // empty currency → EUR
		{99.9, "GBP", ".", "GBP 99.90"},   // dot separator
		{-5.25, "EUR", ",", "EUR -5,25"},  // negative
		{170.65, " usd ", ",", "usd 170,65"}, // trimmed, code passed through
	}
	for _, c := range cases {
		got := formatMoney(c.amount, c.currency, c.sep)
		if got != c.want {
			t.Errorf("formatMoney(%v,%q,%q) = %q, want %q", c.amount, c.currency, c.sep, got, c.want)
		}
	}
}
