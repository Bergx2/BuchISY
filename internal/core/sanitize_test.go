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
