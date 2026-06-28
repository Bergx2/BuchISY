package core

import "testing"

func TestSuggestCategory(t *testing.T) {
	// SKR04 Bewirtung accounts: 6640 (abziehbar), 6644 (nicht abziehbar).
	rules := &BookingRules{
		Regeln: []BookingRule{
			{Kategorie: "standard", Name: "Standard-Aufwand"},
			{Kategorie: "bewirtung", Name: "Bewirtung", AbziehbarProzent: 70, KontoAbziehbar: 6640, KontoNichtAbziehbar: 6644},
			{Kategorie: "reisekosten", Name: "Reisekosten", DefaultKonto: 6650},
			{Kategorie: "kfz", Name: "Kfz-Kosten", DefaultKonto: 6520},
			{Kategorie: "geschenke", Name: "Geschenke", Schwelle: 35},
		},
	}

	cases := []struct {
		name       string
		gegenkonto int
		text       string
		want       string
		ok         bool
	}{
		{"account abziehbar", 6640, "Müller GmbH", "bewirtung", true},
		{"account nicht abziehbar", 6644, "irgendwas", "bewirtung", true},
		{"keyword bewirtung", 4980, "Bewirtungsbeleg Geschäftsessen", "bewirtung", true},
		{"keyword restaurant", 4980, "Restaurant Adler", "bewirtung", true},
		{"keyword gaststätte", 4980, "Gaststätte zum Hirsch", "bewirtung", true},
		{"keyword hotel → reisekosten", 4980, "Hotel Sonne Übernachtung", "reisekosten", true},
		{"keyword tankstelle → kfz", 4980, "Aral Tankstelle", "kfz", true},
		{"keyword geschenk", 4980, "Geschenk für Kunden", "geschenke", true},
		{"restaurant beats hotel (priority)", 4980, "Hotel-Restaurant Krone", "bewirtung", true},
		{"no signal", 4980, "Bürobedarf Staples", "", false},
		{"zero account no keyword", 0, "Beratungsleistung", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := rules.SuggestCategory(c.gegenkonto, c.text)
			if got != c.want || ok != c.ok {
				t.Errorf("SuggestCategory(%d, %q) = (%q,%v), want (%q,%v)", c.gegenkonto, c.text, got, ok, c.want, c.ok)
			}
		})
	}

	// A keyword for a category the profile does NOT have must not be suggested.
	onlyStd := &BookingRules{Regeln: []BookingRule{{Kategorie: "standard"}}}
	if _, ok := onlyStd.SuggestCategory(6640, "Restaurant"); ok {
		t.Error("category absent from rules must return ok=false")
	}

	// nil receiver must not panic.
	if _, ok := (*BookingRules)(nil).SuggestCategory(6640, "Bewirtung"); ok {
		t.Error("nil rules must return ok=false")
	}
}
