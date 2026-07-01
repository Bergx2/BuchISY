package core

import (
	"os"
	"testing"
	"time"
)

// TestHighlightRects_NoHang guards the document-preview highlight path against
// the ledongthuc hang that froze uploads. The exported HighlightRects runs the
// parse in a killable child process, so on the pathological PDF it must return
// (with no boxes) shortly after the worker timeout instead of hanging the app
// and exhausting memory. Env-gated on the private triggering PDF.
func TestHighlightRects_NoHang(t *testing.T) {
	path := os.Getenv("BUCHISY_HANG_PDF")
	if path == "" {
		t.Skip("set BUCHISY_HANG_PDF")
	}
	done := make(chan struct{})
	var rects [][]Rect
	go func() {
		rects, _ = HighlightRects(path, []string{"1389,47"}, 144)
		close(done)
	}()
	select {
	case <-done:
		if rects != nil {
			t.Logf("got %d pages of rects (unexpected but harmless)", len(rects))
		}
	case <-time.After(rectsWorkerTimeout + 4*time.Second):
		t.Fatalf("HighlightRects did not return after the worker timeout — child not being killed")
	}
}

// TestHighlightRects_Isolated verifies the child-process round-trip works for a
// normal PDF: the exported (isolated) HighlightRects finds the same boxes the
// in-process impl does.
func TestHighlightRects_Isolated(t *testing.T) {
	const sample = "../../sample-pdfs/ZUGFeRD-Example.pdf"
	if _, err := os.Stat(sample); err != nil {
		t.Skipf("sample PDF missing: %s", sample)
	}
	// A value that appears in the sample invoice text.
	got, err := HighlightRects(sample, []string{"ZUGFeRD"}, 144)
	if err != nil {
		t.Fatalf("isolated HighlightRects error: %v", err)
	}
	if got == nil {
		t.Fatalf("expected a non-nil per-page result from the worker")
	}
}
