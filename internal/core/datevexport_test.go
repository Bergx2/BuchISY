package core

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestBuildDATEVStapel(t *testing.T) {
	rows := []CSVRow{
		{Rechnungsdatum: "18.06.2026", Rechnungsnummer: "MC9C7PFZ-103052", Auftraggeber: "Matcha Rina",
			Buchung: Booking{Entries: []BookingEntry{
				{Konto: 6640, Betrag: 12.71, Soll: true},
				{Konto: 6644, Betrag: 5.44, Soll: true},
				{Konto: 1406, Betrag: 1.26, Soll: true},
				{Konto: 1401, Betrag: 0.59, Soll: true},
				{Konto: 1800, Betrag: 20.00, Soll: false},
			}}},
		{Rechnungsdatum: "19.06.2026", Auftraggeber: "Ohne Buchung"}, // no booking → skipped
	}
	data, exported, skipped := BuildDATEVStapel(DATEVHeader{BeraterNr: "", MandantNr: "", WJBeginn: "20260101", ErzeugtAm: "20260621120000000", DatumVon: "20260601", DatumBis: "20260630"}, rows)
	if exported != 4 || skipped != 1 {
		t.Fatalf("exported=%d skipped=%d (want 4,1)", exported, skipped)
	}
	s := string(data)
	if !strings.HasPrefix(s, `"EXTF";700;21;"Buchungsstapel"`) {
		t.Errorf("missing EXTF header: %q", s[:40])
	}
	// One data row carries 12,71 booked on 6640 against 1800, Beleg 1806.
	if !strings.Contains(s, `12,71;"S";"EUR";;;;6640;1800;;1806;"MC9C7PFZ-103052"`) {
		t.Errorf("expected 6640 data row not found:\n%s", s)
	}
	// The payment account 1800 is never its own data row (only a Gegenkonto).
	if strings.Contains(s, `;"S";"EUR";;;;1800;`) {
		t.Error("payment account must not be a debit row")
	}
}

// TestDATEVBelegnummerFields verifies that when a Belegnummer is present it
// fills Belegfeld 1 while the supplier invoice number moves to Belegfeld 2.
func TestDATEVBelegnummerFields(t *testing.T) {
	rows := []CSVRow{
		{Rechnungsdatum: "06.06.2026", Belegnummer: "2026-0014", Rechnungsnummer: "MC9C7PFZ-103052", Auftraggeber: "Matcha Rina",
			Buchung: Booking{Entries: []BookingEntry{
				{Konto: 4650, Betrag: 12.71, Soll: true},
				{Konto: 1755, Betrag: 12.71, Soll: false},
			}}},
	}
	data, _, _ := BuildDATEVStapel(DATEVHeader{WJBeginn: "20260101"}, rows)
	s := string(data)
	if !strings.Contains(s, `;1755;;0606;"2026-0014";"MC9C7PFZ-103052";;"Matcha Rina"`) {
		t.Errorf("Belegnummer/Rechnungsnummer not split into Belegfeld 1/2:\n%s", s)
	}
}

func TestDATEVRevenueRow(t *testing.T) {
	rows := []CSVRow{{
		Rechnungsdatum: "10.12.2025", Belegnummer: "2025-0002", Auftraggeber: "Symeo",
		Ausgangsrechnung: true,
		Buchung: Booking{Entries: []BookingEntry{
			{Konto: 1200, Betrag: 7735, Soll: true},
			{Konto: 8400, Betrag: 6500, Soll: false},
			{Konto: 1776, Betrag: 1235, Soll: false},
		}},
	}}
	data, exported, skipped := BuildDATEVStapel(DATEVHeader{WJBeginn: "20250101"}, rows)
	if exported != 2 || skipped != 0 {
		t.Fatalf("exported=%d skipped=%d (want 2,0)", exported, skipped)
	}
	s := string(data)
	// Erlös line: 6500 credited (H) on 8400 against base 1200.
	if !strings.Contains(s, `6500,00;"H";"EUR";;;;8400;1200;;1012;"2025-0002"`) {
		t.Errorf("revenue Erlös row missing:\n%s", s)
	}
	// USt line: 1235 credited (H) on 1776 against 1200.
	if !strings.Contains(s, `1235,00;"H";"EUR";;;;1776;1200;;1012;"2025-0002"`) {
		t.Errorf("revenue USt row missing:\n%s", s)
	}
}

func TestDatevCleanRuneSafe(t *testing.T) {
	// 40 'ü' runes (80 bytes); truncating to 36 runes must stay valid UTF-8.
	in := strings.Repeat("ü", 40)
	got := datevClean(in, 36)
	if utf8.RuneCountInString(got) != 36 {
		t.Errorf("want 36 runes, got %d", utf8.RuneCountInString(got))
	}
	if !utf8.ValidString(got) {
		t.Errorf("result is not valid UTF-8: %q", got)
	}
	// quotes stripped, short strings untouched
	if datevClean(`a"b`, 60) != "ab" {
		t.Errorf("quote strip failed: %q", datevClean(`a"b`, 60))
	}
}
