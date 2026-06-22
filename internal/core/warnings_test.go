package core

import (
	"strings"
	"testing"
)

func hasWarn(ws []string, sub string) bool {
	for _, w := range ws {
		if strings.Contains(w, sub) {
			return true
		}
	}
	return false
}

func TestInvoiceWarnings(t *testing.T) {
	good := CSVRow{BetragNetto: 100, SteuersatzBetrag: 19, Bruttobetrag: 119, Gegenkonto: 6815, Waehrung: "EUR"}
	if w := InvoiceWarnings(good); len(w) != 0 {
		t.Fatalf("expected no warnings, got %v", w)
	}
	mismatch := CSVRow{BetragNetto: 100, SteuersatzBetrag: 19, Bruttobetrag: 200, Gegenkonto: 6815, Waehrung: "EUR"}
	if !hasWarn(InvoiceWarnings(mismatch), "Brutto") {
		t.Error("expected a gross-mismatch warning")
	}
	noAccount := CSVRow{BetragNetto: 100, SteuersatzBetrag: 19, Bruttobetrag: 119, Gegenkonto: 0, Waehrung: "EUR"}
	if !hasWarn(InvoiceWarnings(noAccount), "Gegenkonto") {
		t.Error("expected a missing-account warning")
	}
	fxNoRate := CSVRow{BetragNetto: 100, SteuersatzBetrag: 19, Bruttobetrag: 119, Gegenkonto: 6815, Waehrung: "USD", Wechselkurs: 0}
	if !hasWarn(InvoiceWarnings(fxNoRate), "Wechselkurs") {
		t.Error("expected a foreign-without-rate warning")
	}
}
