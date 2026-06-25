package db

import (
	"math"
	"strings"
	"testing"

	"github.com/bergx2/buchisy/internal/core"
)

// bookingUSD builds a minimal incoming-invoice booking (Haben = payment) in USD.
// gross is the foreign-currency gross.  paymentKonto is the Haben account.
func bookingUSD(gross, net, vat float64, paymentKonto int) core.Booking {
	return core.Booking{Entries: []core.BookingEntry{
		{Konto: 4920, Betrag: net, Soll: true},  // expense (Soll)
		{Konto: 1406, Betrag: vat, Soll: true},  // Vorsteuer (Soll)
		{Konto: paymentKonto, Betrag: gross, Soll: false}, // Zahlungskonto (Haben)
	}}
}

// nearlyEqual reports whether a and b differ by less than 0.005.
func nearlyEqual(a, b float64) bool {
	return math.Abs(a-b) < 0.005
}

// TestRebookForeignToEUR_ConvertsForeignInvoice verifies that a foreign-currency
// invoice with a well-formed booking is rescaled to EUR.
func TestRebookForeignToEUR_ConvertsForeignInvoice(t *testing.T) {
	repo := newTestRepo(t)

	// USD invoice: 119 USD gross (net=100, vat=19), rate=1.19 → EUR gross = 100.00
	usdGross := 119.0
	rate := 1.19
	eurGross := round2(usdGross / rate) // ~100.00

	b := bookingUSD(usdGross, 100.0, 19.0, 1800)

	row := core.CSVRow{
		Dateiname:    "usd-invoice.pdf",
		Jahr:         "2026",
		Monat:        "01",
		Auftraggeber: "AWS LLC",
		Bruttobetrag: usdGross,
		Waehrung:     "USD",
		Wechselkurs:  rate,
		Buchung:      b,
		Belegnummer:  "2026-0001",
	}
	if _, err := repo.Insert(row); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	conv, skip, missing, err := repo.RebookForeignToEUR()
	if err != nil {
		t.Fatalf("RebookForeignToEUR: %v", err)
	}
	if conv != 1 || skip != 0 || missing != 0 {
		t.Errorf("got converted=%d skipped=%d rateMissing=%d, want 1/0/0", conv, skip, missing)
	}

	// Read back and check booking totals.
	rows, err := repo.List("2026", "01")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	got := rows[0].Buchung
	if !nearlyEqual(got.HabenSum(), eurGross) {
		t.Errorf("HabenSum after rebook = %.4f, want ~%.4f", got.HabenSum(), eurGross)
	}
	if !nearlyEqual(got.SollSum(), got.HabenSum()) {
		t.Errorf("booking not balanced: Soll=%.4f Haben=%.4f", got.SollSum(), got.HabenSum())
	}

	// Audit entry should exist.
	entries, err := repo.AuditLog(20)
	if err != nil {
		t.Fatalf("AuditLog: %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Aktion == "rebook-eur" && strings.Contains(e.Schluessel, "usd-invoice.pdf") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected a 'rebook-eur' audit entry for usd-invoice.pdf")
	}
}

// TestRebookForeignToEUR_SkipsAlreadyEUR verifies idempotency: a booking whose
// Haben total already equals the EUR gross (Bruttobetrag/Wechselkurs) is skipped.
func TestRebookForeignToEUR_SkipsAlreadyEUR(t *testing.T) {
	repo := newTestRepo(t)

	// Booking already expressed in EUR (Haben = 100.00 EUR), rate = 1.19.
	rate := 1.19
	usdGross := 119.0
	eurGross := round2(usdGross / rate) // 100.00

	// Build booking with EUR amounts already.
	bEUR := core.Booking{Entries: []core.BookingEntry{
		{Konto: 4920, Betrag: round2(100.0 / rate), Soll: true},
		{Konto: 1406, Betrag: round2(19.0 / rate), Soll: true},
		{Konto: 1800, Betrag: eurGross, Soll: false},
	}}

	row := core.CSVRow{
		Dateiname:    "already-eur.pdf",
		Jahr:         "2026",
		Monat:        "02",
		Auftraggeber: "Already EUR GmbH",
		Bruttobetrag: usdGross,
		Waehrung:     "USD",
		Wechselkurs:  rate,
		Buchung:      bEUR,
	}
	if _, err := repo.Insert(row); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	conv, skip, missing, err := repo.RebookForeignToEUR()
	if err != nil {
		t.Fatalf("RebookForeignToEUR: %v", err)
	}
	if conv != 0 || skip != 1 || missing != 0 {
		t.Errorf("got converted=%d skipped=%d rateMissing=%d, want 0/1/0", conv, skip, missing)
	}

	// Booking should be unchanged.
	rows, err := repo.List("2026", "02")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if !nearlyEqual(rows[0].Buchung.HabenSum(), eurGross) {
		t.Errorf("booking was modified unexpectedly; HabenSum=%.4f want %.4f", rows[0].Buchung.HabenSum(), eurGross)
	}
}

// TestRebookForeignToEUR_RateMissing verifies that foreign invoices without a
// rate (wechselkurs = 0) are counted in rateMissing and left untouched.
func TestRebookForeignToEUR_RateMissing(t *testing.T) {
	repo := newTestRepo(t)

	b := bookingUSD(119.0, 100.0, 19.0, 1800)

	row := core.CSVRow{
		Dateiname:    "no-rate.pdf",
		Jahr:         "2026",
		Monat:        "03",
		Auftraggeber: "No Rate LLC",
		Bruttobetrag: 119.0,
		Waehrung:     "USD",
		Wechselkurs:  0, // missing
		Buchung:      b,
	}
	if _, err := repo.Insert(row); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	conv, skip, missing, err := repo.RebookForeignToEUR()
	if err != nil {
		t.Fatalf("RebookForeignToEUR: %v", err)
	}
	if conv != 0 || skip != 0 || missing != 1 {
		t.Errorf("got converted=%d skipped=%d rateMissing=%d, want 0/0/1", conv, skip, missing)
	}

	// Booking is untouched.
	rows, err := repo.List("2026", "03")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if !nearlyEqual(rows[0].Buchung.HabenSum(), 119.0) {
		t.Errorf("booking was modified; HabenSum=%.4f want 119.00", rows[0].Buchung.HabenSum())
	}
}

// TestRebookForeignToEUR_EURInvoiceUntouched verifies that EUR invoices are
// never touched regardless of their booking content.
func TestRebookForeignToEUR_EURInvoiceUntouched(t *testing.T) {
	repo := newTestRepo(t)

	bEUR := core.Booking{Entries: []core.BookingEntry{
		{Konto: 4920, Betrag: 100.0, Soll: true},
		{Konto: 1406, Betrag: 19.0, Soll: true},
		{Konto: 1800, Betrag: 119.0, Soll: false},
	}}

	row := core.CSVRow{
		Dateiname:    "eur-invoice.pdf",
		Jahr:         "2026",
		Monat:        "04",
		Auftraggeber: "Deutsche GmbH",
		Bruttobetrag: 119.0,
		Waehrung:     "EUR",
		Wechselkurs:  1.0,
		Buchung:      bEUR,
	}
	if _, err := repo.Insert(row); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	conv, skip, missing, err := repo.RebookForeignToEUR()
	if err != nil {
		t.Fatalf("RebookForeignToEUR: %v", err)
	}
	// EUR invoices are excluded by the SQL WHERE clause → all counts should be 0.
	if conv != 0 || skip != 0 || missing != 0 {
		t.Errorf("got converted=%d skipped=%d rateMissing=%d, want 0/0/0", conv, skip, missing)
	}

	rows, err := repo.List("2026", "04")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if !nearlyEqual(rows[0].Buchung.HabenSum(), 119.0) {
		t.Errorf("EUR booking was modified; HabenSum=%.4f want 119.00", rows[0].Buchung.HabenSum())
	}
}
