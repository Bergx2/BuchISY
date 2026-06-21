package core

import "testing"

func TestBookingBalance(t *testing.T) {
	b := Booking{Entries: []BookingEntry{
		{Konto: 6640, Betrag: 12.71, Soll: true},
		{Konto: 6644, Betrag: 5.44, Soll: true},
		{Konto: 1406, Betrag: 1.26, Soll: true},
		{Konto: 1401, Betrag: 0.59, Soll: true},
		{Konto: 1800, Betrag: 20.00, Soll: false},
	}}
	if !almost(b.SollSum(), 20.00) || !almost(b.HabenSum(), 20.00) {
		t.Fatalf("sums: soll=%v haben=%v", b.SollSum(), b.HabenSum())
	}
	if !b.Balanced() {
		t.Error("should be balanced")
	}
	if (Booking{}).Balanced() {
		t.Error("empty booking is not balanced")
	}
	if !(Booking{}).IsEmpty() {
		t.Error("zero booking should be empty")
	}
}
