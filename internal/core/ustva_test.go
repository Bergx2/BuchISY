package core

import "testing"

func TestComputeUStVA(t *testing.T) {
	rules, _ := ParseBookingRules([]byte(`{"vorsteuer_konten":{"19":1406,"7":1401},"regeln":[]}`))
	rows := []CSVRow{
		{Buchung: Booking{Entries: []BookingEntry{{Konto: 6640, Betrag: 100, Soll: true}, {Konto: 1406, Betrag: 19, Soll: true}, {Konto: 1800, Betrag: 119, Soll: false}}}},
		{Buchung: Booking{Entries: []BookingEntry{{Konto: 6815, Betrag: 50, Soll: true}, {Konto: 1401, Betrag: 3.50, Soll: true}, {Konto: 1800, Betrag: 53.50, Soll: false}}}},
		{Buchung: Booking{Entries: []BookingEntry{{Konto: 1406, Betrag: 1, Soll: true}, {Konto: 1800, Betrag: 1, Soll: false}}}},
	}
	u := ComputeUStVA(rows, rules)
	if !almost(u.VorsteuerGesamt, 23.50) {
		t.Errorf("total = %v, want 23.50", u.VorsteuerGesamt)
	}
	if len(u.Zeilen) != 2 {
		t.Fatalf("want 2 rate lines, got %d: %+v", len(u.Zeilen), u.Zeilen)
	}
	// sorted ascending by Satz → 7% first
	if u.Zeilen[0].Satz != 7 || !almost(u.Zeilen[0].Vorsteuer, 3.50) {
		t.Errorf("7%% line = %+v", u.Zeilen[0])
	}
	if u.Zeilen[1].Satz != 19 || !almost(u.Zeilen[1].Vorsteuer, 20.00) || u.Zeilen[1].Konto != 1406 {
		t.Errorf("19%% line = %+v (want 20.00 on 1406)", u.Zeilen[1])
	}
}
