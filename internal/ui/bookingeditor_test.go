package ui

import (
	"testing"
)

func TestBookingFromRows(t *testing.T) {
	rows := []bookingEditRow{
		{Konto: 6640, Betrag: 12.71, Soll: true},
		{Konto: 1800, Betrag: 12.71, Soll: false},
	}
	b := bookingFromRows(rows)
	if !b.Manuell {
		t.Error("manual flag not set")
	}
	if len(b.Entries) != 2 || b.Entries[0].Konto != 6640 || !b.Entries[0].Soll {
		t.Fatalf("entries wrong: %+v", b.Entries)
	}
	if !b.Balanced() {
		t.Errorf("12,71 S vs 12,71 H should balance: %+v", b)
	}
	// rows with Konto==0 are dropped (incomplete)
	if len(bookingFromRows([]bookingEditRow{{Konto: 0, Betrag: 5, Soll: true}}).Entries) != 0 {
		t.Error("zero-account row should be dropped")
	}
}
