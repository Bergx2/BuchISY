package core

import (
	"sort"
	"strconv"
)

// UStVAZeile is the deductible input VAT (Vorsteuer) for one VAT rate.
type UStVAZeile struct {
	Satz      int     // VAT percent (19, 7, …)
	Konto     int     // the Vorsteuer account for that rate
	Vorsteuer float64 // summed input VAT booked to that account
}

// UStVA is the deductible-input-VAT summary for a period.
type UStVA struct {
	Zeilen          []UStVAZeile
	VorsteuerGesamt float64
}

// ComputeUStVA sums the booking Soll entries posted to the profile's Vorsteuer
// accounts (rules.VorsteuerKonten), grouped by VAT rate.
func ComputeUStVA(rows []CSVRow, rules *BookingRules) UStVA {
	// Reverse map: account → rate.
	rateByKonto := map[int]int{}
	for satz, konto := range rules.VorsteuerKonten {
		if s, err := strconv.Atoi(satz); err == nil {
			rateByKonto[konto] = s
		}
	}
	sumByRate := map[int]float64{}
	kontoByRate := map[int]int{}
	for _, r := range rows {
		for _, e := range r.Buchung.DebitEntries() {
			if satz, ok := rateByKonto[e.Konto]; ok {
				sumByRate[satz] += e.Betrag
				kontoByRate[satz] = e.Konto
			}
		}
	}
	var u UStVA
	for satz, summe := range sumByRate {
		summe = round2(summe)
		u.Zeilen = append(u.Zeilen, UStVAZeile{Satz: satz, Konto: kontoByRate[satz], Vorsteuer: summe})
		u.VorsteuerGesamt += summe
	}
	sort.Slice(u.Zeilen, func(i, j int) bool { return u.Zeilen[i].Satz < u.Zeilen[j].Satz })
	u.VorsteuerGesamt = round2(u.VorsteuerGesamt)
	return u
}
