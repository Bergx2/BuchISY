package core

import "testing"

func TestAggregateControlling(t *testing.T) {
	rules, _ := ParseBookingRules([]byte(`{"vorsteuer_konten":{"19":1576},"umsatzsteuer_konten":{"19":1776},"regeln":[{"kategorie":"reverse_charge","rc_satz":19,"konto_vst_rc":1577,"konto_ust_rc":1787}]}`))
	pay := map[int]bool{1200: true, 1000: true}
	rows := []CSVRow{
		// expense: Soll 4240 (expense) + 1576 (VAT, excluded), Haben 1200 (payment, excluded)
		{Buchung: Booking{Entries: []BookingEntry{{Konto: 4240, Betrag: 100, Soll: true}, {Konto: 1576, Betrag: 19, Soll: true}, {Konto: 1200, Betrag: 119, Soll: false}}}},
		// revenue: Soll 1200 (payment, excluded), Haben 8400 (revenue) + 1776 (VAT, excluded)
		{Ausgangsrechnung: true, Buchung: Booking{Entries: []BookingEntry{{Konto: 1200, Betrag: 119, Soll: true}, {Konto: 8400, Betrag: 100, Soll: false}, {Konto: 1776, Betrag: 19, Soll: false}}}},
		// §13b: Soll 27 (expense) + 1577 (VAT, excluded), Haben 1787 (VAT, excluded) + 1200 (payment)
		{Buchung: Booking{Entries: []BookingEntry{{Konto: 27, Betrag: 50, Soll: true}, {Konto: 1577, Betrag: 9.5, Soll: true}, {Konto: 1787, Betrag: 9.5, Soll: false}, {Konto: 1200, Betrag: 50, Soll: false}}}},
	}
	c := AggregateControlling(rows, rules, pay, nil)
	if !almost(c.EinnahmenGesamt, 100) {
		t.Errorf("Einnahmen = %v, want 100 (only 8400)", c.EinnahmenGesamt)
	}
	if !almost(c.AusgabenGesamt, 150) {
		t.Errorf("Ausgaben = %v, want 150 (4240 + 27)", c.AusgabenGesamt)
	}
	if !almost(c.Saldo, -50) {
		t.Errorf("Saldo = %v, want -50", c.Saldo)
	}
	if len(c.Einnahmen) != 1 || c.Einnahmen[0].Konto != 8400 {
		t.Fatalf("Einnahmen lines = %+v (want one, 8400)", c.Einnahmen)
	}
	if len(c.Ausgaben) != 2 {
		t.Fatalf("Ausgaben lines = %+v (want 4240 and 27)", c.Ausgaben)
	}
}

func TestAggregateBookingsByAccount(t *testing.T) {
	chart := NewChartOfAccounts([]SKRAccount{
		{Number: 6640, Name: "Bewirtungskosten (abziehbar)"},
		{Number: 1406, Name: "Abziehbare Vorsteuer 19%"},
	})
	rows := []CSVRow{
		{Buchung: Booking{Entries: []BookingEntry{
			{Konto: 6640, Betrag: 12.71, Soll: true},
			{Konto: 1406, Betrag: 1.26, Soll: true},
			{Konto: 1800, Betrag: 13.97, Soll: false}, // Haben — excluded
		}}},
		{Buchung: Booking{Entries: []BookingEntry{
			{Konto: 6640, Betrag: 7.29, Soll: true},
			{Konto: 1800, Betrag: 7.29, Soll: false},
		}}},
		{Buchung: Booking{}}, // no entries — contributes nothing
	}
	sums, total := AggregateBookingsByAccount(rows, chart)
	if len(sums) != 2 {
		t.Fatalf("want 2 accounts, got %d: %+v", len(sums), sums)
	}
	// sorted ascending by Konto → 1406 first, then 6640
	if sums[0].Konto != 1406 || !almost(sums[0].Summe, 1.26) || sums[0].Name != "Abziehbare Vorsteuer 19%" {
		t.Errorf("sums[0] = %+v", sums[0])
	}
	if sums[1].Konto != 6640 || !almost(sums[1].Summe, 20.00) {
		t.Errorf("6640 sum = %+v (want 20.00)", sums[1])
	}
	if !almost(total, 21.26) {
		t.Errorf("total = %v (want 21.26)", total)
	}
}
