package core

import (
	"strings"
	"testing"
)

func TestBuildLexwareCSV(t *testing.T) {
	rows := []CSVRow{
		{Rechnungsdatum: "18.06.2026", Rechnungsnummer: "R-1", Auftraggeber: "Matcha Rina", Verwendungszweck: "Bewirtung",
			Buchung: Booking{Entries: []BookingEntry{
				{Konto: 6640, Betrag: 12.71, Soll: true},
				{Konto: 1800, Betrag: 12.71, Soll: false},
			}}},
		{Rechnungsdatum: "19.06.2026"}, // no booking → skipped
	}
	data, exported, skipped := BuildLexwareCSV(rows)
	if exported != 1 || skipped != 1 {
		t.Fatalf("exported=%d skipped=%d (want 1,1)", exported, skipped)
	}
	s := string(data)
	if !strings.HasPrefix(s, "Datum;Belegnr;Buchungstext;Betrag;Sollkonto;Habenkonto") {
		t.Errorf("missing header: %q", s)
	}
	if !strings.Contains(s, "18.06.2026;R-1;Matcha Rina Bewirtung;12,71;6640;1800") {
		t.Errorf("data line wrong:\n%s", s)
	}
}

// TestLexwareBelegnummerPreferred verifies the internal Belegnummer is used as
// the Belegnr when present, in preference to the supplier invoice number.
func TestLexwareBelegnummerPreferred(t *testing.T) {
	rows := []CSVRow{
		{Rechnungsdatum: "06.06.2026", Belegnummer: "2026-0014", Rechnungsnummer: "MC9C7PFZ-103052",
			Auftraggeber: "Matcha Rina", Verwendungszweck: "Bewirtung",
			Buchung: Booking{Entries: []BookingEntry{
				{Konto: 4650, Betrag: 12.71, Soll: true},
				{Konto: 1755, Betrag: 12.71, Soll: false},
			}}},
	}
	data, _, _ := BuildLexwareCSV(rows)
	s := string(data)
	if !strings.Contains(s, "06.06.2026;2026-0014;Matcha Rina Bewirtung;12,71;4650;1755") {
		t.Errorf("Belegnummer not used as Belegnr:\n%s", s)
	}
}
