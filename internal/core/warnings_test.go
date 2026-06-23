package core

import (
	"strings"
	"testing"
	"time"
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
	// Outgoing, 0% VAT, no customer VAT-ID → ZM-gap warning.
	zmGap := CSVRow{Ausgangsrechnung: true, BetragNetto: 1000, SteuersatzBetrag: 0, Bruttobetrag: 1000, Gegenkonto: 8341, Waehrung: "EUR"}
	if !hasWarn(InvoiceWarnings(zmGap), "ZM-Eintrag") {
		t.Error("expected a missing-VAT-ID ZM warning for outgoing 0%% invoice")
	}
	// But WITH a customer VAT-ID → no ZM warning.
	zmOk := zmGap
	zmOk.VATID = "FI26378052"
	if hasWarn(InvoiceWarnings(zmOk), "ZM-Eintrag") {
		t.Error("must not warn when the customer VAT-ID is present")
	}
}

func TestInvoiceWarningsAsOf(t *testing.T) {
	today := time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC)
	future := CSVRow{Rechnungsdatum: "01.08.2026", Bruttobetrag: 119, BetragNetto: 100, SteuersatzBetrag: 19, Gegenkonto: 4980, Waehrung: "EUR"}
	if !hasWarn(InvoiceWarningsAsOf(future, today), "Zukunft") {
		t.Error("expected future-date warning")
	}
	zero := CSVRow{Rechnungsdatum: "01.06.2026", Bruttobetrag: 0, Gegenkonto: 4980, Waehrung: "EUR"}
	if !hasWarn(InvoiceWarningsAsOf(zero, today), "Bruttobetrag") {
		t.Error("expected zero-amount warning")
	}
	badVat := CSVRow{Rechnungsdatum: "01.06.2026", Bruttobetrag: 119, BetragNetto: 100, SteuersatzBetrag: 19, Gegenkonto: 4980, Waehrung: "EUR", VATID: "12345"}
	if !hasWarn(InvoiceWarningsAsOf(badVat, today), "USt-IdNr") {
		t.Error("expected invalid VAT-ID format warning")
	}
	ok := CSVRow{Rechnungsdatum: "01.06.2026", Bruttobetrag: 119, BetragNetto: 100, SteuersatzBetrag: 19, Gegenkonto: 4980, Waehrung: "EUR", VATID: "DE287472874"}
	for _, w := range InvoiceWarningsAsOf(ok, today) {
		if strings.Contains(w, "Zukunft") || strings.Contains(w, "USt-IdNr") || strings.Contains(w, "Bruttobetrag") {
			t.Errorf("clean invoice should not warn: %q", w)
		}
	}
}
