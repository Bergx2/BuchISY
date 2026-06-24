package anthropic

import (
	"math"
	"strings"
	"testing"

	"github.com/bergx2/buchisy/internal/core"
)

func almostA(a, b float64) bool {
	return math.Abs(a-b) < 0.005
}

func TestNormalizeVATID(t *testing.T) {
	cases := map[string]string{
		"DE287472874":    "DE287472874",
		"de287472874":    "DE287472874",
		" DE287472874 ":  "DE287472874",
		"DE 287 472 874": "DE287472874",
		"DE-287472874":   "DE287472874",
		"de.287.472.874": "DE287472874",
	}
	for in, want := range cases {
		if got := normalizeVATID(in); got != want {
			t.Errorf("normalizeVATID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsOwnVATID(t *testing.T) {
	own := []string{"DE287472874", "DE319686097"}
	if !isOwnVATID("DE287472874", own) {
		t.Error("DE287472874 must be detected as own")
	}
	if !isOwnVATID("de 287 472 874", own) {
		t.Error("loose formatting must still match own")
	}
	if isOwnVATID("ATU12345678", own) {
		t.Error("foreign VAT-ID must NOT match own")
	}
	if isOwnVATID("", own) {
		t.Error("empty must not match own")
	}
}

func TestParseMultipleTaxLines(t *testing.T) {
	js := `{"auftraggeber":"R","steuerzeilen":[{"satz":19,"netto":14.20,"mwst":2.70},{"satz":7,"netto":18.69,"mwst":1.31}],"trinkgeld":2.00}`
	meta, err := parseExtractionJSON(js, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(meta.TaxLines) != 2 || meta.Trinkgeld != 2.00 {
		t.Fatalf("lines = %+v trinkgeld=%v", meta.TaxLines, meta.Trinkgeld)
	}
	if !almostA(meta.BetragNetto, 32.89) || !almostA(meta.Bruttobetrag, 38.90) {
		t.Errorf("aggregates wrong: netto=%v brutto=%v", meta.BetragNetto, meta.Bruttobetrag)
	}
}

func TestParseExtractionResponseFiltersOwnVAT(t *testing.T) {
	resp := `{"firmenname":"Bergx2 GmbH","rechnungsnummer":"R-1","vat_id":"DE287472874","bruttobetrag":119.0}`
	meta, err := parseExtractionResponse(resp, []string{"DE287472874"})
	if err != nil {
		t.Fatal(err)
	}
	if meta.VATID != "" {
		t.Errorf("expected own VAT-ID to be dropped, got %q", meta.VATID)
	}
}

func TestParseExtractionResponseKeepsForeignVAT(t *testing.T) {
	resp := `{"firmenname":"Google Ireland","vat_id":"IE6388047V","bruttobetrag":50.0}`
	meta, err := parseExtractionResponse(resp, []string{"DE287472874"})
	if err != nil {
		t.Fatal(err)
	}
	if meta.VATID != "IE6388047V" {
		t.Errorf("expected foreign VAT-ID to be kept, got %q", meta.VATID)
	}
}

func TestParseExtractionAccountSuggestions(t *testing.T) {
	resp := `{"auftraggeber":"AWS","verwendungszweck":"Hosting","bruttobetrag":119,"gegenkonto_vorschlaege":[6837,6800,27]}`
	meta, err := parseExtractionJSON(resp, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(meta.KontoVorschlaege) != 3 || meta.KontoVorschlaege[0] != 6837 {
		t.Fatalf("KontoVorschlaege = %v", meta.KontoVorschlaege)
	}
	// absent field → empty (no crash)
	m2, _ := parseExtractionJSON(`{"auftraggeber":"X"}`, nil)
	if len(m2.KontoVorschlaege) != 0 {
		t.Errorf("expected no suggestions, got %v", m2.KontoVorschlaege)
	}
}

func TestParseExtractionAusgangsrechnung(t *testing.T) {
	// ausgangsrechnung: true must propagate to meta.Ausgangsrechnung.
	resp := `{"auftraggeber":"Bergx2 GmbH","rechnungsnummer":"AR-42","bruttobetrag":1190.0,"ausgangsrechnung":true}`
	meta, err := parseExtractionJSON(resp, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !meta.Ausgangsrechnung {
		t.Error("expected meta.Ausgangsrechnung == true, got false")
	}

	// ausgangsrechnung: false (explicit) → false.
	resp2 := `{"auftraggeber":"AWS","bruttobetrag":50.0,"ausgangsrechnung":false}`
	meta2, err := parseExtractionJSON(resp2, nil)
	if err != nil {
		t.Fatal(err)
	}
	if meta2.Ausgangsrechnung {
		t.Error("expected meta.Ausgangsrechnung == false, got true")
	}

	// Field absent → default false (zero value).
	resp3 := `{"auftraggeber":"Google","bruttobetrag":20.0}`
	meta3, err := parseExtractionJSON(resp3, nil)
	if err != nil {
		t.Fatal(err)
	}
	if meta3.Ausgangsrechnung {
		t.Error("expected meta.Ausgangsrechnung == false when field absent")
	}
}

func TestSystemPromptForAusgangsrechnungBlock(t *testing.T) {
	// With own VAT-IDs: prompt must contain the Ausgangsrechnung instruction.
	p := systemPromptFor([]string{"DE287472874"})
	if !strings.Contains(p, "ausgangsrechnung") {
		t.Error("systemPromptFor with ownVATIDs must mention ausgangsrechnung")
	}
	if !strings.Contains(p, "DE287472874") {
		t.Error("systemPromptFor must include the own VAT-ID in ausgangsrechnung block")
	}

	// Without own VAT-IDs: base prompt returned; no extra instruction needed.
	pBase := systemPromptFor(nil)
	if pBase != systemPromptBase {
		t.Error("systemPromptFor with no ownVATIDs must return systemPromptBase unchanged")
	}
}

func TestParseExtractionCashPayment(t *testing.T) {
	// bezahldatum + bar_bezahlt=true must propagate to Meta.
	resp := `{"auftraggeber":"Bäckerei Müller","bruttobetrag":4.80,"bezahldatum":"20.06.2026","bar_bezahlt":true}`
	meta, err := parseExtractionJSON(resp, nil)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Bezahldatum != "20.06.2026" {
		t.Errorf("expected Bezahldatum=20.06.2026, got %q", meta.Bezahldatum)
	}
	if !meta.BarBezahlt {
		t.Error("expected BarBezahlt==true, got false")
	}

	// bar_bezahlt=false and no bezahldatum → zero values.
	resp2 := `{"auftraggeber":"AWS","bruttobetrag":50.0,"bar_bezahlt":false}`
	meta2, err := parseExtractionJSON(resp2, nil)
	if err != nil {
		t.Fatal(err)
	}
	if meta2.Bezahldatum != "" {
		t.Errorf("expected empty Bezahldatum, got %q", meta2.Bezahldatum)
	}
	if meta2.BarBezahlt {
		t.Error("expected BarBezahlt==false, got true")
	}

	// Both fields absent → zero values (no crash).
	resp3 := `{"auftraggeber":"Google","bruttobetrag":20.0}`
	meta3, err := parseExtractionJSON(resp3, nil)
	if err != nil {
		t.Fatal(err)
	}
	if meta3.Bezahldatum != "" || meta3.BarBezahlt {
		t.Errorf("absent fields must be zero: Bezahldatum=%q BarBezahlt=%v", meta3.Bezahldatum, meta3.BarBezahlt)
	}
}

func TestAccountHintSectionListsAccounts(t *testing.T) {
	e := NewExtractor(nil, false)
	e.SetAccountHints([]core.SKRAccount{{Number: 6837, Name: "Fremdleistungen"}, {Number: 6815, Name: "Bürobedarf"}})
	s := e.accountHintSection()
	if !strings.Contains(s, "6837") || !strings.Contains(s, "Fremdleistungen") || !strings.Contains(s, "gegenkonto_vorschlaege") {
		t.Errorf("account hint section missing content:\n%s", s)
	}
	// no hints → empty section (no token cost)
	if NewExtractor(nil, false).accountHintSection() != "" {
		t.Error("no hints should yield empty section")
	}
}
