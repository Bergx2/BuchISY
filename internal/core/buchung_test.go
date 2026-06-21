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

func TestBuildBookingBewirtung(t *testing.T) {
	rules, _ := ParseBookingRules([]byte(`{"vorsteuer_konten":{"19":1406,"7":1401},"regeln":[{"kategorie":"standard","name":"Standard"},{"kategorie":"bewirtung","name":"Bewirtung","abziehbar_prozent":70,"konto_abziehbar":6640,"konto_nicht_abziehbar":6644}]}`))
	lines := []TaxLine{
		{Netto: 6.64, SatzProzent: 19, MwStBetrag: 1.26},
		{Netto: 8.41, SatzProzent: 7, MwStBetrag: 0.59},
	}
	b, err := BuildBooking(rules, "bewirtung", lines, 3.10, 0, 1800)
	if err != nil {
		t.Fatal(err)
	}
	if !b.Balanced() || !almost(b.HabenSum(), 20.00) {
		t.Fatalf("not balanced / haben != 20: %+v (haben=%v)", b, b.HabenSum())
	}
	got := map[int]float64{}
	for _, e := range b.Entries {
		if e.Soll {
			got[e.Konto] += e.Betrag
		}
	}
	// net+trinkgeld = 18.15; 70% = 12.71 (6640), remainder 5.44 (6644); VSt 1.26/0.59.
	if !almost(got[6640], 12.71) || !almost(got[6644], 5.44) {
		t.Errorf("split wrong: 6640=%v 6644=%v", got[6640], got[6644])
	}
	if !almost(got[1406], 1.26) || !almost(got[1401], 0.59) {
		t.Errorf("vorsteuer wrong: 1406=%v 1401=%v", got[1406], got[1401])
	}
}

func TestBuildBookingStandard(t *testing.T) {
	rules, _ := ParseBookingRules([]byte(`{"vorsteuer_konten":{"19":1406},"regeln":[{"kategorie":"standard","name":"Standard"}]}`))
	lines := []TaxLine{{Netto: 100, SatzProzent: 19, MwStBetrag: 19}}
	b, err := BuildBooking(rules, "standard", lines, 0, 6815, 1800)
	if err != nil {
		t.Fatal(err)
	}
	got := map[int]float64{}
	for _, e := range b.Entries {
		if e.Soll {
			got[e.Konto] += e.Betrag
		}
	}
	if !almost(got[6815], 100) || !almost(got[1406], 19) || !almost(b.HabenSum(), 119) {
		t.Errorf("standard booking wrong: %+v", b)
	}
	if _, err := BuildBooking(rules, "unbekannt", lines, 0, 6815, 1800); err == nil {
		t.Error("unknown category should error")
	}
}
