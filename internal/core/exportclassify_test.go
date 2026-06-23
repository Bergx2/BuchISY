package core

import "testing"

func TestClassifyForExport(t *testing.T) {
	good := Booking{Entries: []BookingEntry{{Konto: 6640, Betrag: 10, Soll: true}, {Konto: 1800, Betrag: 10, Soll: false}}}
	rows := []CSVRow{
		{Dateiname: "neu.pdf", Buchung: good},
		{Dateiname: "alt.pdf", Buchung: good, Exportiert: true},
		{Dateiname: "leer.pdf"},
		{Dateiname: "schief.pdf", Buchung: Booking{Entries: []BookingEntry{{Konto: 6640, Betrag: 10, Soll: true}}}},
	}
	c := ClassifyForExport(rows, false)
	if len(c.Exportable) != 1 || c.Exportable[0].Dateiname != "neu.pdf" {
		t.Errorf("exportable = %+v", c.Exportable)
	}
	if len(c.AlreadyExported) != 1 {
		t.Errorf("alreadyExported = %+v", c.AlreadyExported)
	}
	if len(c.Skipped) != 2 {
		t.Fatalf("skipped = %+v", c.Skipped)
	}
	// includeExported puts the already-exported row back into Exportable.
	if len(ClassifyForExport(rows, true).Exportable) != 2 {
		t.Error("includeExported should yield 2 exportable")
	}
}

// TestClassifyForExport_RevenueNotSkipped verifies that a balanced revenue
// booking (Ausgangsrechnung: 1 Soll payment + 2 Haben Erlös/USt) lands in
// Exportable rather than Skipped. This is the regression test for the bug
// where PaymentEntry() (which requires exactly 1 Haben) wrongly rejected
// revenue invoices before PaymentAndCounters was introduced.
func TestClassifyForExport_RevenueNotSkipped(t *testing.T) {
	// Revenue booking: 1 Soll (Zahlungskonto 1200) + 2 Haben (Erlös 8400, USt 1776).
	revBuchung := Booking{Entries: []BookingEntry{
		{Konto: 1200, Betrag: 119, Soll: true},
		{Konto: 8400, Betrag: 100, Soll: false},
		{Konto: 1776, Betrag: 19, Soll: false},
	}}
	rows := []CSVRow{
		{Dateiname: "rechnung.pdf", Buchung: revBuchung, Ausgangsrechnung: true},
	}
	c := ClassifyForExport(rows, false)
	if len(c.Exportable) != 1 || c.Exportable[0].Dateiname != "rechnung.pdf" {
		t.Errorf("revenue invoice must be Exportable, got exportable=%+v skipped=%+v", c.Exportable, c.Skipped)
	}
	if len(c.Skipped) != 0 {
		t.Errorf("expected 0 skipped, got %+v", c.Skipped)
	}
}
