package core

import "testing"

func TestMetaTaxLinesRoundTrip(t *testing.T) {
	m := Meta{
		Auftraggeber: "Restaurant",
		TaxLines:     []TaxLine{{Netto: 14.20, SatzProzent: 19, MwStBetrag: 2.70}},
		Trinkgeld:    2.00,
	}
	row := m.ToCSVRow()
	if len(row.TaxLines) != 1 || row.Trinkgeld != 2.00 {
		t.Fatalf("ToCSVRow lost detail: %+v", row)
	}
	back := row.ToMeta()
	if len(back.TaxLines) != 1 || back.Trinkgeld != 2.00 {
		t.Fatalf("ToMeta lost detail: %+v", back)
	}
}
