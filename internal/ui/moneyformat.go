package ui

import "strings"

// formatMoney renders an amount as "<CODE> <number>", e.g. "EUR 170,65" or
// "USD 200,00" — the ISO currency code BEFORE the amount, no symbol. An empty
// currency defaults to EUR. sep is the decimal separator ("," or ".").
func formatMoney(amount float64, currency, sep string) string {
	code := strings.TrimSpace(currency)
	if code == "" {
		code = "EUR"
	}
	return code + " " + formatDecimal(amount, sep)
}
