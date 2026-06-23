package core

import (
	"sort"
	"strconv"
)

// UStVAZeile is one VAT-rate/account line on either side of the UStVA.
type UStVAZeile struct {
	Satz   int     // VAT percent (19, 7, …)
	Konto  int     // the Vorsteuer / Umsatzsteuer account
	Betrag float64 // summed VAT booked to that account
}

// UStVA summarizes a period: the output VAT owed (Umsatzsteuer, from outgoing
// invoices and §13b), the deductible input VAT (Vorsteuer, incl. §13b), and the
// resulting Zahllast (Umsatzsteuer − Vorsteuer; negative = Überschuss/refund).
type UStVA struct {
	Umsatzsteuer       []UStVAZeile
	Vorsteuer          []UStVAZeile
	UmsatzsteuerGesamt float64
	VorsteuerGesamt    float64
	Zahllast           float64
}

// ComputeUStVA sums booking entries posted to the profile's Vorsteuer accounts
// (Soll) and Umsatzsteuer accounts (Haben), grouped per account. §13b
// reverse-charge contributes its input-VAT account (KontoVStRC) to the Vorsteuer
// side and its output-VAT account (KontoUStRC) to the Umsatzsteuer side.
func ComputeUStVA(rows []CSVRow, rules *BookingRules) UStVA {
	// account → VAT rate, per side.
	vstRate := map[int]int{}
	for satz, konto := range rules.VorsteuerKonten {
		if s, err := strconv.Atoi(satz); err == nil {
			vstRate[konto] = s
		}
	}
	ustRate := map[int]int{}
	for satz, konto := range rules.UmsatzsteuerKonten {
		if s, err := strconv.Atoi(satz); err == nil {
			ustRate[konto] = s
		}
	}
	if rc, ok := rules.Rule("reverse_charge"); ok {
		rcSatz := int(rc.RcSatz + 0.5)
		if rc.KontoVStRC != 0 {
			vstRate[rc.KontoVStRC] = rcSatz
		}
		if rc.KontoUStRC != 0 {
			ustRate[rc.KontoUStRC] = rcSatz
		}
	}

	// Sum per account: Soll on a Vorsteuer account, Haben on an Umsatzsteuer one.
	vstSum := map[int]float64{}
	ustSum := map[int]float64{}
	for _, r := range rows {
		for _, e := range r.Buchung.Entries {
			if e.Soll {
				if _, ok := vstRate[e.Konto]; ok {
					vstSum[e.Konto] += e.Betrag
				}
			} else {
				if _, ok := ustRate[e.Konto]; ok {
					ustSum[e.Konto] += e.Betrag
				}
			}
		}
	}

	var u UStVA
	for konto, s := range ustSum {
		s = round2(s)
		u.Umsatzsteuer = append(u.Umsatzsteuer, UStVAZeile{Satz: ustRate[konto], Konto: konto, Betrag: s})
		u.UmsatzsteuerGesamt += s
	}
	for konto, s := range vstSum {
		s = round2(s)
		u.Vorsteuer = append(u.Vorsteuer, UStVAZeile{Satz: vstRate[konto], Konto: konto, Betrag: s})
		u.VorsteuerGesamt += s
	}
	sortZeilen(u.Umsatzsteuer)
	sortZeilen(u.Vorsteuer)
	u.UmsatzsteuerGesamt = round2(u.UmsatzsteuerGesamt)
	u.VorsteuerGesamt = round2(u.VorsteuerGesamt)
	u.Zahllast = round2(u.UmsatzsteuerGesamt - u.VorsteuerGesamt)
	return u
}

// sortZeilen orders lines ascending by rate, then by account.
func sortZeilen(z []UStVAZeile) {
	sort.Slice(z, func(i, j int) bool {
		if z[i].Satz != z[j].Satz {
			return z[i].Satz < z[j].Satz
		}
		return z[i].Konto < z[j].Konto
	})
}
