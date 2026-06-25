package core

import (
	"fmt"
	"sort"
)

// AccountBalance is one row in the trial balance (Summen- und Saldenliste).
// Saldo = SollSumme − HabenSumme (positive = debit excess, negative = credit excess).
type AccountBalance struct {
	Konto      int
	Name       string
	SollSumme  float64
	HabenSumme float64
	Saldo      float64
}

// ComputeSuSa computes the trial balance from all booking entries in rows.
// It accumulates Soll and Haben per account number, then returns a sorted
// slice (ascending by Konto). Name is resolved via chart.Find; if chart is
// nil or the account is not found, Name falls back to the number string.
func ComputeSuSa(rows []CSVRow, chart *ChartOfAccounts) []AccountBalance {
	rows = RowsEUR(rows)
	type sums struct{ soll, haben float64 }
	acc := make(map[int]*sums)

	for _, r := range rows {
		for _, e := range r.Buchung.Entries {
			s, ok := acc[e.Konto]
			if !ok {
				s = &sums{}
				acc[e.Konto] = s
			}
			if e.Soll {
				s.soll += e.Betrag
			} else {
				s.haben += e.Betrag
			}
		}
	}

	out := make([]AccountBalance, 0, len(acc))
	for konto, s := range acc {
		name := fmt.Sprintf("%d", konto)
		if chart != nil {
			if a, ok := chart.Find(konto); ok {
				name = a.Name
			}
		}
		out = append(out, AccountBalance{
			Konto:      konto,
			Name:       name,
			SollSumme:  round2(s.soll),
			HabenSumme: round2(s.haben),
			Saldo:      round2(s.soll - s.haben),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Konto < out[j].Konto })
	return out
}

// GuV is a simplified income statement (Gewinn- und Verlustrechnung).
type GuV struct {
	ErloesPosten  []AccountBalance // revenue accounts (Type == "revenue")
	AufwandPosten []AccountBalance // expense accounts (Type == "expense")
	ErloeseGesamt float64          // sum of revenue: HabenSumme − SollSumme per account
	AufwandGesamt float64          // sum of expenses: SollSumme − HabenSumme per account
	Ergebnis      float64          // ErloeseGesamt − AufwandGesamt
}

// ComputeGuV partitions the trial balance into revenue and expense accounts
// using the chart Type field, then computes totals and Ergebnis.
// Revenue contribution = HabenSumme − SollSumme (credit-side accounts).
// Expense contribution = SollSumme − HabenSumme (debit-side accounts).
// Accounts without a chart entry (or with a non-revenue/expense type) are
// ignored. chart may be nil, in which case no accounts are classified and
// the GuV is empty.
func ComputeGuV(susa []AccountBalance, chart *ChartOfAccounts) GuV {
	var g GuV
	for _, b := range susa {
		if chart == nil {
			continue
		}
		a, ok := chart.Find(b.Konto)
		if !ok {
			continue
		}
		switch a.Type {
		case "revenue":
			contrib := round2(b.HabenSumme - b.SollSumme)
			g.ErloesPosten = append(g.ErloesPosten, b)
			g.ErloeseGesamt += contrib
		case "expense":
			contrib := round2(b.SollSumme - b.HabenSumme)
			g.AufwandPosten = append(g.AufwandPosten, b)
			g.AufwandGesamt += contrib
		}
	}
	g.ErloeseGesamt = round2(g.ErloeseGesamt)
	g.AufwandGesamt = round2(g.AufwandGesamt)
	g.Ergebnis = round2(g.ErloeseGesamt - g.AufwandGesamt)
	return g
}
