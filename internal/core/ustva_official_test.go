package core

import "testing"

func TestComputeUStVAOfficial(t *testing.T) {
	rules, _ := ParseBookingRules([]byte(`{"regeln":[{"kategorie":"reverse_charge","rc_satz":19}]}`))
	rows := []CSVRow{
		// domestic sale 19% (Symeo): net 6500 → Kz81
		{Ausgangsrechnung: true, VATID: "DE123", TaxLines: []TaxLine{{Netto: 6500, SatzProzent: 19, MwStBetrag: 1235}}},
		// foreign non-taxable (Wullehus CH): 0%, no EU VAT-ID → Kz45
		{Ausgangsrechnung: true, VATID: "", TaxLines: []TaxLine{{Netto: 1000, SatzProzent: 0, MwStBetrag: 0}}},
		// intra-EU service (EU customer): 0%, EU VAT-ID → Kz21
		{Ausgangsrechnung: true, VATID: "FI26378052", TaxLines: []TaxLine{{Netto: 2000, SatzProzent: 0, MwStBetrag: 0}}},
		// §13b incoming (Google IE): 0%, EU supplier VAT-ID → Kz84/85/67
		{Ausgangsrechnung: false, VATID: "IE123", TaxLines: []TaxLine{{Netto: 462.40, SatzProzent: 0, MwStBetrag: 0}}},
		// normal incoming with VAT: Kz66 += 31.19
		{Ausgangsrechnung: false, VATID: "DE999", TaxLines: []TaxLine{{Netto: 164.16, SatzProzent: 19, MwStBetrag: 31.19}}},
	}
	u := ComputeUStVAOfficial(rows, rules)
	check := func(name string, got, want float64) {
		if !almost(got, want) {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
	}
	check("Kz81", u.Kz81, 6500)
	check("Kz45", u.Kz45, 1000)
	check("Kz21", u.Kz21, 2000)
	check("Kz84", u.Kz84, 462.40)
	check("Kz85", u.Kz85, 87.86) // 462.40 * 19%
	check("Kz67", u.Kz67, 87.86)
	check("Kz66", u.Kz66, 31.19)
	check("USt81", u.USt81, 1235) // 6500 * 19%
	// Kz83 = (1235 + 0 + 87.86) - (31.19 + 87.86) = 1203.81
	check("Kz83", u.Kz83, 1203.81)
}
