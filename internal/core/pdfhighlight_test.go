package core

import (
	"testing"

	"github.com/ledongthuc/pdf"
)

func TestPdfBoxToPixel(t *testing.T) {
	// dpi 144 -> scale 2.0. Page 842pt high. Box at (100,700) size 50x10.
	// Y flips: imageY = (842 - 700 - 10) * 2 = 264.
	got := pdfBoxToPixel(pdfBox{x: 100, y: 700, w: 50, h: 10}, 842, 144)
	want := Rect{X: 200, Y: 264, W: 100, H: 20}
	if got != want {
		t.Errorf("pdfBoxToPixel = %+v, want %+v", got, want)
	}
}

func TestValueBoxInRow(t *testing.T) {
	frags := []pdf.Text{
		{X: 10, Y: 100, W: 30, FontSize: 12, S: "Rechnung "},
		{X: 40, Y: 100, W: 25, FontSize: 12, S: "Nr. "},
		{X: 65, Y: 100, W: 40, FontSize: 12, S: "2025-0815"},
	}

	// Single fragment, case-insensitive.
	box, ok := valueBoxInRow(frags, "2025-0815")
	if !ok {
		t.Fatal("expected to find 2025-0815")
	}
	if box != (pdfBox{x: 65, y: 100, w: 40, h: 12}) {
		t.Errorf("single-fragment box = %+v", box)
	}

	// Spanning two fragments: "Nr. 2025" covers frag 2 and frag 3.
	box, ok = valueBoxInRow(frags, "Nr. 2025")
	if !ok {
		t.Fatal("expected to find spanning value")
	}
	if box != (pdfBox{x: 40, y: 100, w: 65, h: 12}) {
		t.Errorf("spanning box = %+v", box)
	}

	// Not present.
	if _, ok := valueBoxInRow(frags, "Telekom"); ok {
		t.Error("Telekom should not be found")
	}

	// Empty value never matches.
	if _, ok := valueBoxInRow(frags, "  "); ok {
		t.Error("empty value should not match")
	}
}

func TestPdfBoxToPixelTopOrigin(t *testing.T) {
	// Top-origin box at y=840 in a 1031-tall coord space, page 792pt, 110 DPI.
	// Expected Y ≈ 840 * (110/72) * (792/1031) ≈ 986 px.
	r := pdfBoxToPixelTopOrigin(pdfBox{x: 100, y: 840, w: 50, h: 12}, 792, 1031, 110)
	if r.Y < 980 || r.Y > 992 {
		t.Errorf("top-origin Y mapping off: got %.1f, want ~986", r.Y)
	}
	if r.H < 13 || r.H > 16 {
		t.Errorf("top-origin H mapping off: got %.1f, want ~14", r.H)
	}
}

func TestStmtDateRe(t *testing.T) {
	dated := []string{"11/05CLAUDE.AI", "11.05 Qonto", "2/05 TESTRAIL", "31/12"}
	for _, s := range dated {
		if !stmtDateRe.MatchString(s) {
			t.Errorf("expected %q to be detected as a dated booking row", s)
		}
	}
	undated := []string{"1.17198945209493 USD = 1.00 EUR", "Karte **6868", "- 200.00 USD", "Abonnement / Zusatzgebühren"}
	for _, s := range undated {
		if stmtDateRe.MatchString(s) {
			t.Errorf("expected %q NOT to be detected as a dated booking row", s)
		}
	}
}
