package core

import (
	"os"
	"testing"
	"time"
)

// TestExtractTextGuarded_SamplePDFs is a regression guard: the guarded extractor
// must return usable text for ordinary PDFs, quickly.
func TestExtractTextGuarded_SamplePDFs(t *testing.T) {
	e := NewPDFTextExtractor()
	for _, name := range []string{
		"../../sample-pdfs/ZUGFeRD-Example.pdf",
		"../../sample-pdfs/XRECHNUNG_Einfach.pdf",
	} {
		if _, err := os.Stat(name); err != nil {
			t.Skipf("sample PDF missing: %s", name)
		}
		txt, err := e.ExtractTextGuarded(name, 12*time.Second)
		if err != nil {
			t.Errorf("%s: unexpected error: %v", name, err)
			continue
		}
		if !HasText(txt) {
			t.Errorf("%s: expected usable text, got %d chars", name, len(txt))
		}
	}
}

// TestExtractTextGuarded_NoHang is the direct regression for the freeze bug:
// certain PDFs sent the former primary parser (ledongthuc/pdf) into an
// unbounded-allocation loop that froze the machine. The guarded extractor must
// now return quickly with usable text for such a PDF. Env-gated because the
// triggering file is a private invoice, not committed to the repo.
//
//	BUCHISY_HANG_PDF=/path/to/pathological.pdf go test ./internal/core -run NoHang -v
func TestExtractTextGuarded_NoHang(t *testing.T) {
	path := os.Getenv("BUCHISY_HANG_PDF")
	if path == "" {
		t.Skip("set BUCHISY_HANG_PDF to the freeze-triggering PDF")
	}
	e := NewPDFTextExtractor()

	done := make(chan struct{})
	var txt string
	var err error
	start := time.Now()
	go func() {
		txt, err = e.ExtractTextGuarded(path, 12*time.Second)
		close(done)
	}()
	select {
	case <-done:
		t.Logf("returned in %v (len=%d, err=%v)", time.Since(start), len(txt), err)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !HasText(txt) {
			t.Fatalf("expected usable text, got %d chars", len(txt))
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("ExtractTextGuarded did not return within 3s — still using the hanging parser")
	}
}
