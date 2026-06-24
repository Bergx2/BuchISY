package core

import (
	"testing"
)

func TestComputeSuSa(t *testing.T) {
	// expense booking: Soll 4663 100 / Haben 1200 100
	// revenue booking: Soll 1200 200 / Haben 8400 200
	rows := []CSVRow{
		{Buchung: Booking{Entries: []BookingEntry{
			{Konto: 4663, Betrag: 100, Soll: true},
			{Konto: 1200, Betrag: 100, Soll: false},
		}}},
		{Buchung: Booking{Entries: []BookingEntry{
			{Konto: 1200, Betrag: 200, Soll: true},
			{Konto: 8400, Betrag: 200, Soll: false},
		}}},
	}

	bals := ComputeSuSa(rows, nil)

	// expect sorted by konto: 1200, 4663, 8400
	if len(bals) != 3 {
		t.Fatalf("expected 3 account balances, got %d", len(bals))
	}

	find := func(konto int) AccountBalance {
		for _, b := range bals {
			if b.Konto == konto {
				return b
			}
		}
		t.Fatalf("konto %d not found in SuSa", konto)
		return AccountBalance{}
	}

	// konto 4663: Soll 100, Haben 0, Saldo 100
	b4663 := find(4663)
	if b4663.SollSumme != 100 {
		t.Errorf("4663 SollSumme: got %.2f, want 100", b4663.SollSumme)
	}
	if b4663.HabenSumme != 0 {
		t.Errorf("4663 HabenSumme: got %.2f, want 0", b4663.HabenSumme)
	}
	if b4663.Saldo != 100 {
		t.Errorf("4663 Saldo: got %.2f, want 100", b4663.Saldo)
	}

	// konto 8400: Soll 0, Haben 200, Saldo -200
	b8400 := find(8400)
	if b8400.HabenSumme != 200 {
		t.Errorf("8400 HabenSumme: got %.2f, want 200", b8400.HabenSumme)
	}
	if b8400.Saldo != -200 {
		t.Errorf("8400 Saldo: got %.2f, want -200", b8400.Saldo)
	}

	// konto 1200: Soll 200, Haben 100, Saldo 100
	b1200 := find(1200)
	if b1200.SollSumme != 200 {
		t.Errorf("1200 SollSumme: got %.2f, want 200", b1200.SollSumme)
	}
	if b1200.HabenSumme != 100 {
		t.Errorf("1200 HabenSumme: got %.2f, want 100", b1200.HabenSumme)
	}
	if b1200.Saldo != 100 {
		t.Errorf("1200 Saldo: got %.2f, want 100", b1200.Saldo)
	}

	// sorted order: 1200, 4663, 8400
	if bals[0].Konto != 1200 || bals[1].Konto != 4663 || bals[2].Konto != 8400 {
		t.Errorf("expected sorted order [1200,4663,8400], got [%d,%d,%d]", bals[0].Konto, bals[1].Konto, bals[2].Konto)
	}
}

func TestComputeSuSaWithChart(t *testing.T) {
	chart := NewChartOfAccounts([]SKRAccount{
		{Number: 4663, Name: "Reisekosten", Type: "expense"},
		{Number: 8400, Name: "Erlöse 19%", Type: "revenue"},
		{Number: 1200, Name: "Bank", Type: "asset"},
	})
	rows := []CSVRow{
		{Buchung: Booking{Entries: []BookingEntry{
			{Konto: 4663, Betrag: 100, Soll: true},
			{Konto: 1200, Betrag: 100, Soll: false},
		}}},
	}
	bals := ComputeSuSa(rows, chart)
	for _, b := range bals {
		if b.Konto == 4663 && b.Name != "Reisekosten" {
			t.Errorf("expected Name 'Reisekosten' for 4663, got %q", b.Name)
		}
		if b.Konto == 1200 && b.Name != "Bank" {
			t.Errorf("expected Name 'Bank' for 1200, got %q", b.Name)
		}
	}
}

func TestComputeGuV(t *testing.T) {
	chart := NewChartOfAccounts([]SKRAccount{
		{Number: 4663, Name: "Reisekosten", Type: "expense"},
		{Number: 8400, Name: "Erlöse 19%", Type: "revenue"},
		{Number: 1200, Name: "Bank", Type: "asset"},
	})
	rows := []CSVRow{
		{Buchung: Booking{Entries: []BookingEntry{
			{Konto: 4663, Betrag: 100, Soll: true},
			{Konto: 1200, Betrag: 100, Soll: false},
		}}},
		{Buchung: Booking{Entries: []BookingEntry{
			{Konto: 1200, Betrag: 200, Soll: true},
			{Konto: 8400, Betrag: 200, Soll: false},
		}}},
	}

	bals := ComputeSuSa(rows, chart)
	g := ComputeGuV(bals, chart)

	if g.ErloeseGesamt != 200 {
		t.Errorf("ErloeseGesamt: got %.2f, want 200", g.ErloeseGesamt)
	}
	if g.AufwandGesamt != 100 {
		t.Errorf("AufwandGesamt: got %.2f, want 100", g.AufwandGesamt)
	}
	if g.Ergebnis != 100 {
		t.Errorf("Ergebnis: got %.2f, want 100", g.Ergebnis)
	}

	// 1200 (asset) should not appear in GuV
	for _, p := range g.ErloesPosten {
		if p.Konto == 1200 {
			t.Errorf("1200 (asset) should not be in ErloesPosten")
		}
	}
	for _, p := range g.AufwandPosten {
		if p.Konto == 1200 {
			t.Errorf("1200 (asset) should not be in AufwandPosten")
		}
	}
}

func TestBuildSuSaPDF(t *testing.T) {
	bals := []AccountBalance{
		{Konto: 4663, Name: "Reisekosten", SollSumme: 100, HabenSumme: 0, Saldo: 100},
		{Konto: 8400, Name: "Erlöse", SollSumme: 0, HabenSumme: 200, Saldo: -200},
	}
	data, err := BuildSuSaPDF(bals, "Summen- und Saldenliste")
	if err != nil {
		t.Fatalf("BuildSuSaPDF error: %v", err)
	}
	if len(data) < 4 || string(data[:4]) != "%PDF" {
		t.Errorf("expected PDF output starting with %%PDF")
	}
}

func TestBuildGuVPDF(t *testing.T) {
	g := GuV{
		ErloesPosten:  []AccountBalance{{Konto: 8400, Name: "Erlöse 19%", SollSumme: 0, HabenSumme: 200, Saldo: -200}},
		AufwandPosten: []AccountBalance{{Konto: 4663, Name: "Reisekosten", SollSumme: 100, HabenSumme: 0, Saldo: 100}},
		ErloeseGesamt: 200,
		AufwandGesamt: 100,
		Ergebnis:      100,
	}
	data, err := BuildGuVPDF(g, "Gewinn- und Verlustrechnung")
	if err != nil {
		t.Fatalf("BuildGuVPDF error: %v", err)
	}
	if len(data) < 4 || string(data[:4]) != "%PDF" {
		t.Errorf("expected PDF output starting with %%PDF")
	}
}

