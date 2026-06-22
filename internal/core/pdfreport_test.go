package core

import "testing"

func TestBuildBookingJournalPDF(t *testing.T) {
	rows := []CSVRow{
		{Rechnungsdatum: "18.06.2026", Rechnungsnummer: "R-1", Auftraggeber: "Matcha Rina (Café)",
			Buchung: Booking{Entries: []BookingEntry{
				{Konto: 6640, Betrag: 12.71, Soll: true},
				{Konto: 1800, Betrag: 12.71, Soll: false},
			}}},
		{Rechnungsdatum: "19.06.2026", Auftraggeber: "Ohne Buchung"}, // skipped
	}
	data, err := BuildBookingJournalPDF(rows, nil, "Buchungsjournal Juni 2026")
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 100 || string(data[:4]) != "%PDF" {
		t.Fatalf("not a PDF (%d bytes)", len(data))
	}
	// must not panic on empty input
	if _, err := BuildBookingJournalPDF(nil, nil, "Leer"); err != nil {
		t.Errorf("empty journal errored: %v", err)
	}
}
