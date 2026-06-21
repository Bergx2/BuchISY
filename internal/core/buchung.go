package core

import "math"

// BookingEntry is one line of a double-entry booking: an amount posted to an
// account on the debit (Soll=true) or credit (Soll=false) side.
type BookingEntry struct {
	Konto            int     `json:"konto"`
	Betrag           float64 `json:"betrag"`
	Soll             bool    `json:"soll"` // true = Soll (debit), false = Haben (credit)
	Steuerschluessel string  `json:"steuerschluessel,omitempty"`
}

// Booking is the set of entries that posts a single receipt, plus a free-text
// rationale/notes ("Buchungswissen").
type Booking struct {
	Entries []BookingEntry `json:"entries,omitempty"`
	Info    string         `json:"info,omitempty"`
}

// SollSum returns the total of the debit entries.
func (b Booking) SollSum() float64 {
	var s float64
	for _, e := range b.Entries {
		if e.Soll {
			s += e.Betrag
		}
	}
	return s
}

// HabenSum returns the total of the credit entries.
func (b Booking) HabenSum() float64 {
	var s float64
	for _, e := range b.Entries {
		if !e.Soll {
			s += e.Betrag
		}
	}
	return s
}

// Balanced reports whether debits equal credits (within rounding) and there is
// at least one entry.
func (b Booking) Balanced() bool {
	return len(b.Entries) > 0 && math.Abs(b.SollSum()-b.HabenSum()) < 0.005
}

// IsEmpty reports whether the booking carries no entries and no info.
func (b Booking) IsEmpty() bool {
	return len(b.Entries) == 0 && b.Info == ""
}
