package db

import (
	"path/filepath"
	"testing"

	"github.com/bergx2/buchisy/internal/core"
)

func newTestRepo(t *testing.T) *Repository {
	t.Helper()
	repo, err := NewRepository(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("NewRepository: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	return repo
}

// TestUpdateMovesInvoiceAcrossMonths verifies that Update can move an invoice
// to a different filing month in a single statement (rewriting jahr/monat),
// rather than needing a delete-and-reinsert.
func TestUpdateMovesInvoiceAcrossMonths(t *testing.T) {
	repo := newTestRepo(t)
	orig := core.CSVRow{Dateiname: "a.pdf", Jahr: "2026", Monat: "01", Auftraggeber: "AWS", Bruttobetrag: 10, VATID: "DE123"}
	if _, err := repo.Insert(orig); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	// Move to February with a new filename and VAT-ID.
	moved := orig
	moved.Monat = "02"
	moved.Dateiname = "b.pdf"
	moved.VATID = "DE999"
	if err := repo.Update("2026", "01", "a.pdf", moved); err != nil {
		t.Fatalf("Update: %v", err)
	}

	jan, err := repo.List("2026", "01")
	if err != nil {
		t.Fatal(err)
	}
	if len(jan) != 0 {
		t.Errorf("January should be empty after the move, got %d rows", len(jan))
	}

	feb, err := repo.List("2026", "02")
	if err != nil {
		t.Fatal(err)
	}
	if len(feb) != 1 {
		t.Fatalf("February should have 1 invoice, got %d", len(feb))
	}
	if feb[0].Dateiname != "b.pdf" || feb[0].VATID != "DE999" {
		t.Errorf("moved row = %+v, want Dateiname=b.pdf VATID=DE999", feb[0])
	}
}

// TestUpdateInPlace verifies an edit within the same month still works.
func TestUpdateInPlace(t *testing.T) {
	repo := newTestRepo(t)
	if _, err := repo.Insert(core.CSVRow{Dateiname: "a.pdf", Jahr: "2026", Monat: "03", Bruttobetrag: 5}); err != nil {
		t.Fatal(err)
	}
	upd := core.CSVRow{Dateiname: "a.pdf", Jahr: "2026", Monat: "03", Bruttobetrag: 7, Auftraggeber: "X"}
	if err := repo.Update("2026", "03", "a.pdf", upd); err != nil {
		t.Fatal(err)
	}
	rows, err := repo.List("2026", "03")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Bruttobetrag != 7 || rows[0].Auftraggeber != "X" {
		t.Errorf("in-place update failed: %+v", rows)
	}
}

// TestVATIDPersistsThroughDB confirms the issuer VAT-ID round-trips via the
// ustidnr column (the field was deduplicated onto CSVRow.VATID).
func TestVATIDPersistsThroughDB(t *testing.T) {
	repo := newTestRepo(t)
	if _, err := repo.Insert(core.CSVRow{Dateiname: "a.pdf", Jahr: "2026", Monat: "05", VATID: "ATU12345678"}); err != nil {
		t.Fatal(err)
	}
	rows, err := repo.List("2026", "05")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].VATID != "ATU12345678" {
		t.Errorf("VATID did not round-trip through DB: %+v", rows)
	}
}
