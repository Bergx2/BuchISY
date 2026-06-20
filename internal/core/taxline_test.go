package core

import "testing"

func almost(a, b float64) bool { d := a - b; return d < 0.005 && d > -0.005 }

func TestTaxLineSums(t *testing.T) {
	lines := []TaxLine{
		{Netto: 14.20, SatzProzent: 19, MwStBetrag: 2.70},
		{Netto: 18.69, SatzProzent: 7, MwStBetrag: 1.31},
	}
	if !almost(SumNetto(lines), 32.89) {
		t.Errorf("SumNetto = %v, want 32.89", SumNetto(lines))
	}
	if !almost(SumMwSt(lines), 4.01) {
		t.Errorf("SumMwSt = %v, want 4.01", SumMwSt(lines))
	}
	if !almost(ComputeBrutto(lines, 2.00), 38.90) {
		t.Errorf("ComputeBrutto = %v, want 38.90", ComputeBrutto(lines, 2.00))
	}
	if PrimarySatz(lines) != 19 {
		t.Errorf("PrimarySatz = %v, want 19", PrimarySatz(lines))
	}
	if PrimarySatz(nil) != 0 {
		t.Errorf("PrimarySatz(nil) = %v, want 0", PrimarySatz(nil))
	}
}

func TestTaxLineJSONAndReconstruct(t *testing.T) {
	lines := []TaxLine{{Netto: 14.20, SatzProzent: 19, MwStBetrag: 2.70}}
	js := MarshalTaxLines(lines)
	got := ParseTaxLines(js)
	if len(got) != 1 || !almost(got[0].Netto, 14.20) || got[0].SatzProzent != 19 {
		t.Fatalf("round-trip failed: %q -> %+v", js, got)
	}
	if MarshalTaxLines(nil) != "" {
		t.Errorf("empty should marshal to empty string")
	}
	if ParseTaxLines("") != nil || ParseTaxLines("not json") != nil {
		t.Errorf("invalid JSON should parse to nil")
	}
	rc := ReconstructTaxLines(14.20, 19, 2.70)
	if len(rc) != 1 || rc[0].SatzProzent != 19 {
		t.Errorf("reconstruct = %+v", rc)
	}
	if ReconstructTaxLines(0, 0, 0) != nil {
		t.Errorf("all-zero reconstruct should be nil")
	}
}
