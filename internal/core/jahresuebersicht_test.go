package core

import (
	"testing"
	"time"
)

func TestComputeYearOverview(t *testing.T) {
	months := make([]MonthInput, 12)
	// January: stored book, opening 100, one deposit of 50.
	months[0] = MonthInput{
		HasStoredBook: true,
		Book: CashBook{Konto: "Barkasse", Anfangsbestand: 100, Einlagen: []CashDeposit{
			{Datum: "10.01.2026", Beschreibung: "Einlage", Betrag: 50},
		}},
	}
	// February: no stored book, one cash invoice of 30.
	months[1] = MonthInput{
		Invoices: []CSVRow{{Auftraggeber: "X", Bruttobetrag: 30, Bezahldatum: "05.02.2026"}},
	}
	// March..December: empty.

	got := ComputeYearOverview(999, months) // carriedIn ignored: Jan has a stored book.

	if len(got) != 12 {
		t.Fatalf("got %d summaries, want 12", len(got))
	}
	if got[0].Month != time.January {
		t.Errorf("month[0] = %v, want January", got[0].Month)
	}
	if got[0].Anfangsbestand != 100 {
		t.Errorf("Jan opening = %v, want 100 (stored book overrides carriedIn)", got[0].Anfangsbestand)
	}
	if got[0].Einnahmen != 50 || got[0].Ausgaben != 0 || got[0].Endbestand != 150 {
		t.Errorf("Jan = %+v, want Einnahmen 50 / Ausgaben 0 / Endbestand 150", got[0])
	}
	if got[1].Anfangsbestand != 150 {
		t.Errorf("Feb opening = %v, want 150 (carried from Jan)", got[1].Anfangsbestand)
	}
	if got[1].Ausgaben != 30 || got[1].Endbestand != 120 {
		t.Errorf("Feb = %+v, want Ausgaben 30 / Endbestand 120", got[1])
	}
	if got[2].Anfangsbestand != 120 || got[2].Endbestand != 120 {
		t.Errorf("Mar = %+v, want 120/120 (empty month carries forward)", got[2])
	}
}

func TestComputeYearOverviewCarriedIn(t *testing.T) {
	months := make([]MonthInput, 12) // all empty, no stored books
	got := ComputeYearOverview(3497.35, months)
	if got[0].Anfangsbestand != 3497.35 || got[0].Endbestand != 3497.35 {
		t.Errorf("Jan = %+v, want carriedIn 3497.35 unchanged", got[0])
	}
	if got[11].Endbestand != 3497.35 {
		t.Errorf("Dec Endbestand = %v, want 3497.35", got[11].Endbestand)
	}
}
