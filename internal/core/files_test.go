package core

import "testing"

func TestIsSupportedFile(t *testing.T) {
	cases := map[string]bool{
		"rechnung.pdf": true,
		"Rechnung.PDF": true,
		"tabelle.xlsx": true,
		"alt.xls":      true,
		"brief.docx":   true,
		"brief.doc":    true,
		"folien.pptx":  true,
		"calc.ods":     true,
		"text.odt":     true,
		"foto.JPG":     true,
		"bild.png":     true,
		"scan.tiff":    true,
		"archiv.zip":   false,
		"daten.csv":    false,
		"noext":        false,
	}
	for name, want := range cases {
		if got := IsSupportedFile(name); got != want {
			t.Errorf("IsSupportedFile(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestIsPDF(t *testing.T) {
	if !IsPDF("a.pdf") {
		t.Error("a.pdf should be PDF")
	}
	if !IsPDF("A.PDF") {
		t.Error("A.PDF should be PDF")
	}
	if IsPDF("a.xlsx") {
		t.Error("a.xlsx should not be PDF")
	}
}

func TestReplaceExtension(t *testing.T) {
	if got := ReplaceExtension("2025-08_AWS_EUR.pdf", ".xlsx"); got != "2025-08_AWS_EUR.xlsx" {
		t.Errorf("got %q", got)
	}
	if got := ReplaceExtension("noext", ".pdf"); got != "noext.pdf" {
		t.Errorf("got %q", got)
	}
	if got := ReplaceExtension("file.pdf", ""); got != "file" {
		t.Errorf("got %q", got)
	}
}

func TestAttachmentName(t *testing.T) {
	if got := AttachmentName("2025-08-01_AWS_EUR.pdf", 1, ".xlsx"); got != "2025-08-01_AWS_EUR_Anhang1.xlsx" {
		t.Errorf("got %q", got)
	}
	if got := AttachmentName("inv.pdf", 2, ".pdf"); got != "inv_Anhang2.pdf" {
		t.Errorf("got %q", got)
	}
}

func TestParseAttachmentName(t *testing.T) {
	main := "2025-08-01_AWS_EUR.pdf"
	cases := []struct {
		name    string
		wantIdx int
		wantOK  bool
	}{
		{"2025-08-01_AWS_EUR_Anhang1.xlsx", 1, true},
		{"2025-08-01_AWS_EUR_Anhang12.pdf", 12, true},
		{"2025-08-01_AWS_EUR.pdf", 0, false},      // the main file itself
		{"2025-08-01_AWS_EUR_Anhang0.pdf", 0, false}, // 0 is not a valid index
		{"2025-08-01_AWS_EUR_AnhangX.pdf", 0, false}, // non-numeric
		{"other_Anhang1.pdf", 0, false},              // different invoice
	}
	for _, c := range cases {
		idx, ok := ParseAttachmentName(c.name, main)
		if ok != c.wantOK || idx != c.wantIdx {
			t.Errorf("ParseAttachmentName(%q) = (%d,%v), want (%d,%v)", c.name, idx, ok, c.wantIdx, c.wantOK)
		}
	}
}
