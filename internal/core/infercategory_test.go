package core

import "testing"

func TestInferBookingCategory(t *testing.T) {
	rc := Booking{Entries: []BookingEntry{
		{Konto: 4825, Betrag: 170.65, Soll: true},
		{Konto: 1577, Betrag: 32.42, Soll: true},
		{Konto: 1787, Betrag: 32.42, Soll: false},
		{Konto: 1270, Betrag: 170.65, Soll: false},
	}}
	if got := InferBookingCategory(rc); got != "reverse_charge" {
		t.Errorf("reverse_charge: got %q", got)
	}

	bew := Booking{Entries: []BookingEntry{
		{Konto: 4650, Betrag: 167.99, Soll: true},
		{Konto: 4654, Betrag: 72.00, Soll: true},
		{Konto: 1576, Betrag: 8.16, Soll: true},
		{Konto: 1000, Betrag: 260.00, Soll: false},
	}}
	if got := InferBookingCategory(bew); got != "bewirtung" {
		t.Errorf("bewirtung: got %q", got)
	}

	// SKR04 reverse-charge accounts.
	rc04 := Booking{Entries: []BookingEntry{{Konto: 1407, Soll: true}, {Konto: 3837, Soll: false}}}
	if got := InferBookingCategory(rc04); got != "reverse_charge" {
		t.Errorf("reverse_charge SKR04: got %q", got)
	}

	// Standard booking → no special category.
	std := Booking{Entries: []BookingEntry{
		{Konto: 4650, Betrag: 100, Soll: true}, // only abz, no nicht → NOT bewirtung
		{Konto: 1576, Betrag: 19, Soll: true},
		{Konto: 1000, Betrag: 119, Soll: false},
	}}
	if got := InferBookingCategory(std); got != "" {
		t.Errorf("standard (4650 only, no 4654): got %q, want \"\"", got)
	}
	if got := InferBookingCategory(Booking{}); got != "" {
		t.Errorf("empty: got %q", got)
	}
}
