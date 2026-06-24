package core

import "testing"

func TestBuildVerfahrensdokumentationPDF(t *testing.T) {
	s := Settings{
		StorageRoot:     "/home/user/BuchISY",
		NamingTemplate:  "${Company}_${YYYY-MM-DD}_${InvoiceNumber}",
		ProcessingMode:  "claude",
		CurrencyDefault: "EUR",
	}
	data, err := BuildVerfahrensdokumentationPDF(s, 420, "Bergx2 GmbH", "24.06.2026")
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 500 {
		t.Fatalf("PDF too short (%d bytes)", len(data))
	}
	if string(data[:4]) != "%PDF" {
		t.Fatalf("not a PDF (first 4 bytes: %q)", string(data[:4]))
	}

	// zero-value Settings must not error
	if _, err := BuildVerfahrensdokumentationPDF(Settings{}, 0, "", ""); err != nil {
		t.Errorf("zero Settings errored: %v", err)
	}
}
