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
	// GWG account (4855) but net > 800 € → not a GWG, warn.
	gwgOver := CSVRow{BetragNetto: 1116.85, SteuersatzBetrag: 212.20, Bruttobetrag: 1329.05, Gegenkonto: 4855, Waehrung: "EUR"}
	if !hasWarn(InvoiceWarnings(gwgOver), "Netto > 800") {
		t.Error("expected a GWG-over-limit warning for net > 800 on account 4855")
	}
	// GWG account with net ≤ 800 € → fine, no warning.
	gwgOk := CSVRow{BetragNetto: 165.55, SteuersatzBetrag: 31.45, Bruttobetrag: 197.00, Gegenkonto: 4855, Waehrung: "EUR"}
	if hasWarn(InvoiceWarnings(gwgOk), "Netto > 800") {
		t.Error("must not warn for a genuine GWG (net ≤ 800)")
	}
	// Bewirtung booked 100% to 4650 (no 4654 split) → warn.
	bewUnsplit := CSVRow{BetragNetto: 21.97, SteuersatzBetrag: 2.43, Bruttobetrag: 27.00, Gegenkonto: 4650, Waehrung: "EUR",
		Buchung: Booking{Entries: []BookingEntry{{Konto: 4650, Betrag: 24.57, Soll: true}, {Konto: 1576, Betrag: 2.43, Soll: true}, {Konto: 1000, Betrag: 27.00, Soll: false}}}}
	if !hasWarn(InvoiceWarnings(bewUnsplit), "70/30") {
		t.Error("expected a Bewirtung-70/30 warning when 4654 split is missing")
	}
	// Bewirtung correctly split (4650 + 4654) → no warning.
	bewSplit := bewUnsplit
	bewSplit.Buchung = Booking{Entries: []BookingEntry{{Konto: 4650, Betrag: 17.20, Soll: true}, {Konto: 4654, Betrag: 7.37, Soll: true}, {Konto: 1576, Betrag: 2.43, Soll: true}, {Konto: 1000, Betrag: 27.00, Soll: false}}}
	if hasWarn(InvoiceWarnings(bewSplit), "70/30") {
		t.Error("must not warn when Bewirtung is correctly split 70/30")
	}
}

func TestInvoiceWarnings13b(t *testing.T) {
	// USD 0%-VAT expense with no §13b booking → warns.
	rc13bMissing := CSVRow{
		BetragNetto: 500, SteuersatzBetrag: 0, Bruttobetrag: 500,
		Gegenkonto: 6815, Waehrung: "USD",
		Buchung: Booking{Entries: []BookingEntry{
			{Konto: 6815, Betrag: 500, Soll: true},
			{Konto: 1200, Betrag: 500, Soll: false},
		}},
	}
	if !hasWarn(InvoiceWarnings(rc13bMissing), "§13b") && !hasWarn(InvoiceWarnings(rc13bMissing), "Reverse-Charge") {
		t.Error("expected §13b/Reverse-Charge warning for USD 0%-VAT expense without §13b booking")
	}

	// Same but booking already has 1577+1787 entries → no warn.
	rc13bBooked := rc13bMissing
	rc13bBooked.Buchung = Booking{Entries: []BookingEntry{
		{Konto: 6815, Betrag: 500, Soll: true},
		{Konto: 1577, Betrag: 95, Soll: true},
		{Konto: 1787, Betrag: 95, Soll: false},
		{Konto: 1200, Betrag: 500, Soll: false},
	}}
	if hasWarn(InvoiceWarnings(rc13bBooked), "§13b") || hasWarn(InvoiceWarnings(rc13bBooked), "Reverse-Charge") {
		t.Error("must not warn when §13b accounts (1577/1787) are already in the booking")
	}

	// Domestic EUR 0% expense (e.g. Gegenkonto 4138, no VAT-ID) → no warn.
	rc13bDomestic := CSVRow{
		BetragNetto: 200, SteuersatzBetrag: 0, Bruttobetrag: 200,
		Gegenkonto: 4138, Waehrung: "EUR",
		Buchung: Booking{Entries: []BookingEntry{
			{Konto: 4138, Betrag: 200, Soll: true},
			{Konto: 1200, Betrag: 200, Soll: false},
		}},
	}
	if hasWarn(InvoiceWarnings(rc13bDomestic), "§13b") || hasWarn(InvoiceWarnings(rc13bDomestic), "Reverse-Charge") {
		t.Error("must not warn for a domestic EUR 0%-VAT expense without foreign signal")
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
