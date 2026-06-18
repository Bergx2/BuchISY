package anthropic

import "testing"

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
