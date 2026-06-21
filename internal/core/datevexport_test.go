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
