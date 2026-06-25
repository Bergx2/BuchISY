package core

import (
	"testing"
)

// TestRowEUR_USDConversion verifies that a USD row is correctly converted to EUR.
func TestRowEUR_USDConversion(t *testing.T) {
	row := CSVRow{
		Waehrung:        "USD",
		Wechselkurs:     1.1720, // 1 EUR = 1.1720 USD
		BetragNetto:     168.09,
		SteuersatzBetrag: 31.94,
		Bruttobetrag:    200.00,
		BetragNetto_EUR: 168.09,
		Gebuehr:         2.34,
		Trinkgeld:       1.17,
		Rabatt:          5.86,
	}

	got, rateMissing := RowEUR(row)

	if rateMissing {
		t.Fatal("rateMissing should be false for a row with a valid kurs")
	}
	if got.Waehrung != "EUR" {
		t.Errorf("Waehrung = %q, want %q", got.Waehrung, "EUR")
	}
	if got.Wechselkurs != 0 {
		t.Errorf("Wechselkurs = %v, want 0", got.Wechselkurs)
	}

	// BetragNetto: round2(168.09 / 1.1720) ≈ 143.42
	wantNetto := round2(168.09 / 1.1720)
	if got.BetragNetto != wantNetto {
		t.Errorf("BetragNetto = %v, want %v", got.BetragNetto, wantNetto)
	}

	// Bruttobetrag: round2(200.00 / 1.1720) ≈ 170.65
	wantBrutto := round2(200.00 / 1.1720)
	if got.Bruttobetrag != wantBrutto {
		t.Errorf("Bruttobetrag = %v, want %v", got.Bruttobetrag, wantBrutto)
	}

	// Gebuehr: round2(2.34 / 1.1720) ≈ 2.00
	wantGebuehr := round2(2.34 / 1.1720)
	if got.Gebuehr != wantGebuehr {
		t.Errorf("Gebuehr = %v, want %v", got.Gebuehr, wantGebuehr)
	}

	// Trinkgeld: round2(1.17 / 1.1720) ≈ 1.00
	wantTrinkgeld := round2(1.17 / 1.1720)
	if got.Trinkgeld != wantTrinkgeld {
		t.Errorf("Trinkgeld = %v, want %v", got.Trinkgeld, wantTrinkgeld)
	}

	// Rabatt: round2(5.86 / 1.1720) ≈ 5.00
	wantRabatt := round2(5.86 / 1.1720)
	if got.Rabatt != wantRabatt {
		t.Errorf("Rabatt = %v, want %v", got.Rabatt, wantRabatt)
	}
}

// TestRowEUR_ForeignNoRate verifies that a foreign row without a rate is passed
// through at face value with rateMissing=true.
func TestRowEUR_ForeignNoRate(t *testing.T) {
	row := CSVRow{
		Waehrung:     "USD",
		Wechselkurs:  0, // no rate
		BetragNetto:  100.00,
		Bruttobetrag: 119.00,
	}

	got, rateMissing := RowEUR(row)

	if !rateMissing {
		t.Fatal("rateMissing should be true for a foreign row without a rate")
	}
	// Amounts must be left unchanged
	if got.BetragNetto != 100.00 {
		t.Errorf("BetragNetto = %v, want 100.00 (face value)", got.BetragNetto)
	}
	if got.Bruttobetrag != 119.00 {
		t.Errorf("Bruttobetrag = %v, want 119.00 (face value)", got.Bruttobetrag)
	}
	// Waehrung / Wechselkurs untouched
	if got.Waehrung != "USD" {
		t.Errorf("Waehrung = %q, want %q", got.Waehrung, "USD")
	}
}

// TestRowEUR_EURUnchanged verifies that a EUR row is returned unchanged.
func TestRowEUR_EURUnchanged(t *testing.T) {
	row := CSVRow{
		Waehrung:     "EUR",
		Wechselkurs:  0,
		BetragNetto:  84.03,
		Bruttobetrag: 100.00,
	}

	got, rateMissing := RowEUR(row)

	if rateMissing {
		t.Fatal("rateMissing should be false for a EUR row")
	}
	if got.BetragNetto != 84.03 {
		t.Errorf("BetragNetto = %v, want 84.03", got.BetragNetto)
	}
	if got.Bruttobetrag != 100.00 {
		t.Errorf("Bruttobetrag = %v, want 100.00", got.Bruttobetrag)
	}
	if got.Waehrung != "EUR" {
		t.Errorf("Waehrung = %q, want EUR", got.Waehrung)
	}
}

// TestRowEUR_EmptyWaehrung verifies that a row with no currency is treated as EUR.
func TestRowEUR_EmptyWaehrung(t *testing.T) {
	row := CSVRow{
		Waehrung:     "",
		Wechselkurs:  0,
		BetragNetto:  50.00,
		Bruttobetrag: 59.50,
	}

	got, rateMissing := RowEUR(row)

	if rateMissing {
		t.Fatal("rateMissing should be false for a row with empty Waehrung (treated as EUR)")
	}
	if got.BetragNetto != 50.00 {
		t.Errorf("BetragNetto = %v, want 50.00", got.BetragNetto)
	}
}

// TestRowsEUR verifies that RowsEUR maps RowEUR correctly over a slice.
func TestRowsEUR(t *testing.T) {
	rows := []CSVRow{
		{Waehrung: "EUR", BetragNetto: 84.03, Bruttobetrag: 100.00},
		{Waehrung: "USD", Wechselkurs: 1.1720, BetragNetto: 200.00, Bruttobetrag: 238.00},
		{Waehrung: "CHF", Wechselkurs: 0, BetragNetto: 50.00, Bruttobetrag: 54.00}, // rate missing
	}

	got := RowsEUR(rows)

	if len(got) != 3 {
		t.Fatalf("len(got) = %d, want 3", len(got))
	}
	// EUR row unchanged
	if got[0].BetragNetto != 84.03 {
		t.Errorf("row[0].BetragNetto = %v, want 84.03", got[0].BetragNetto)
	}
	// USD row converted
	wantUSD := round2(200.00 / 1.1720)
	if got[1].BetragNetto != wantUSD {
		t.Errorf("row[1].BetragNetto = %v, want %v", got[1].BetragNetto, wantUSD)
	}
	if got[1].Waehrung != "EUR" {
		t.Errorf("row[1].Waehrung = %q, want EUR", got[1].Waehrung)
	}
	// CHF row (rate missing) — face value unchanged
	if got[2].BetragNetto != 50.00 {
		t.Errorf("row[2].BetragNetto = %v, want 50.00 (face value, rate missing)", got[2].BetragNetto)
	}
}

// TestTaxLinesEUR_Conversion verifies that TaxLinesEUR converts Netto + MwStBetrag.
func TestTaxLinesEUR_Conversion(t *testing.T) {
	lines := []TaxLine{
		{Netto: 168.09, SatzProzent: 19, MwStBetrag: 31.94},
	}

	got := TaxLinesEUR(lines, "USD", 1.1720)

	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	wantNetto := round2(168.09 / 1.1720)
	if got[0].Netto != wantNetto {
		t.Errorf("Netto = %v, want %v", got[0].Netto, wantNetto)
	}
	wantMwSt := round2(31.94 / 1.1720)
	if got[0].MwStBetrag != wantMwSt {
		t.Errorf("MwStBetrag = %v, want %v", got[0].MwStBetrag, wantMwSt)
	}
	// SatzProzent must be unchanged
	if got[0].SatzProzent != 19 {
		t.Errorf("SatzProzent = %v, want 19", got[0].SatzProzent)
	}
}

// TestTaxLinesEUR_EURUnchanged verifies that EUR lines pass through unchanged.
func TestTaxLinesEUR_EURUnchanged(t *testing.T) {
	lines := []TaxLine{
		{Netto: 84.03, SatzProzent: 19, MwStBetrag: 15.97},
	}

	got := TaxLinesEUR(lines, "EUR", 0)

	if got[0].Netto != 84.03 {
		t.Errorf("Netto = %v, want 84.03", got[0].Netto)
	}
	if got[0].MwStBetrag != 15.97 {
		t.Errorf("MwStBetrag = %v, want 15.97", got[0].MwStBetrag)
	}
}

// TestTaxLinesEUR_NoRate verifies that foreign lines with no rate pass through unchanged.
func TestTaxLinesEUR_NoRate(t *testing.T) {
	lines := []TaxLine{
		{Netto: 100.00, SatzProzent: 7, MwStBetrag: 7.00},
	}

	got := TaxLinesEUR(lines, "USD", 0)

	if got[0].Netto != 100.00 {
		t.Errorf("Netto = %v, want 100.00 (unchanged, no rate)", got[0].Netto)
	}
}
