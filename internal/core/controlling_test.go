package core

import "testing"

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
