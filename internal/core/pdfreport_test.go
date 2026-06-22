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

func TestBuildControllingPDF(t *testing.T) {
	sums := []AccountSum{{Konto: 6640, Name: "Bewirtungskosten (abziehbar)", Summe: 1240.00}}
	data, err := BuildControllingPDF(sums, 1240.00, "Controlling 2026")
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 100 || string(data[:4]) != "%PDF" {
		t.Fatalf("not a PDF (%d bytes)", len(data))
	}
	if _, err := BuildControllingPDF(nil, 0, "Leer"); err != nil {
		t.Errorf("empty controlling PDF errored: %v", err)
	}
}

func TestBuildInvoiceListPDF(t *testing.T) {
	rows := []CSVRow{{Rechnungsdatum: "18.06.2026", Auftraggeber: "Müller GmbH", Rechnungsnummer: "R-1", BetragNetto: 100, SteuersatzBetrag: 19, Bruttobetrag: 119}}
	data, err := BuildInvoiceListPDF(rows, "Belegliste Juni 2026")
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 100 || string(data[:4]) != "%PDF" {
		t.Fatalf("not a PDF (%d bytes)", len(data))
	}
	if _, err := BuildInvoiceListPDF(nil, "Leer"); err != nil {
		t.Errorf("empty list PDF errored: %v", err)
	}
}

func TestPDFReportsPaginate(t *testing.T) {
	var rows []CSVRow
	for i := 0; i < 200; i++ {
		rows = append(rows, CSVRow{Rechnungsdatum: "18.06.2026", Auftraggeber: "Firma", Rechnungsnummer: "R", BetragNetto: 1, SteuersatzBetrag: 0.19, Bruttobetrag: 1.19,
			Buchung: Booking{Entries: []BookingEntry{{Konto: 6640, Betrag: 1, Soll: true}, {Konto: 1800, Betrag: 1, Soll: false}}}})
	}
	j, err := BuildBookingJournalPDF(rows, nil, "Journal")
	if err != nil || len(j) < 1000 || string(j[:4]) != "%PDF" {
		t.Fatalf("journal: err=%v len=%d", err, len(j))
	}
	l, err := BuildInvoiceListPDF(rows, "Liste")
	if err != nil || len(l) < 1000 || string(l[:4]) != "%PDF" {
		t.Fatalf("list: err=%v len=%d", err, len(l))
	}
	var sums []AccountSum
	for i := 0; i < 200; i++ {
		sums = append(sums, AccountSum{Konto: 6000 + i, Name: "Konto", Summe: 1})
	}
	c, err := BuildControllingPDF(sums, 200, "Controlling")
	if err != nil || len(c) < 1000 || string(c[:4]) != "%PDF" {
		t.Fatalf("controlling: err=%v len=%d", err, len(c))
	}
}
