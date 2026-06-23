package core

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
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
	Manuell bool           `json:"manuell,omitempty"` // true = hand-edited, not auto-generated
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

// MarshalBooking encodes a booking as compact JSON ("" when empty).
func MarshalBooking(b Booking) string {
	if b.IsEmpty() {
		return ""
	}
	data, err := json.Marshal(b)
	if err != nil {
		return ""
	}
	return string(data)
}

// ParseBooking decodes a booking from JSON ("" / invalid → empty Booking).
func ParseBooking(s string) Booking {
	s = strings.TrimSpace(s)
	if s == "" {
		return Booking{}
	}
	var b Booking
	if err := json.Unmarshal([]byte(s), &b); err != nil {
		return Booking{}
	}
	return b
}

// round2 rounds a float64 to 2 decimal places.
func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

// PaymentEntry returns the single Haben (credit) entry of the booking — the
// Zahlungskonto side. ok is false unless there is exactly one Haben entry.
func (b Booking) PaymentEntry() (BookingEntry, bool) {
	var found BookingEntry
	n := 0
	for _, e := range b.Entries {
		if !e.Soll {
			found = e
			n++
		}
	}
	if n != 1 {
		return BookingEntry{}, false
	}
	return found, true
}

// PaymentAndCounters splits a booking into its single payment/base entry and
// the counter entries that post against it. For an incoming invoice the base is
// the single Haben (Zahlungskonto); for a revenue invoice (isRevenue) it is the
// single Soll. ok is false unless the base side has exactly one entry and there
// is at least one counter — the exporters skip the booking in that case.
func (b Booking) PaymentAndCounters(isRevenue bool) (BookingEntry, []BookingEntry, bool) {
	var base BookingEntry
	baseCount := 0
	counters := make([]BookingEntry, 0, len(b.Entries))
	for _, e := range b.Entries {
		isBase := (isRevenue && e.Soll) || (!isRevenue && !e.Soll)
		if isBase {
			base = e
			baseCount++
		} else {
			counters = append(counters, e)
		}
	}
	if baseCount != 1 || len(counters) == 0 {
		return BookingEntry{}, nil, false
	}
	return base, counters, true
}

// WithSettlementAccount returns a copy of a revenue booking with its single Soll
// (receivable) entry's account changed to bankKonto — used when the incoming
// payment of an outgoing invoice is reconciled (Forderung → Bank). No-op unless
// there is exactly one Soll entry.
func (b Booking) WithSettlementAccount(bankKonto int) Booking {
	sollCount := 0
	for _, e := range b.Entries {
		if e.Soll {
			sollCount++
		}
	}
	if sollCount != 1 {
		return b
	}
	out := Booking{Info: b.Info, Manuell: b.Manuell, Entries: make([]BookingEntry, len(b.Entries))}
	copy(out.Entries, b.Entries)
	for i := range out.Entries {
		if out.Entries[i].Soll {
			out.Entries[i].Konto = bankKonto
		}
	}
	return out
}

// DebitEntries returns the Soll (debit) entries — the expense/Vorsteuer lines.
func (b Booking) DebitEntries() []BookingEntry {
	out := make([]BookingEntry, 0, len(b.Entries))
	for _, e := range b.Entries {
		if e.Soll {
			out = append(out, e)
		}
	}
	return out
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
	case "reverse_charge":
		net := round2(SumNetto(lines) + trinkgeld)
		vat := round2(net * rule.RcSatz / 100)
		return Booking{Entries: []BookingEntry{
			{Konto: expenseAccount, Betrag: net, Soll: true},
			{Konto: rule.KontoVStRC, Betrag: vat, Soll: true},
			{Konto: rule.KontoUStRC, Betrag: vat, Soll: false},
			{Konto: paymentAccount, Betrag: net, Soll: false},
		}}, nil
	case "geschenke":
		if netTotal > rule.Schwelle {
			gross := round2(netTotal + SumMwSt(lines))
			return Booking{Entries: []BookingEntry{
				{Konto: rule.KontoNichtAbziehbar, Betrag: gross, Soll: true},
				{Konto: paymentAccount, Betrag: gross, Soll: false},
			}}, nil
		}
		entries = append(entries, BookingEntry{Konto: rule.KontoAbziehbar, Betrag: netTotal, Soll: true})
	case "bewirtung":
		abz := round2(netTotal * rule.AbziehbarProzent / 100)
		nicht := round2(netTotal - abz)
		entries = append(entries,
			BookingEntry{Konto: rule.KontoAbziehbar, Betrag: abz, Soll: true},
			BookingEntry{Konto: rule.KontoNichtAbziehbar, Betrag: nicht, Soll: true},
		)
	case "reisekosten", "kfz":
		entries = append(entries, BookingEntry{Konto: rule.DefaultKonto, Betrag: netTotal, Soll: true})
	case "standard":
		entries = append(entries, BookingEntry{Konto: expenseAccount, Betrag: netTotal, Soll: true})
	default:
		return Booking{}, fmt.Errorf("Buchungskategorie ohne Buchungslogik: %s", kategorie)
	}

	// Vorsteuer per rate (Soll), for the categories that fall through here.
	for _, l := range lines {
		if l.MwStBetrag == 0 {
			continue
		}
		if konto, ok := rules.VorsteuerKonto(l.SatzProzent); ok {
			entries = append(entries, BookingEntry{Konto: konto, Betrag: round2(l.MwStBetrag), Soll: true})
		}
	}

	// Payment (Haben) = Σ Soll, so the booking always balances.
	var sollSum float64
	for _, e := range entries {
		sollSum += e.Betrag
	}
	entries = append(entries, BookingEntry{Konto: paymentAccount, Betrag: round2(sollSum), Soll: false})
	return Booking{Entries: entries}, nil
}

// BuildRevenueBooking turns an outgoing invoice's tax lines into a balanced
// revenue Booking: Soll paymentAccount (gross received), Haben revenueAccount
// (net) + Umsatzsteuer per rate. The mirror of BuildBooking. paymentAccount is
// computed as the sum of the Haben side, so the booking always balances even if
// a rate's Umsatzsteuer account is unconfigured.
func BuildRevenueBooking(rules *BookingRules, lines []TaxLine, revenueAccount, paymentAccount int) (Booking, error) {
	if len(lines) == 0 {
		return Booking{}, fmt.Errorf("keine Steuerzeilen für Erlösbuchung")
	}
	entries := []BookingEntry{
		{Konto: revenueAccount, Betrag: round2(SumNetto(lines)), Soll: false},
	}
	for _, l := range lines {
		if l.MwStBetrag == 0 {
			continue
		}
		if konto, ok := rules.UmsatzsteuerKonto(l.SatzProzent); ok {
			entries = append(entries, BookingEntry{Konto: konto, Betrag: round2(l.MwStBetrag), Soll: false})
		}
	}
	var habenSum float64
	for _, e := range entries {
		habenSum += e.Betrag
	}
	// Payment (Soll) = Σ Haben, prepended so it reads payment-first.
	// Use ForderungsKonto (receivable) for Soll-Besteuerung if set, else paymentAccount (cash-basis fallback).
	sollKonto := paymentAccount
	if rules.ForderungsKonto != 0 {
		sollKonto = rules.ForderungsKonto
	}
	entries = append([]BookingEntry{{Konto: sollKonto, Betrag: round2(habenSum), Soll: true}}, entries...)
	return Booking{Entries: entries}, nil
}
