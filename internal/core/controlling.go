package core

import "sort"

// AccountSum is the total Soll amount booked to one SKR04 account in a period.
type AccountSum struct {
	Konto int
	Name  string
	Summe float64
}

// Controlling splits a period's bookings into revenue (Einnahmen, Haben on
// non-tax/non-payment accounts) and expense (Ausgaben, Soll on non-tax/non-
// payment accounts), with the resulting Saldo (Einnahmen − Ausgaben).
type Controlling struct {
	Einnahmen       []AccountSum
	Ausgaben        []AccountSum
	EinnahmenGesamt float64
	AusgabenGesamt  float64
	Saldo           float64
}

// AggregateBookingsByAccount sums the Soll (debit) entries of every booking in
// rows, grouped by account number, and returns the per-account sums sorted by
// account plus the grand total. Names are filled from chart (empty if chart is
// nil or the account is unknown). Bookings without entries contribute nothing.
func AggregateBookingsByAccount(rows []CSVRow, chart *ChartOfAccounts) ([]AccountSum, float64) {
	byKonto := map[int]float64{}
	for _, r := range rows {
		for _, e := range r.Buchung.DebitEntries() {
			byKonto[e.Konto] += e.Betrag
		}
	}
	sums := make([]AccountSum, 0, len(byKonto))
	var total float64
	for konto, summe := range byKonto {
		summe = round2(summe)
		name := ""
		if chart != nil {
			if acc, ok := chart.Find(konto); ok {
				name = acc.Name
			}
		}
		sums = append(sums, AccountSum{Konto: konto, Name: name, Summe: summe})
		total += summe
	}
	sort.Slice(sums, func(i, j int) bool { return sums[i].Konto < sums[j].Konto })
	return sums, round2(total)
}

// AggregateControlling sums P&L accounts only: it excludes the profile's VAT
// accounts (Vorsteuer + Umsatzsteuer + §13b) and the payment accounts
// (paymentKonten). On the remaining accounts, Soll entries are expenses and
// Haben entries are revenue. Names come from chart (nil → empty).
func AggregateControlling(rows []CSVRow, rules *BookingRules, paymentKonten map[int]bool, chart *ChartOfAccounts) Controlling {
	rows = RowsEUR(rows)
	exclude := map[int]bool{}
	for k := range paymentKonten {
		exclude[k] = true
	}
	for _, k := range rules.VorsteuerKonten {
		exclude[k] = true
	}
	for _, k := range rules.UmsatzsteuerKonten {
		exclude[k] = true
	}
	if rc, ok := rules.Rule("reverse_charge"); ok {
		if rc.KontoVStRC != 0 {
			exclude[rc.KontoVStRC] = true
		}
		if rc.KontoUStRC != 0 {
			exclude[rc.KontoUStRC] = true
		}
	}

	einn := map[int]float64{}
	ausg := map[int]float64{}
	for _, r := range rows {
		for _, e := range r.Buchung.Entries {
			if exclude[e.Konto] {
				continue
			}
			if e.Soll {
				ausg[e.Konto] += e.Betrag
			} else {
				einn[e.Konto] += e.Betrag
			}
		}
	}
	var c Controlling
	c.Einnahmen, c.EinnahmenGesamt = toSums(einn, chart)
	c.Ausgaben, c.AusgabenGesamt = toSums(ausg, chart)
	c.Saldo = round2(c.EinnahmenGesamt - c.AusgabenGesamt)
	return c
}

// toSums turns an account→amount map into sorted AccountSums + the rounded total.
func toSums(byKonto map[int]float64, chart *ChartOfAccounts) ([]AccountSum, float64) {
	sums := make([]AccountSum, 0, len(byKonto))
	var total float64
	for konto, summe := range byKonto {
		name := ""
		if chart != nil {
			if acc, ok := chart.Find(konto); ok {
				name = acc.Name
			}
		}
		sums = append(sums, AccountSum{Konto: konto, Name: name, Summe: round2(summe)})
		total += summe
	}
	sort.Slice(sums, func(i, j int) bool { return sums[i].Konto < sums[j].Konto })
	return sums, round2(total)
}
