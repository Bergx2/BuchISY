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

func TestComputeYearOverviewStrayLaterBook(t *testing.T) {
	// Jan anchors at 3497.35; May spends 16.68; June has a STRAY stored book
	// with opening 0 (saved before the carry chain existed). The June 0 must be
	// ignored — June opens with May's close (3480.67), not reset to 0.
	months := make([]MonthInput, 12)
	months[0] = MonthInput{HasStoredBook: true, Book: CashBook{Konto: "Barkasse", Anfangsbestand: 3497.35}}
	months[4] = MonthInput{Invoices: []CSVRow{{Bruttobetrag: 16.68, Bezahldatum: "10.05.2026"}}} // May
	months[5] = MonthInput{HasStoredBook: true, Book: CashBook{Konto: "Barkasse", Anfangsbestand: 0}, // June stray 0-book
		Invoices: []CSVRow{{Bruttobetrag: 15.16, Bezahldatum: "15.06.2026"}}}

	got := ComputeYearOverview(0, months)
	if got[4].Endbestand != 3480.67 {
		t.Errorf("May Endbestand = %v, want 3480.67", got[4].Endbestand)
	}
	if got[5].Anfangsbestand != 3480.67 {
		t.Errorf("June opening = %v, want 3480.67 (carried, stray 0-book ignored)", got[5].Anfangsbestand)
	}
	if got[5].Endbestand != 3465.51 {
		t.Errorf("June Endbestand = %v, want 3465.51 (not negative)", got[5].Endbestand)
	}
	if got[6].Anfangsbestand != 3465.51 {
		t.Errorf("July opening = %v, want 3465.51", got[6].Anfangsbestand)
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
