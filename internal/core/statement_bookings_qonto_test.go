package core

import (
	"testing"
)

// sampleQontoText mirrors the Qonto statement format described in the feature spec.
const sampleQontoText = `Kontoauszüge
Vom 01/04/2026 bis zum 30/04/2026
Kontostand am 01/04 + 32406.58 EUR
Eingänge + 75404.05 EUR
Ausgänge - 73123.42 EUR
Kontostand am 30/04 + 34687.21 EUR
Abrechnungstag Transaktionen Belastung Gutschrift
01/04 Qonto
Abonnement / Zusatzgebühren
- 108.00 EUR
02/04 CLAUDE.AI SUBSCRIPTION
Karte **6868
- 18.00 EUR
09/04 CLAUDE.AI SUBSCRIPTION
1.15220647540039 USD = 1.00 EUR
Karte **6868
- 86.79 EUR
- 100.00 USD
23/04 SW Operations GmbH
Re-Nr.:20148 abzgl. IDA/Screenway 19580,- netto
+ 75404.05 EUR
24/04 euhost.com, Matevz Sernc-Urban
Rechnung 17681 vom 16.04.2026. Netto EUR 15.000,00.
- 17850.00 EUR
`

func TestParseQontoStatement_Count(t *testing.T) {
	got := parseQontoStatement(sampleQontoText)
	// Expected transactions: 01/04 Qonto, 02/04 CLAUDE.AI, 09/04 CLAUDE.AI, 23/04 SW Operations, 24/04 euhost
	if len(got) != 5 {
		t.Fatalf("want 5 transactions, got %d: %+v", len(got), got)
	}
}

func TestParseQontoStatement_FirstTransaction(t *testing.T) {
	got := parseQontoStatement(sampleQontoText)
	first := got[0]
	if first.Date != "01.04.2026" {
		t.Errorf("first.Date = %q, want %q", first.Date, "01.04.2026")
	}
	if first.Betrag != 108.00 {
		t.Errorf("first.Betrag = %v, want 108.00", first.Betrag)
	}
	if first.IstGutschrift {
		t.Errorf("first.IstGutschrift = true, want false (debit)")
	}
}

func TestParseQontoStatement_USDLineIgnored(t *testing.T) {
	// The 09/04 CLAUDE.AI transaction has both a EUR amount (86.79) and a USD amount (100.00).
	// Only the EUR amount (first EUR line) should be used; USD line must be ignored.
	got := parseQontoStatement(sampleQontoText)
	// 09/04 is index 2 (0-based)
	claudeUSD := got[2]
	if claudeUSD.Date != "09.04.2026" {
		t.Errorf("got[2].Date = %q, want %q", claudeUSD.Date, "09.04.2026")
	}
	if claudeUSD.Betrag != 86.79 {
		t.Errorf("got[2].Betrag = %v, want 86.79 (EUR, not 100 USD)", claudeUSD.Betrag)
	}
	if claudeUSD.IstGutschrift {
		t.Errorf("got[2].IstGutschrift = true, want false")
	}
}

func TestParseQontoStatement_CreditTransaction(t *testing.T) {
	// 23/04 SW Operations GmbH → credit of 75404.05 EUR
	got := parseQontoStatement(sampleQontoText)
	swOps := got[3]
	if swOps.Date != "23.04.2026" {
		t.Errorf("got[3].Date = %q, want %q", swOps.Date, "23.04.2026")
	}
	if swOps.Betrag != 75404.05 {
		t.Errorf("got[3].Betrag = %v, want 75404.05", swOps.Betrag)
	}
	if !swOps.IstGutschrift {
		t.Errorf("got[3].IstGutschrift = false, want true (credit)")
	}
}

func TestParseQontoStatement_LastTransaction(t *testing.T) {
	// 24/04 euhost → debit of 17850.00 EUR
	got := parseQontoStatement(sampleQontoText)
	euhost := got[4]
	if euhost.Date != "24.04.2026" {
		t.Errorf("got[4].Date = %q, want %q", euhost.Date, "24.04.2026")
	}
	if euhost.Betrag != 17850.00 {
		t.Errorf("got[4].Betrag = %v, want 17850.00", euhost.Betrag)
	}
	if euhost.IstGutschrift {
		t.Errorf("got[4].IstGutschrift = true, want false (debit)")
	}
}

func TestParseQontoStatement_NoFalsePositivesFromHeaders(t *testing.T) {
	// Summary lines (Kontostand, Eingänge, Ausgänge, Abrechnungstag) must NOT
	// appear as transactions, even though they contain amounts.
	got := parseQontoStatement(sampleQontoText)
	for _, b := range got {
		if b.Date == "01.04." || b.Date == "30.04." {
			t.Errorf("header Kontostand line was emitted as transaction: %+v", b)
		}
	}
}

func TestParseQontoAmount_DotDecimal(t *testing.T) {
	cases := []struct {
		in   string
		want float64
	}{
		{"108.00", 108.00},
		{"75404.05", 75404.05},
		{"86.79", 86.79},
		{"17850.00", 17850.00},
		{"18.00", 18.00},
	}
	for _, c := range cases {
		got := parseQontoAmount(c.in)
		if got != c.want {
			t.Errorf("parseQontoAmount(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestParseQontoAmount_GermanFormat(t *testing.T) {
	cases := []struct {
		in   string
		want float64
	}{
		{"1.793,68", 1793.68},
		{"2.000,00", 2000.00},
		{"1.234,56", 1234.56},
	}
	for _, c := range cases {
		got := parseQontoAmount(c.in)
		if got != c.want {
			t.Errorf("parseQontoAmount(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestParseQontoStatement_LineIdxMonotone(t *testing.T) {
	got := parseQontoStatement(sampleQontoText)
	for i, b := range got {
		if b.LineIdx != i+1 {
			t.Errorf("got[%d].LineIdx = %d, want %d", i, b.LineIdx, i+1)
		}
	}
}

func TestParseQontoStatement_YearFallback(t *testing.T) {
	// Text without a "Vom DD/MM/YYYY" header → dates should still parse but year is empty.
	text := `Abrechnungstag Transaktionen Belastung Gutschrift
01/04 Testfirma
- 50.00 EUR
`
	got := parseQontoStatement(text)
	if len(got) != 1 {
		t.Fatalf("want 1 transaction, got %d", len(got))
	}
	// Without year we produce "01.04." (no year suffix)
	if got[0].Date != "01.04." {
		t.Errorf("Date without year = %q, want %q", got[0].Date, "01.04.")
	}
}
