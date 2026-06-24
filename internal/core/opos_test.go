package core

import (
	"testing"
	"time"
)

func TestComputeOpenItems(t *testing.T) {
	asOf := time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC)

	// 40 days before asOf → "31–60" bucket
	date40 := asOf.AddDate(0, 0, -40).Format("02.01.2006")
	// 10 days before asOf → "0–30" bucket
	date10 := asOf.AddDate(0, 0, -10).Format("02.01.2006")

	rows := []CSVRow{
		{
			Belegnummer:      "2026-0001",
			Rechnungsnummer:  "RE-2026-001",
			Rechnungsdatum:   date40,
			Auftraggeber:     "Kunde GmbH",
			Bruttobetrag:     1000.00,
			Ausgangsrechnung: true,
			Bezahldatum:      "",
			BuchungRef:       "",
		},
		{
			Belegnummer:     "2026-0002",
			Rechnungsnummer: "ER-2026-001",
			Rechnungsdatum:  date10,
			Auftraggeber:    "Lieferant AG",
			Bruttobetrag:    250.00,
			Ausgangsrechnung: false,
			Bezahldatum:     "",
			BuchungRef:      "",
		},
		// PAID — must be excluded
		{
			Belegnummer:     "2026-0003",
			Rechnungsnummer: "ER-2026-002",
			Rechnungsdatum:  date10,
			Auftraggeber:    "Anderer Lieferant",
			Bruttobetrag:    500.00,
			Ausgangsrechnung: false,
			Bezahldatum:     "10.06.2026",
			BuchungRef:      "",
		},
	}

	oi := ComputeOpenItems(rows, asOf)

	// 1 Forderung
	if len(oi.Forderungen) != 1 {
		t.Fatalf("expected 1 Forderung, got %d", len(oi.Forderungen))
	}
	f := oi.Forderungen[0]
	if f.Bucket != "31–60" {
		t.Errorf("Forderung bucket = %q, want \"31–60\"", f.Bucket)
	}
	if f.AgeDays != 40 {
		t.Errorf("Forderung AgeDays = %d, want 40", f.AgeDays)
	}
	if f.Partner != "Kunde GmbH" {
		t.Errorf("Forderung Partner = %q, want \"Kunde GmbH\"", f.Partner)
	}
	if f.Betrag != 1000.00 {
		t.Errorf("Forderung Betrag = %.2f, want 1000.00", f.Betrag)
	}

	// 1 Verbindlichkeit
	if len(oi.Verbindlichkeiten) != 1 {
		t.Fatalf("expected 1 Verbindlichkeit, got %d", len(oi.Verbindlichkeiten))
	}
	v := oi.Verbindlichkeiten[0]
	if v.Bucket != "0–30" {
		t.Errorf("Verbindlichkeit bucket = %q, want \"0–30\"", v.Bucket)
	}
	if v.AgeDays != 10 {
		t.Errorf("Verbindlichkeit AgeDays = %d, want 10", v.AgeDays)
	}

	// Totals
	if oi.ForderungenGesamt != 1000.00 {
		t.Errorf("ForderungenGesamt = %.2f, want 1000.00", oi.ForderungenGesamt)
	}
	if oi.VerbindlichkeitenGesamt != 250.00 {
		t.Errorf("VerbindlichkeitenGesamt = %.2f, want 250.00", oi.VerbindlichkeitenGesamt)
	}
}

func TestBuildOpenItemsPDF(t *testing.T) {
	oi := OpenItems{
		Forderungen: []OpenItem{
			{Belegnummer: "2026-0001", Rechnungsnummer: "RE-001", Datum: "15.05.2026", Partner: "Kunde GmbH", Betrag: 1190.00, AgeDays: 40, Bucket: "31–60"},
		},
		Verbindlichkeiten: []OpenItem{
			{Belegnummer: "2026-0002", Rechnungsnummer: "ER-001", Datum: "14.06.2026", Partner: "Lieferant AG", Betrag: 297.50, AgeDays: 10, Bucket: "0–30"},
		},
		ForderungenGesamt:      1190.00,
		VerbindlichkeitenGesamt: 297.50,
	}

	data, err := BuildOpenItemsPDF(oi, "Offene Posten 2026")
	if err != nil {
		t.Fatalf("BuildOpenItemsPDF error: %v", err)
	}
	if len(data) < 4 || string(data[:4]) != "%PDF" {
		t.Errorf("output does not start with %%PDF")
	}
}
