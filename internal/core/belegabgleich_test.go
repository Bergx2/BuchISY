package core

import "testing"

func TestMatchInvoiceToStatement(t *testing.T) {
	lines := []StatementBooking{
		{Page: 0, LineIdx: 1, Date: "12.01.2026", Text: "Lastschrift Telekom 49,99", Betrag: 49.99},
		{Page: 0, LineIdx: 2, Date: "14.01.2026", Text: "AMAZON WEB SERVICES 78,53", Betrag: 78.53},
		{Page: 0, LineIdx: 3, Date: "20.01.2026", Text: "REWE Markt 78,53", Betrag: 78.53},
	}
	// Unique exact amount + close date + name overlap → Auto.
	auto := CSVRow{Auftraggeber: "AWS", Bezahldatum: "14.01.2026", Bruttobetrag: 78.53, Waehrung: "EUR"}
	// remove the third 78,53 line for the unique case
	cfg := DefaultMatchConfig()
	kind, cands := MatchInvoiceToStatement(auto, lines[:2], cfg)
	if kind != MatchAuto || len(cands) == 0 || cands[0].Line.LineIdx != 2 {
		t.Fatalf("auto: kind=%v cands=%+v", kind, cands)
	}
	// Two lines share 78,53 → ambiguous → Suggest, best (closest date / name) first.
	kind2, cands2 := MatchInvoiceToStatement(auto, lines, cfg)
	if kind2 != MatchSuggest || len(cands2) != 2 || cands2[0].Line.LineIdx != 2 {
		t.Errorf("suggest: kind=%v cands=%+v", kind2, cands2)
	}
	// No amount match → None.
	none := CSVRow{Auftraggeber: "X", Bezahldatum: "14.01.2026", Bruttobetrag: 999, Waehrung: "EUR"}
	if k, _ := MatchInvoiceToStatement(none, lines, cfg); k != MatchNone {
		t.Errorf("none: kind=%v", k)
	}
	// Foreign currency: EUR debit = round2(89.18/1.1583)+1.54 = 78.53 → matches line 2.
	fx := CSVRow{Auftraggeber: "AWS", Bezahldatum: "14.01.2026", Bruttobetrag: 89.18, Waehrung: "USD", Wechselkurs: 1.1583, Gebuehr: 1.54}
	if !almost(InvoiceEURAmount(fx), 78.53) {
		t.Errorf("InvoiceEURAmount(fx) = %v, want 78.53", InvoiceEURAmount(fx))
	}
}

func TestMatchConfigForeignToleranceAndCredit(t *testing.T) {
	cfg := DefaultMatchConfig()
	lines := []StatementBooking{
		{Page: 0, LineIdx: 1, Date: "14.01.2026", Text: "VISA AWS 78,90", Betrag: 78.90},
		{Page: 0, LineIdx: 2, Date: "14.01.2026", Text: "Gutschrift 78,53 H", Betrag: 78.53, IstGutschrift: true},
	}
	// Foreign invoice EUR amount 78.53; bank debited 78.90 (rate drift). Within 1.5% → matches line 1, NOT the credit line.
	fx := CSVRow{Auftraggeber: "AWS", Bezahldatum: "14.01.2026", Bruttobetrag: 89.18, Waehrung: "USD", Wechselkurs: 1.1583, Gebuehr: 1.54}
	kind, cands := MatchInvoiceToStatement(fx, lines, cfg)
	if kind == MatchNone || len(cands) == 0 || cands[0].Line.LineIdx != 1 {
		t.Fatalf("foreign tolerance: kind=%v cands=%+v", kind, cands)
	}
	for _, c := range cands {
		if c.Line.IstGutschrift {
			t.Errorf("credit line must be excluded: %+v", c)
		}
	}
	// EUR invoice keeps the strict 0.01 filter: 78.90 line must NOT match an 78.53 EUR invoice.
	eur := CSVRow{Auftraggeber: "X", Bezahldatum: "14.01.2026", Bruttobetrag: 78.53, Waehrung: "EUR"}
	if k, _ := MatchInvoiceToStatement(eur, lines, cfg); k != MatchNone {
		t.Errorf("EUR strict tolerance broken: %v", k)
	}
	// Alias boost: a learned token lets a no-shared-word supplier rank.
	cfg.Aliases = map[string][]string{"aws": {"amazon"}}
	al := []StatementBooking{{Page: 0, LineIdx: 1, Date: "14.01.2026", Text: "AMAZON WEB SERV 78,53", Betrag: 78.53}}
	if k, c := MatchInvoiceToStatement(CSVRow{Auftraggeber: "AWS", Bezahldatum: "14.01.2026", Bruttobetrag: 78.53, Waehrung: "EUR"}, al, cfg); k == MatchNone || len(c) == 0 {
		t.Errorf("alias match failed: %v", k)
	}
}
