package core

import "testing"

func TestIsEUVatID(t *testing.T) {
	cases := map[string]bool{
		"FI26378052":   true,
		"ATU12345678":  true,
		"FR12345678901": true,
		"DE287472874":  false, // domestic is not a ZM counterparty
		"CHE123456789": false, // Switzerland is not EU
		"":             false,
		"12345":        false,
	}
	for in, want := range cases {
		if IsEUVatID(in) != want {
			t.Errorf("IsEUVatID(%q) = %v, want %v", in, !want, want)
		}
	}
}

func TestComputeZM(t *testing.T) {
	rows := []CSVRow{
		// EU customer, no VAT, outgoing → ZM. net 6500.
		{Ausgangsrechnung: true, VATID: "FI26378052", TaxLines: []TaxLine{{Netto: 6500, SatzProzent: 0, MwStBetrag: 0}}},
		// same customer again, net 1000 → accumulates.
		{Ausgangsrechnung: true, VATID: "FI26378052", TaxLines: []TaxLine{{Netto: 1000, SatzProzent: 0, MwStBetrag: 0}}},
		// domestic outgoing (DE, has VAT) → excluded.
		{Ausgangsrechnung: true, VATID: "DE123", TaxLines: []TaxLine{{Netto: 100, SatzProzent: 19, MwStBetrag: 19}}},
		// Swiss outgoing, no EU VAT-ID → excluded.
		{Ausgangsrechnung: true, VATID: "", TaxLines: []TaxLine{{Netto: 500, SatzProzent: 0, MwStBetrag: 0}}},
		// incoming EU supplier (not Ausgangsrechnung) → excluded.
		{Ausgangsrechnung: false, VATID: "IE123", TaxLines: []TaxLine{{Netto: 200, SatzProzent: 0, MwStBetrag: 0}}},
	}
	z := ComputeZM(rows)
	if len(z.Zeilen) != 1 || z.Zeilen[0].UStIdNr != "FI26378052" || !almost(z.Zeilen[0].Netto, 7500) {
		t.Fatalf("ZM = %+v (want one line FI26378052 / 7500)", z.Zeilen)
	}
	if !almost(z.Kontrollsumme, 7500) {
		t.Errorf("Kontrollsumme = %v, want 7500", z.Kontrollsumme)
	}
}
