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
