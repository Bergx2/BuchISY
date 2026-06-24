package core

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

// --- AutobookPlausible ---

func okMeta() Meta {
	return Meta{
		TaxLines:     []TaxLine{{Netto: 100, MwStBetrag: 19, SatzProzent: 19}},
		Gegenkonto:   4920,
		Bruttobetrag: 119,
		Waehrung:     "EUR",
	}
}

func TestAutobookPlausible_OK(t *testing.T) {
	if !AutobookPlausible(okMeta()) {
		t.Fatal("expected true for a clean EUR invoice")
	}
}

func TestAutobookPlausible_WithTip(t *testing.T) {
	m := okMeta()
	m.Bruttobetrag = 124  // 119 + 5 tip
	m.Trinkgeld = 5
	if !AutobookPlausible(m) {
		t.Fatal("expected true when Trinkgeld is included in gross")
	}
}

func TestAutobookPlausible_NoTaxLines(t *testing.T) {
	m := okMeta()
	m.TaxLines = nil
	if AutobookPlausible(m) {
		t.Fatal("expected false: no tax lines")
	}
}

func TestAutobookPlausible_ZeroGegenkonto(t *testing.T) {
	m := okMeta()
	m.Gegenkonto = 0
	if AutobookPlausible(m) {
		t.Fatal("expected false: Gegenkonto=0")
	}
}

func TestAutobookPlausible_ZeroBrutto(t *testing.T) {
	m := okMeta()
	m.Bruttobetrag = 0
	m.TaxLines = []TaxLine{{Netto: 0, MwStBetrag: 0, SatzProzent: 19}}
	if AutobookPlausible(m) {
		t.Fatal("expected false: Bruttobetrag=0")
	}
}

func TestAutobookPlausible_GrossMismatch(t *testing.T) {
	m := okMeta()
	m.Bruttobetrag = 130 // too far off 119
	if AutobookPlausible(m) {
		t.Fatal("expected false: gross mismatch > 0.02")
	}
}

func TestAutobookPlausible_ForeignCurrencyNoRate(t *testing.T) {
	m := okMeta()
	m.Waehrung = "USD"
	m.Wechselkurs = 0 // missing rate
	if AutobookPlausible(m) {
		t.Fatal("expected false: foreign currency with no exchange rate")
	}
}

func TestAutobookPlausible_ForeignCurrencyWithRate(t *testing.T) {
	m := okMeta()
	m.Waehrung = "USD"
	m.Wechselkurs = 1.08
	if !AutobookPlausible(m) {
		t.Fatal("expected true: foreign currency but rate provided")
	}
}

func TestAutobookPlausible_BorderlineTolerance(t *testing.T) {
	m := okMeta()
	// diff exactly 0.02 — within tolerance
	m.Bruttobetrag = math.Round((119+0.02)*100) / 100
	if !AutobookPlausible(m) {
		t.Fatal("expected true: diff == 0.02 is within tolerance")
	}

	// diff 0.03 — out of tolerance
	m.Bruttobetrag = math.Round((119+0.03)*100) / 100
	if AutobookPlausible(m) {
		t.Fatal("expected false: diff == 0.03 exceeds tolerance")
	}
}

// --- MatchAutobookRule ---

func tempStore(t *testing.T) *BookingTemplateStore {
	t.Helper()
	dir := t.TempDir()
	s := NewBookingTemplateStore(dir)
	return s
}

func TestMatchAutobookRule_MatchAndEnabled(t *testing.T) {
	s := tempStore(t)
	if err := s.Set("ACME GmbH", BookingTemplate{Kategorie: "standard", ExpenseKonto: 4920, Autobook: true}); err != nil {
		t.Fatal(err)
	}
	tpl, ok := MatchAutobookRule("ACME GmbH", s)
	if !ok {
		t.Fatal("expected match")
	}
	if !tpl.Autobook {
		t.Fatal("expected Autobook=true")
	}
}

func TestMatchAutobookRule_MatchButDisabled(t *testing.T) {
	s := tempStore(t)
	if err := s.Set("ACME GmbH", BookingTemplate{Kategorie: "standard", ExpenseKonto: 4920, Autobook: false}); err != nil {
		t.Fatal(err)
	}
	_, ok := MatchAutobookRule("ACME GmbH", s)
	if ok {
		t.Fatal("expected no match: Autobook=false")
	}
}

func TestMatchAutobookRule_NoTemplate(t *testing.T) {
	s := tempStore(t)
	_, ok := MatchAutobookRule("Unknown GmbH", s)
	if ok {
		t.Fatal("expected no match: company not in store")
	}
}

func TestMatchAutobookRule_PersistRoundtrip(t *testing.T) {
	dir := t.TempDir()
	s1 := NewBookingTemplateStore(dir)
	if err := s1.Set("Lieferant AG", BookingTemplate{Kategorie: "standard", ExpenseKonto: 4910, Autobook: true}); err != nil {
		t.Fatal(err)
	}

	// Load a fresh store from the same dir — Autobook must survive serialisation.
	s2 := NewBookingTemplateStore(dir)
	if err := s2.Load(); err != nil {
		t.Fatal(err)
	}
	// Verify the file exists (sanity check).
	if _, err := os.Stat(filepath.Join(dir, "booking_templates.json")); err != nil {
		t.Fatal("booking_templates.json not found:", err)
	}
	tpl, ok := MatchAutobookRule("Lieferant AG", s2)
	if !ok {
		t.Fatal("expected match after round-trip")
	}
	if !tpl.Autobook {
		t.Fatal("Autobook flag lost after JSON round-trip")
	}
}
