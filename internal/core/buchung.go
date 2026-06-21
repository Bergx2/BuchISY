package core

import (
	"fmt"
	"math"
)

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

// round2 rounds a float64 to 2 decimal places.
func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

// BuildBooking turns a receipt's tax lines into a balanced Booking per the
// category rule. expenseAccount is the Soll account for the "standard" case;
// paymentAccount is the Haben account (Zahlungskonto). Returns an error for an
// unknown category.
func BuildBooking(rules *BookingRules, kategorie string, lines []TaxLine, trinkgeld float64, expenseAccount, paymentAccount int) (Booking, error) {
	rule, ok := rules.Rule(kategorie)
	if !ok {
		return Booking{}, fmt.Errorf("unbekannte Buchungskategorie: %s", kategorie)
	}

	netTotal := round2(SumNetto(lines) + trinkgeld)
	var entries []BookingEntry

	switch kategorie {
	case "bewirtung":
		abz := round2(netTotal * rule.AbziehbarProzent / 100)
		nicht := round2(netTotal - abz)
		entries = append(entries,
			BookingEntry{Konto: rule.KontoAbziehbar, Betrag: abz, Soll: true},
			BookingEntry{Konto: rule.KontoNichtAbziehbar, Betrag: nicht, Soll: true},
		)
	default: // "standard"
		entries = append(entries, BookingEntry{Konto: expenseAccount, Betrag: netTotal, Soll: true})
	}

	// Vorsteuer per rate (Soll).
	for _, l := range lines {
		if l.MwStBetrag == 0 {
			continue
		}
		if konto, ok := rules.VorsteuerKonto(l.SatzProzent); ok {
			entries = append(entries, BookingEntry{Konto: konto, Betrag: round2(l.MwStBetrag), Soll: true})
		}
	}

	// Payment (Haben) = sum of all Soll entries, ensuring Σ Soll == Σ Haben by construction.
	var sollSum float64
	for _, e := range entries {
		sollSum += e.Betrag
	}
	entries = append(entries, BookingEntry{Konto: paymentAccount, Betrag: round2(sollSum), Soll: false})

	return Booking{Entries: entries}, nil
}
