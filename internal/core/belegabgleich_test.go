package core

import (
	"testing"
)

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
	// Foreign currency: EUR debit = round2(89.18/1.1583) = 76.99 (RowEUR converts Bruttobetrag; Gebuehr is part of Bruttobetrag).
	fx := CSVRow{Auftraggeber: "AWS", Bezahldatum: "14.01.2026", Bruttobetrag: 89.18, Waehrung: "USD", Wechselkurs: 1.1583, Gebuehr: 1.54}
	wantFX := round2(89.18 / 1.1583)
	if !almost(InvoiceEURAmount(fx), wantFX) {
		t.Errorf("InvoiceEURAmount(fx) = %v, want %v", InvoiceEURAmount(fx), wantFX)
	}
}

// TestInvoiceEURAmountRabatt verifies that InvoiceEURAmount subtracts Rabatt from
// both the EUR and the plain-Bruttobetrag branches (Method B reconciliation).
func TestInvoiceEURAmountRabatt(t *testing.T) {
	// Plain EUR branch: 1329.05 − 50 = 1279.05.
	row := CSVRow{Bruttobetrag: 1329.05, Waehrung: "EUR", Rabatt: 50}
	if got := InvoiceEURAmount(row); !almost(got, 1279.05) {
		t.Errorf("EUR branch: InvoiceEURAmount = %v, want 1279.05", got)
	}
	// Zero Rabatt: unchanged.
	row0 := CSVRow{Bruttobetrag: 1329.05, Waehrung: "EUR", Rabatt: 0}
	if got := InvoiceEURAmount(row0); !almost(got, 1329.05) {
		t.Errorf("zero Rabatt: InvoiceEURAmount = %v, want 1329.05", got)
	}
}

func TestFindGroupedPayments(t *testing.T) {
	cfg := DefaultMatchConfig()
	invoices := []CSVRow{
		{Dateiname: "a.pdf", Auftraggeber: "X", Bezahldatum: "10.01.2026", Bruttobetrag: 30, Waehrung: "EUR"},
		{Dateiname: "b.pdf", Auftraggeber: "Y", Bezahldatum: "10.01.2026", Bruttobetrag: 70, Waehrung: "EUR"},
		{Dateiname: "c.pdf", Auftraggeber: "Z", Bezahldatum: "10.01.2026", Bruttobetrag: 999, Waehrung: "EUR"},
	}
	lines := []StatementBooking{{Page: 0, LineIdx: 1, Date: "10.01.2026", Text: "Sammelüberweisung 100,00", Betrag: 100}}
	groups := FindGroupedPayments(invoices, lines, cfg)
	if len(groups) != 1 || len(groups[0].Dateinamen) != 2 {
		t.Fatalf("expected one 2-invoice group summing to 100, got %+v", groups)
	}
	// Verify the correct pair was found (a+b=100, not involving c=999).
	names := map[string]bool{groups[0].Dateinamen[0]: true, groups[0].Dateinamen[1]: true}
	if !names["a.pdf"] || !names["b.pdf"] {
		t.Errorf("expected a.pdf+b.pdf in group, got %v", groups[0].Dateinamen)
	}
	// Credit lines must be skipped.
	creditLines := []StatementBooking{{Page: 0, LineIdx: 2, Date: "10.01.2026", Betrag: 100, IstGutschrift: true}}
	if g := FindGroupedPayments(invoices, creditLines, cfg); len(g) != 0 {
		t.Errorf("credit line must be skipped, got %+v", g)
	}
	// No group when no pair sums to line amount.
	noMatch := []StatementBooking{{Page: 0, LineIdx: 3, Date: "10.01.2026", Betrag: 55}}
	if g := FindGroupedPayments(invoices, noMatch, cfg); len(g) != 0 {
		t.Errorf("no group expected, got %+v", g)
	}
}

func TestPartialPaymentLines(t *testing.T) {
	lines := []StatementBooking{
		{Page: 0, LineIdx: 1, Date: "10.01.2026", Text: "Teilzahlung 1", Betrag: 50},
		{Page: 0, LineIdx: 2, Date: "10.01.2026", Text: "Gutschrift", Betrag: 50, IstGutschrift: true},
		{Page: 0, LineIdx: 3, Date: "10.01.2026", Text: "Vollzahlung", Betrag: 100},
	}
	row := CSVRow{Dateiname: "r.pdf", Bezahldatum: "10.01.2026", Bruttobetrag: 100, Waehrung: "EUR", Teilzahlung: true}
	cands := PartialPaymentLines(row, lines)
	if len(cands) != 1 || cands[0].Line.LineIdx != 1 {
		t.Fatalf("expected one partial candidate (lineIdx=1), got %+v", cands)
	}
	// Non-Teilzahlung row: must return nil.
	rowFull := CSVRow{Dateiname: "r2.pdf", Bezahldatum: "10.01.2026", Bruttobetrag: 100, Waehrung: "EUR", Teilzahlung: false}
	if cands2 := PartialPaymentLines(rowFull, lines); len(cands2) != 0 {
		t.Errorf("non-Teilzahlung: expected nil, got %+v", cands2)
	}
}

func TestMatchConfigForeignToleranceAndCredit(t *testing.T) {
	cfg := DefaultMatchConfig()
	lines := []StatementBooking{
		{Page: 0, LineIdx: 1, Date: "14.01.2026", Text: "VISA AWS 78,90", Betrag: 78.90},
		{Page: 0, LineIdx: 2, Date: "14.01.2026", Text: "Gutschrift 78,53 H", Betrag: 78.53, IstGutschrift: true},
	}
	// Foreign invoice EUR amount ≈ round2(91.39/1.1583) ≈ 78.90; bank debited 78.90 (rate drift within 1.5%) → matches line 1, NOT the credit line.
	fx := CSVRow{Auftraggeber: "AWS", Bezahldatum: "14.01.2026", Bruttobetrag: 91.39, Waehrung: "USD", Wechselkurs: 1.1583}
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

func TestMatchRevenueToStatement(t *testing.T) {
	cfg := DefaultMatchConfig()
	row := CSVRow{Auftraggeber: "Acme Ltd", Ausgangsrechnung: true, Bruttobetrag: 1190, Rechnungsdatum: "10.01.2026"}
	lines := []StatementBooking{
		{Page: 0, LineIdx: 1, Date: "12.01.2026", Text: "Acme Ltd Zahlung", Betrag: 1190, IstGutschrift: true},  // incoming credit → match
		{Page: 0, LineIdx: 2, Date: "12.01.2026", Text: "Acme Ltd", Betrag: 1190, IstGutschrift: false},          // debit → must NOT match
	}
	kind, cands := MatchRevenueToStatement(row, lines, cfg)
	if kind == MatchNone || len(cands) != 1 {
		t.Fatalf("want one credit-line match, got kind=%v cands=%+v", kind, cands)
	}
	if !cands[0].Line.IstGutschrift || cands[0].Line.LineIdx != 1 {
		t.Errorf("matched the wrong line: %+v", cands[0].Line)
	}
	// And the expense matcher must still ignore the credit line:
	exKind, exCands := MatchInvoiceToStatement(row, lines, cfg)
	for _, c := range exCands {
		if c.Line.IstGutschrift {
			t.Errorf("expense matcher must never return a credit line: %+v", c)
		}
	}
	_ = exKind
}

func TestFindGroupedPaymentsTriple(t *testing.T) {
	cfg := DefaultMatchConfig()
	invoices := []CSVRow{
		{Dateiname: "a.pdf", Bezahldatum: "10.01.2026", Bruttobetrag: 20, Waehrung: "EUR"},
		{Dateiname: "b.pdf", Bezahldatum: "10.01.2026", Bruttobetrag: 30, Waehrung: "EUR"},
		{Dateiname: "c.pdf", Bezahldatum: "10.01.2026", Bruttobetrag: 50, Waehrung: "EUR"},
	}
	lines := []StatementBooking{{Page: 0, LineIdx: 1, Date: "10.01.2026", Text: "Sammel 100,00", Betrag: 100}}
	groups := FindGroupedPayments(invoices, lines, cfg)
	if len(groups) != 1 || len(groups[0].Dateinamen) != 3 {
		t.Fatalf("expected one 3-invoice group summing to 100, got %+v", groups)
	}
}

func TestFindGroupedRevenuePayments(t *testing.T) {
	cfg := DefaultMatchConfig()
	// One credit (Gutschrift) of 300 = two outgoing invoices 100 + 200.
	invs := []CSVRow{
		{Dateiname: "a.pdf", Ausgangsrechnung: true, Bruttobetrag: 100, Rechnungsdatum: "10.01.2026"},
		{Dateiname: "b.pdf", Ausgangsrechnung: true, Bruttobetrag: 200, Rechnungsdatum: "11.01.2026"},
	}
	credit := []StatementBooking{{Page: 0, LineIdx: 1, Date: "12.01.2026", Betrag: 300, IstGutschrift: true}}
	g := FindGroupedRevenuePayments(invs, credit, cfg)
	if len(g) != 1 || len(g[0].Dateinamen) != 2 {
		t.Fatalf("want one 2-invoice group on the credit, got %+v", g)
	}
	// A DEBIT line of 300 must NOT produce a revenue group.
	debit := []StatementBooking{{Page: 0, LineIdx: 2, Date: "12.01.2026", Betrag: 300, IstGutschrift: false}}
	if g := FindGroupedRevenuePayments(invs, debit, cfg); len(g) != 0 {
		t.Errorf("debit line must not group revenue: %+v", g)
	}
}

func TestRevenuePartialPaymentLines(t *testing.T) {
	row := CSVRow{Ausgangsrechnung: true, Teilzahlung: true, Bruttobetrag: 1000, Rechnungsdatum: "10.01.2026"}
	lines := []StatementBooking{
		{Page: 0, LineIdx: 1, Date: "12.01.2026", Betrag: 400, IstGutschrift: true},  // partial credit → candidate
		{Page: 0, LineIdx: 2, Date: "12.01.2026", Betrag: 400, IstGutschrift: false}, // debit → excluded
	}
	c := RevenuePartialPaymentLines(row, lines)
	if len(c) != 1 || !c[0].Line.IstGutschrift {
		t.Fatalf("want one credit partial line, got %+v", c)
	}
}
