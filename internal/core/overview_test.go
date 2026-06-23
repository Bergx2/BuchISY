package core

import (
	"testing"
)

func TestOverviewKPIs(t *testing.T) {
	// Row 1: booked and linked (Bankkonto set, BuchungRef set) — no warnings expected
	// We give it a valid Gegenkonto to avoid the "Kein Gegenkonto" warning.
	// Brutto = Netto + USt so the check passes.
	row1 := CSVRow{
		BetragNetto:      100.00,
		SteuersatzBetrag: 19.00,
		Bruttobetrag:     119.00,
		Ausgangsrechnung: false,
		Bankkonto:        "Sparkasse",
		BuchungRef:       "stmt_jan.pdf|0|3",
		Gegenkonto:       4920,
		Rechnungsdatum:   "01.01.2026",
	}

	// Row 2: bank account but no BuchungRef (open reconcile) + triggers a warning
	// (Bruttobetrag = 0 → "Bruttobetrag fehlt oder ist 0" + "Kein Gegenkonto")
	row2 := CSVRow{
		BetragNetto:      50.00,
		SteuersatzBetrag: 9.50,
		Bruttobetrag:     0, // deliberately wrong → triggers warning
		Ausgangsrechnung: false,
		Bankkonto:        "Kreditkarte",
		BuchungRef:       "", // open
		Gegenkonto:       0,  // triggers warning
		Rechnungsdatum:   "15.01.2026",
	}

	rows := []CSVRow{row1, row2}
	kpi := OverviewKPIs(rows)

	if kpi.Count != 2 {
		t.Errorf("Count: got %d, want 2", kpi.Count)
	}

	wantNetto := 150.00
	if kpi.Netto != wantNetto {
		t.Errorf("Netto: got %.2f, want %.2f", kpi.Netto, wantNetto)
	}

	wantUSt := 28.50
	if kpi.USt != wantUSt {
		t.Errorf("USt: got %.2f, want %.2f", kpi.USt, wantUSt)
	}

	wantBrutto := 119.00 // row2 Brutto is 0
	if kpi.Brutto != wantBrutto {
		t.Errorf("Brutto: got %.2f, want %.2f", kpi.Brutto, wantBrutto)
	}

	// OpenReconcile: row2 has Bankkonto set and BuchungRef == ""
	if kpi.OpenReconcile != 1 {
		t.Errorf("OpenReconcile: got %d, want 1", kpi.OpenReconcile)
	}

	// Warnings: row2 has at least one warning (Bruttobetrag=0, Gegenkonto=0)
	if kpi.Warnings < 1 {
		t.Errorf("Warnings: got %d, want >= 1", kpi.Warnings)
	}
}

func TestOverviewKPIsZahllast(t *testing.T) {
	// Zahllast = Σ SteuersatzBetrag(Ausgangsrechnung) − Σ SteuersatzBetrag(!Ausgangsrechnung)
	out := CSVRow{
		BetragNetto:      200.00,
		SteuersatzBetrag: 38.00,
		Bruttobetrag:     238.00,
		Ausgangsrechnung: true,
		Gegenkonto:       8400,
		Rechnungsdatum:   "01.01.2026",
	}
	in := CSVRow{
		BetragNetto:      100.00,
		SteuersatzBetrag: 19.00,
		Bruttobetrag:     119.00,
		Ausgangsrechnung: false,
		Gegenkonto:       4920,
		Rechnungsdatum:   "05.01.2026",
	}

	kpi := OverviewKPIs([]CSVRow{out, in})
	wantZahllast := 38.00 - 19.00 // 19.00
	if kpi.Zahllast != wantZahllast {
		t.Errorf("Zahllast: got %.2f, want %.2f", kpi.Zahllast, wantZahllast)
	}
}

func TestOverviewKPIsEmpty(t *testing.T) {
	kpi := OverviewKPIs(nil)
	if kpi.Count != 0 || kpi.Netto != 0 || kpi.Brutto != 0 {
		t.Errorf("empty rows: got non-zero KPI: %+v", kpi)
	}
}
