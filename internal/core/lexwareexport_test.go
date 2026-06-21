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
