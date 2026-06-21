package core

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCSVRoundTripWithAttachments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invoices.csv")
	repo := NewCSVRepository()

	row := CSVRow{
		Dateiname:      "2025-08-01_AWS_EUR.pdf",
		Auftraggeber:   "AWS",
		Bruttobetrag:   37.64,
		Waehrung:       "EUR",
		HatAnhaenge:    true,
		AnzahlAnhaenge: 2,
	}
	if err := repo.Append(path, row); err != nil {
		t.Fatalf("append: %v", err)
	}

	rows, err := repo.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if rows[0].Auftraggeber != "AWS" {
		t.Errorf("Auftraggeber = %q, want AWS", rows[0].Auftraggeber)
	}
	if !rows[0].HatAnhaenge {
		t.Error("HatAnhaenge should be true")
	}
	if rows[0].AnzahlAnhaenge != 2 {
		t.Errorf("AnzahlAnhaenge = %d, want 2", rows[0].AnzahlAnhaenge)
	}
}

func TestCSVLoadLegacyWithoutAttachmentColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invoices.csv")
	legacy := "Dateiname,Firmenname\nalt.pdf,Telekom\n"
	if err := os.WriteFile(path, []byte(legacy), 0644); err != nil {
		t.Fatal(err)
	}
	rows, err := NewCSVRepository().Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows", len(rows))
	}
	if rows[0].HatAnhaenge || rows[0].AnzahlAnhaenge != 0 {
		t.Error("legacy row should default attachments to false/0")
	}
}

func TestCSVUnterordnerRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invoices.csv")
	repo := NewCSVRepository()

	if err := repo.Append(path, CSVRow{Dateiname: "a.pdf", Unterordner: "Bar"}); err != nil {
		t.Fatalf("append: %v", err)
	}
	rows, err := repo.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(rows) != 1 || rows[0].Unterordner != "Bar" {
		t.Fatalf("round-trip mismatch: %+v", rows)
	}
}

func TestCSVWriteTo(t *testing.T) {
	repo := NewCSVRepository()
	var buf bytes.Buffer
	rows := []CSVRow{
		{Dateiname: "a.pdf", Auftraggeber: "Foo GmbH"},
		{Dateiname: "b.pdf", Auftraggeber: "Bar AG"},
	}
	if err := repo.WriteTo(&buf, rows); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Dateiname") {
		t.Errorf("header missing in output: %q", out)
	}
	if !strings.Contains(out, "a.pdf") || !strings.Contains(out, "Foo GmbH") ||
		!strings.Contains(out, "b.pdf") || !strings.Contains(out, "Bar AG") {
		t.Errorf("row data missing in output: %q", out)
	}
	lines := 0
	for _, ln := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if strings.TrimSpace(ln) != "" {
			lines++
		}
	}
	if lines != 3 {
		t.Errorf("expected 3 lines, got %d: %q", lines, out)
	}
}

func TestCSVTaxLinesRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invoices.csv")
	repo := NewCSVRepository()
	row := CSVRow{
		Dateiname: "a.pdf", Jahr: "2026", Monat: "06",
		BetragNetto: 32.89, SteuersatzBetrag: 4.01, Bruttobetrag: 38.90,
		TaxLines: []TaxLine{
			{Netto: 14.20, SatzProzent: 19, MwStBetrag: 2.70},
			{Netto: 18.69, SatzProzent: 7, MwStBetrag: 1.31},
		},
		Trinkgeld: 2.00,
	}
	if err := repo.Append(path, row); err != nil {
		t.Fatal(err)
	}
	rows, err := repo.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || len(rows[0].TaxLines) != 2 || rows[0].Trinkgeld != 2.00 {
		t.Fatalf("tax lines not round-tripped: %+v", rows)
	}
	// Verify MwStBetrag survived the round-trip (first line: 2.70).
	if got := rows[0].TaxLines[0].MwStBetrag; got < 2.695 || got > 2.705 {
		t.Errorf("TaxLines[0].MwStBetrag = %v, want ~2.70", got)
	}
}

func TestCSVLegacyReconstructsTaxLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invoices.csv")
	legacy := "Dateiname,BetragNetto,Steuersatz_Prozent,Steuersatz_Betrag\nalt.pdf,10.00,19,1.90\n"
	if err := os.WriteFile(path, []byte(legacy), 0644); err != nil {
		t.Fatal(err)
	}
	rows, err := NewCSVRepository().Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || len(rows[0].TaxLines) != 1 || rows[0].TaxLines[0].SatzProzent != 19 {
		t.Fatalf("legacy row should reconstruct one TaxLine: %+v", rows)
	}
}

func TestSetColumnOrderKeepsNewColumns(t *testing.T) {
	repo := NewCSVRepository()
	repo.SetColumnOrder([]string{"Dateiname", "Firmenname"})
	header := repo.GetHeader()
	found := false
	for _, c := range header {
		if c == "Unterordner" {
			found = true
		}
	}
	if !found {
		t.Errorf("GetHeader() = %v, must include Unterordner even for a legacy column order", header)
	}
}

func TestCSVBookingRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invoices.csv")
	repo := NewCSVRepository()
	row := CSVRow{Dateiname: "a.pdf", Jahr: "2026", Monat: "06",
		Buchung: Booking{Entries: []BookingEntry{{Konto: 6640, Betrag: 12.71, Soll: true}, {Konto: 1800, Betrag: 12.71, Soll: false}}, Info: "Bewirtung"}}
	if err := repo.Append(path, row); err != nil {
		t.Fatal(err)
	}
	rows, _ := repo.Load(path)
	if len(rows) != 1 || len(rows[0].Buchung.Entries) != 2 || rows[0].Buchung.Info != "Bewirtung" {
		t.Fatalf("booking not round-tripped: %+v", rows)
	}
}
