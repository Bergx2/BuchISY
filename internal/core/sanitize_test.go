package core

import "testing"

func TestSanitizeFilenameKeepsComma(t *testing.T) {
	got := SanitizeFilename("2026-05-21_Foo_EUR_15,23.pdf")
	want := "2026-05-21_Foo_EUR_15,23.pdf"
	if got != want {
		t.Errorf("SanitizeFilename = %q, want %q (comma must be kept)", got, want)
	}
}

func TestSanitizeFilenameRemovesUnsafe(t *testing.T) {
	got := SanitizeFilename(`a<b>c:d"e|f?g*h.pdf`)
	want := "abcdefgh.pdf"
	if got != want {
		t.Errorf("SanitizeFilename = %q, want %q", got, want)
	}
}

func TestNormalizeVerwendungszweck(t *testing.T) {
	cases := map[string]string{
		"Einstellgebühr & Top-Anzeige kleinanzeigen.de 05/2026": "Einstellgebühr und Top-Anzeige kleinanzeigen.de 05/2026",
		"A & B":         "A und B",
		"A&B":           "A und B",
		"A&B&C":         "A und B und C",
		"A&  B":         "A und B",
		"kein Symbol":   "kein Symbol",
		"":              "",
		"& Anfang":      "und Anfang",
		"Ende &":        "Ende und",
	}
	for in, want := range cases {
		if got := NormalizeVerwendungszweck(in); got != want {
			t.Errorf("NormalizeVerwendungszweck(%q) = %q, want %q", in, got, want)
		}
	}
}
