package core

import "sort"

// AccountSum is the total Soll amount booked to one SKR04 account in a period.
type AccountSum struct {
	Konto int
	Name  string
	Summe float64
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
