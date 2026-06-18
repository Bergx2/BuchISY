package core

import (
	"path/filepath"
	"testing"
)

func TestCashBooksRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kassenbuch.json")

	books := []CashBook{
		{
			Konto:          "Barkasse",
			Anfangsbestand: 200.50,
			Einlagen: []CashDeposit{
				{Datum: "03.05.2026", Beschreibung: "Bankabhebung", Betrag: 300},
			},
		},
	}
	if err := SaveCashBooks(path, books); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := LoadCashBooks(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != 1 || got[0].Konto != "Barkasse" || got[0].Anfangsbestand != 200.50 {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
	if len(got[0].Einlagen) != 1 || got[0].Einlagen[0].Betrag != 300 {
		t.Fatalf("deposits mismatch: %+v", got[0].Einlagen)
	}
}

func TestLoadCashBooksMissingFile(t *testing.T) {
	got, err := LoadCashBooks(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("missing file should yield empty slice, got %+v", got)
	}
}

func TestComputeCashReport(t *testing.T) {
	book := CashBook{
		Konto:          "Barkasse",
		Anfangsbestand: 100,
		Einlagen: []CashDeposit{
			{Datum: "10.05.2026", Beschreibung: "Einlage", Betrag: 50},
		},
	}
	invoices := []CSVRow{
		{Firmenname: "Spät", Dateiname: "b.pdf", Bruttobetrag: 20, Bezahldatum: "05.05.2026"},
		{Firmenname: "Früh", Dateiname: "a.pdf", Bruttobetrag: 30, Rechnungsdatum: "01.05.2026"}, // no Bezahldatum
	}

	entries, end := ComputeCashReport(book, invoices)
	if len(entries) != 3 {
		t.Fatalf("got %d entries, want 3", len(entries))
	}
	// Chronological: 01.05 (Früh, -30), 05.05 (Spät, -20), 10.05 (Einlage, +50)
	if entries[0].Beschreibung != "Früh" || entries[0].Saldo != 70 {
		t.Errorf("entry 0 = %+v", entries[0])
	}
	if entries[1].Beschreibung != "Spät" || entries[1].Saldo != 50 {
		t.Errorf("entry 1 = %+v", entries[1])
	}
	if entries[2].Einnahme != 50 || entries[2].Saldo != 100 {
		t.Errorf("entry 2 = %+v", entries[2])
	}
	if end != 100 {
		t.Errorf("endbestand = %v, want 100", end)
	}
}

func TestComputeCashReportUndatableSortsLast(t *testing.T) {
	book := CashBook{Konto: "Barkasse", Anfangsbestand: 100}
	invoices := []CSVRow{
		{Firmenname: "Kaputt", Dateiname: "x.pdf", Bruttobetrag: 10, Bezahldatum: "ungültig"},
		{Firmenname: "Gut", Dateiname: "y.pdf", Bruttobetrag: 5, Bezahldatum: "04.05.2026"},
	}
	entries, _ := ComputeCashReport(book, invoices)
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	if entries[0].Beschreibung != "Gut" {
		t.Errorf("datable entry should sort first, got %q", entries[0].Beschreibung)
	}
	if entries[1].Beschreibung != "Kaputt" {
		t.Errorf("undatable entry should sort last, got %q", entries[1].Beschreibung)
	}
}
