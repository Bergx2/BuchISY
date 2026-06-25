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
	data, err := BuildBookingJournalPDF(rows, nil, "Buchungsjournal Juni 2026", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 100 || string(data[:4]) != "%PDF" {
		t.Fatalf("not a PDF (%d bytes)", len(data))
	}
	// must not panic on empty input
	if _, err := BuildBookingJournalPDF(nil, nil, "Leer", ""); err != nil {
		t.Errorf("empty journal errored: %v", err)
	}
}

func TestBuildControllingPDF(t *testing.T) {
	c := Controlling{
		Einnahmen:       []AccountSum{{Konto: 8400, Name: "Erlöse", Summe: 1000.00}},
		Ausgaben:        []AccountSum{{Konto: 6640, Name: "Bewirtungskosten (abziehbar)", Summe: 1240.00}},
		EinnahmenGesamt: 1000.00,
		AusgabenGesamt:  1240.00,
		Saldo:           -240.00,
	}
	data, err := BuildControllingPDF(c, "Controlling 2026", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 100 || string(data[:4]) != "%PDF" {
		t.Fatalf("not a PDF (%d bytes)", len(data))
	}
	if _, err := BuildControllingPDF(Controlling{}, "Leer", ""); err != nil {
		t.Errorf("empty controlling PDF errored: %v", err)
	}
}

func TestBuildInvoiceListPDF(t *testing.T) {
	rows := []CSVRow{{Rechnungsdatum: "18.06.2026", Auftraggeber: "Müller GmbH", Rechnungsnummer: "R-1", BetragNetto: 100, SteuersatzBetrag: 19, Bruttobetrag: 119}}
	data, err := BuildInvoiceListPDF(rows, "Belegliste Juni 2026", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 100 || string(data[:4]) != "%PDF" {
		t.Fatalf("not a PDF (%d bytes)", len(data))
	}
	if _, err := BuildInvoiceListPDF(nil, "Leer", ""); err != nil {
		t.Errorf("empty list PDF errored: %v", err)
	}
}

func TestBuildUStVAPDF(t *testing.T) {
	u := UStVAOfficial{Kz81: 6500, USt81: 1235, Kz45: 1077.60, Kz84: 462.40, Kz85: 87.86, Kz67: 87.86, Kz66: 37.79, Kz83: 1197.21}
	data, err := BuildUStVAPDF(u, "UStVA 2025", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 100 || string(data[:4]) != "%PDF" {
		t.Fatalf("not a PDF (%d bytes)", len(data))
	}
	// empty (everything zero) still renders Kz 83 without error
	if _, err := BuildUStVAPDF(UStVAOfficial{}, "Leer", ""); err != nil {
		t.Errorf("empty UStVA PDF errored: %v", err)
	}
}

func TestBuildZMPDF(t *testing.T) {
	z := ZM{Zeilen: []ZMZeile{{UStIdNr: "FI26378052", Netto: 44795}}, Kontrollsumme: 44795}
	data, err := BuildZMPDF(z, "287472874", "Zusammenfassende Meldung 2025", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 100 || string(data[:4]) != "%PDF" {
		t.Fatalf("not a PDF (%d bytes)", len(data))
	}
	if _, err := BuildZMPDF(ZM{}, "", "Leer", ""); err != nil {
		t.Errorf("empty ZM PDF errored: %v", err)
	}
}

func TestBuildSalesJournalPDF(t *testing.T) {
	rows := []CSVRow{
		{Ausgangsrechnung: true, Belegnummer: "2025-0002", Rechnungsnummer: "RA-1", Rechnungsdatum: "10.12.2025", Auftraggeber: "Symeo GmbH", Gegenkonto: 8400, BetragNetto: 6500, SteuersatzBetrag: 1235, Bruttobetrag: 7735},
		{Ausgangsrechnung: false, Auftraggeber: "Lieferant"}, // incoming → excluded
	}
	data, err := BuildSalesJournalPDF(rows, nil, "Rechnungsausgangsbuch Dezember 2025", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 100 || string(data[:4]) != "%PDF" {
		t.Fatalf("not a PDF (%d bytes)", len(data))
	}
	if _, err := BuildSalesJournalPDF(nil, nil, "Leer", ""); err != nil {
		t.Errorf("empty sales journal errored: %v", err)
	}
}

func TestPDFReportsPaginate(t *testing.T) {
	var rows []CSVRow
	for i := 0; i < 200; i++ {
		rows = append(rows, CSVRow{Rechnungsdatum: "18.06.2026", Auftraggeber: "Firma", Rechnungsnummer: "R", BetragNetto: 1, SteuersatzBetrag: 0.19, Bruttobetrag: 1.19,
			Buchung: Booking{Entries: []BookingEntry{{Konto: 6640, Betrag: 1, Soll: true}, {Konto: 1800, Betrag: 1, Soll: false}}}})
	}
	j, err := BuildBookingJournalPDF(rows, nil, "Journal", "")
	if err != nil || len(j) < 1000 || string(j[:4]) != "%PDF" {
		t.Fatalf("journal: err=%v len=%d", err, len(j))
	}
	l, err := BuildInvoiceListPDF(rows, "Liste", "")
	if err != nil || len(l) < 1000 || string(l[:4]) != "%PDF" {
		t.Fatalf("list: err=%v len=%d", err, len(l))
	}
	var ausgaben []AccountSum
	for i := 0; i < 200; i++ {
		ausgaben = append(ausgaben, AccountSum{Konto: 6000 + i, Name: "Konto", Summe: 1})
	}
	ctrl := Controlling{
		Ausgaben:       ausgaben,
		AusgabenGesamt: 200,
		Saldo:          -200,
	}
	cp, err := BuildControllingPDF(ctrl, "Controlling", "")
	if err != nil || len(cp) < 1000 || string(cp[:4]) != "%PDF" {
		t.Fatalf("controlling: err=%v len=%d", err, len(cp))
	}
}
