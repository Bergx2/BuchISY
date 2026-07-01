package core

import (
	"strings"
	"testing"
)

// Golden runs captured from a real Sparkasse "Umsätze - Druckansicht" export
// (Kautionskonto, 3 bookings). Each booking spans a label row (label + signed
// amount + merged date cell) and a "… | Wertstellung …" sub-row; the footer
// carries a print timestamp. The generic heuristic reported 7 phantom
// zero-amount rows for this; the dedicated parser must find exactly 3.
func druckansichtFixture() [][]htmlLine {
	return [][]htmlLine{{
		{top: 96.7, left: 69.8, text: "Umsätze - Druckansicht"},
		{top: 138.6, left: 421.9, text: "14.577,76 EUR *"}, // balance (no sign) — must be ignored
		{top: 219.9, left: 466.0, text: "14.577,76 EUR*"},  // balance
		{top: 244.1, left: 357.6, text: "BUCHUNG WERTSTELLUNG"},
		{top: 264.0, left: 63.6, text: "SOLIDARITAETSZUSCHLAG"},
		{top: 267.2, left: 491.6, text: "-0,73 EUR"},
		{top: 268.8, left: 357.6, text: "30.12.202501.01.2026"},
		{top: 275.6, left: 63.6, text: "30.12.2025 | Wertstellung 01.01.2026"},
		{top: 292.6, left: 63.6, text: "KAPITALERTRAGSTEUER"},
		{top: 295.8, left: 485.3, text: "-13,28 EUR"},
		{top: 297.5, left: 357.6, text: "30.12.202501.01.2026"},
		{top: 304.3, left: 63.6, text: "30.12.2025 | Wertstellung 01.01.2026"},
		{top: 321.3, left: 63.6, text: "ZINSEN"},
		{top: 324.5, left: 482.9, text: "+53,11 EUR"},
		{top: 326.1, left: 357.6, text: "30.12.202501.01.2026"},
		{top: 332.9, left: 63.6, text: "30.12.2025 | Wertstellung 01.01.2026"},
		{top: 359.5, left: 466.0, text: "14.538,66 EUR*"},    // balance
		{top: 832.0, left: 523.2, text: "24.05.2026, 13:16"}, // print timestamp — must be ignored
	}}
}

func TestParseSparkasseDruckansicht(t *testing.T) {
	got := parseSparkasseDruckansicht(druckansichtFixture())
	if len(got) != 3 {
		t.Fatalf("expected 3 bookings, got %d: %+v", len(got), got)
	}
	want := []struct {
		date   string
		betrag float64
		credit bool
		label  string
	}{
		{"30.12.2025", 0.73, false, "SOLIDARITAETSZUSCHLAG"},
		{"30.12.2025", 13.28, false, "KAPITALERTRAGSTEUER"},
		{"30.12.2025", 53.11, true, "ZINSEN"},
	}
	for i, w := range want {
		b := got[i]
		if b.Date != w.date || b.Betrag != w.betrag || b.IstGutschrift != w.credit {
			t.Errorf("[%d] got date=%q betrag=%.2f credit=%v; want date=%q betrag=%.2f credit=%v",
				i, b.Date, b.Betrag, b.IstGutschrift, w.date, w.betrag, w.credit)
		}
		if !strings.Contains(b.Text, w.label) {
			t.Errorf("[%d] text %q missing label %q", i, b.Text, w.label)
		}
	}
}

func TestIsSparkasseDruckansicht(t *testing.T) {
	if !isSparkasseDruckansicht("… Umsätze - Druckansicht …\nBUCHUNG WERTSTELLUNG\n…") {
		t.Error("should detect Druckansicht")
	}
	if isSparkasseDruckansicht("02.01.2025 LS-Einlösung SEPA\nBetrag EUR") {
		t.Error("classic statement must NOT be detected as Druckansicht")
	}
}
