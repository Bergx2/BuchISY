package db

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bergx2/buchisy/internal/core"
	"github.com/bergx2/buchisy/internal/logging"
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

// TestDBTaxLinesRoundTrip verifies TaxLines and Trinkgeld persist through Insert/List.
func TestDBTaxLinesRoundTrip(t *testing.T) {
	repo := newTestRepo(t)
	row := core.CSVRow{
		Dateiname: "a.pdf", Jahr: "2026", Monat: "06",
		TaxLines: []core.TaxLine{
			{Netto: 14.20, SatzProzent: 19, MwStBetrag: 2.70},
			{Netto: 18.69, SatzProzent: 7, MwStBetrag: 1.31},
		},
		Trinkgeld: 2.00,
	}
	if _, err := repo.Insert(row); err != nil {
		t.Fatal(err)
	}
	rows, err := repo.List("2026", "06")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || len(rows[0].TaxLines) != 2 || rows[0].Trinkgeld != 2.00 {
		t.Fatalf("DB did not round-trip tax lines: %+v", rows)
	}
	// Verify MwStBetrag survived the round-trip (first line: 2.70).
	if got := rows[0].TaxLines[0].MwStBetrag; got < 2.695 || got > 2.705 {
		t.Errorf("TaxLines[0].MwStBetrag = %v, want ~2.70", got)
	}
}

// TestMigrateCSVToDatabaseBackfillsEmptyDB verifies the CSV→DB import that was
// previously never wired up: an empty database is back-filled from invoices.csv
// files under the storage root, and a second run is a no-op (idempotent).
func TestMigrateCSVToDatabaseBackfillsEmptyDB(t *testing.T) {
	repo := newTestRepo(t)
	logger, err := logging.New(t.TempDir(), logging.ERROR)
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	defer func() { _ = logger.Close() }()
	csvRepo := core.NewCSVRepository()

	// Build a storage root with one month folder containing a legacy CSV
	// (old Firmenname column → read via backward-compat into Auftraggeber).
	root := t.TempDir()
	monthDir := filepath.Join(root, "2026", "2026-03")
	if err := os.MkdirAll(monthDir, 0755); err != nil {
		t.Fatal(err)
	}
	row := core.CSVRow{Dateiname: "a.pdf", Jahr: "2026", Monat: "03", Auftraggeber: "AWS", Bruttobetrag: 10}
	if err := csvRepo.Append(filepath.Join(monthDir, "invoices.csv"), row); err != nil {
		t.Fatal(err)
	}

	// Empty DB → import should bring the row in.
	n, err := repo.MigrateCSVToDatabase(root, csvRepo, logger)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if n != 1 {
		t.Fatalf("imported %d, want 1", n)
	}
	rows, _ := repo.List("2026", "03")
	if len(rows) != 1 || rows[0].Auftraggeber != "AWS" {
		t.Fatalf("row not imported: %+v", rows)
	}

	// Second run: DB already populated → no-op.
	n2, err := repo.MigrateCSVToDatabase(root, csvRepo, logger)
	if err != nil {
		t.Fatalf("migrate (2nd): %v", err)
	}
	if n2 != 0 {
		t.Errorf("second run imported %d, want 0 (db not empty)", n2)
	}
}

// TestListHandlesNullTaxColumns reproduces the field bug where invoices created
// before the trinkgeld/steuerzeilen columns existed have NULL in those columns
// (the ALTER TABLE migration adds them without a default), and List then failed
// to scan NULL into float64/string — making the whole table appear empty.
func TestListHandlesNullTaxColumns(t *testing.T) {
	repo := newTestRepo(t)
	if _, err := repo.Insert(core.CSVRow{Dateiname: "a.pdf", Jahr: "2026", Monat: "06", Bruttobetrag: 10}); err != nil {
		t.Fatal(err)
	}
	// Simulate a pre-Phase-A row: the new columns are NULL.
	if _, err := repo.db.Exec(`UPDATE invoices SET trinkgeld = NULL, steuerzeilen = NULL`); err != nil {
		t.Fatal(err)
	}
	rows, err := repo.List("2026", "06")
	if err != nil {
		t.Fatalf("List must not error on NULL tax columns: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1 (NULL columns hid the row)", len(rows))
	}
	if rows[0].Trinkgeld != 0 {
		t.Errorf("NULL trinkgeld should read as 0, got %v", rows[0].Trinkgeld)
	}
}

func TestDBBookingRoundTrip(t *testing.T) {
	repo := newTestRepo(t)
	if _, err := repo.Insert(core.CSVRow{Dateiname: "a.pdf", Jahr: "2026", Monat: "06",
		Buchung: core.Booking{Entries: []core.BookingEntry{{Konto: 6640, Betrag: 12.71, Soll: true}}, Info: "x"}}); err != nil {
		t.Fatal(err)
	}
	rows, _ := repo.List("2026", "06")
	if len(rows) != 1 || len(rows[0].Buchung.Entries) != 1 || rows[0].Buchung.Info != "x" {
		t.Fatalf("DB booking round-trip failed: %+v", rows)
	}
}

func TestDBWechselkursRoundTrip(t *testing.T) {
	repo := newTestRepo(t)
	if _, err := repo.Insert(core.CSVRow{Dateiname: "a.pdf", Jahr: "2026", Monat: "06", Wechselkurs: 1.1583, GebuehrProzent: 2}); err != nil {
		t.Fatal(err)
	}
	rows, _ := repo.List("2026", "06")
	if len(rows) != 1 || rows[0].Wechselkurs != 1.1583 || rows[0].GebuehrProzent != 2 {
		t.Fatalf("DB kurs/prozent round-trip failed: %+v", rows)
	}
}

func TestBuchungRefRoundTrip(t *testing.T) {
	repo := newTestRepo(t)
	if _, err := repo.Insert(core.CSVRow{Dateiname: "a.pdf", Jahr: "2026", Monat: "06", BuchungRef: "Auszug.pdf|0|3"}); err != nil {
		t.Fatal(err)
	}
	rows, _ := repo.List("2026", "06")
	if len(rows) != 1 || rows[0].BuchungRef != "Auszug.pdf|0|3" {
		t.Fatalf("BuchungRef not persisted via DB: %+v", rows)
	}
	rows[0].BuchungRef = "Auszug2.pdf|1|5"
	if err := repo.Update("2026", "06", "a.pdf", rows[0]); err != nil {
		t.Fatal(err)
	}
	rows, _ = repo.List("2026", "06")
	if rows[0].BuchungRef != "Auszug2.pdf|1|5" {
		t.Errorf("Update did not persist BuchungRef: %q", rows[0].BuchungRef)
	}
}

func TestMarkExportedAndUpdateResets(t *testing.T) {
	repo := newTestRepo(t)
	if _, err := repo.Insert(core.CSVRow{Dateiname: "a.pdf", Jahr: "2026", Monat: "06"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.MarkExported("2026", "06", "a.pdf"); err != nil {
		t.Fatal(err)
	}
	rows, _ := repo.List("2026", "06")
	if len(rows) != 1 || !rows[0].Exportiert {
		t.Fatalf("expected Exportiert=true after MarkExported: %+v", rows)
	}
	// Updating the invoice must reset the exported flag.
	if err := repo.Update("2026", "06", "a.pdf", rows[0]); err != nil {
		t.Fatal(err)
	}
	rows, _ = repo.List("2026", "06")
	if rows[0].Exportiert {
		t.Error("Update must reset Exportiert to false")
	}
}

// TestFindDuplicate verifies that FindDuplicate matches across all months when
// Auftraggeber + Rechnungsnummer match, and returns the existing Belegnummer.
func TestFindDuplicate(t *testing.T) {
	repo := newTestRepo(t)

	// Insert an invoice in January with a Belegnummer
	jan := core.CSVRow{
		Dateiname: "invoice1.pdf", Jahr: "2026", Monat: "01",
		Auftraggeber: "AWS", Rechnungsnummer: "INV-001",
		Belegnummer: "2026-0001", Bruttobetrag: 119,
	}
	if _, err := repo.Insert(jan); err != nil {
		t.Fatalf("Insert January: %v", err)
	}

	// Try to find a duplicate in a different month with same company and invoice number
	searchRow := core.CSVRow{
		Auftraggeber: "AWS", Rechnungsnummer: "INV-001",
	}
	label, found, err := repo.FindDuplicate(searchRow)
	if err != nil {
		t.Fatalf("FindDuplicate: %v", err)
	}
	if !found {
		t.Error("expected to find duplicate across months")
	}
	if label != "2026-0001" {
		t.Errorf("expected Belegnummer '2026-0001', got '%s'", label)
	}

	// Test with case-insensitive and whitespace-tolerant match
	searchRow2 := core.CSVRow{
		Auftraggeber: "aws", Rechnungsnummer: "INV-001",
	}
	label2, found2, err := repo.FindDuplicate(searchRow2)
	if err != nil {
		t.Fatalf("FindDuplicate case-insensitive: %v", err)
	}
	if !found2 {
		t.Error("expected to find duplicate with different case")
	}
	if label2 != "2026-0001" {
		t.Errorf("expected Belegnummer '2026-0001', got '%s'", label2)
	}

	// Test with blank Rechnungsnummer → no early signal
	searchBlank := core.CSVRow{
		Auftraggeber: "AWS", Rechnungsnummer: "",
	}
	_, foundBlank, err := repo.FindDuplicate(searchBlank)
	if err != nil {
		t.Fatalf("FindDuplicate blank: %v", err)
	}
	if foundBlank {
		t.Error("expected no duplicate found for blank Rechnungsnummer")
	}

	// Test non-existent combination
	searchNone := core.CSVRow{
		Auftraggeber: "Google", Rechnungsnummer: "INV-999",
	}
	_, foundNone, err := repo.FindDuplicate(searchNone)
	if err != nil {
		t.Fatalf("FindDuplicate non-existent: %v", err)
	}
	if foundNone {
		t.Error("expected no duplicate found for non-existent company+invoice")
	}

	// Test fallback to Dateiname when Belegnummer is empty
	feb := core.CSVRow{
		Dateiname: "invoice2.pdf", Jahr: "2026", Monat: "02",
		Auftraggeber: "GCP", Rechnungsnummer: "INV-002",
		Belegnummer: "", // empty, should fallback to dateiname
		Bruttobetrag: 150,
	}
	if _, err := repo.Insert(feb); err != nil {
		t.Fatalf("Insert February: %v", err)
	}
	searchFeb := core.CSVRow{
		Auftraggeber: "GCP", Rechnungsnummer: "INV-002",
	}
	labelFallback, foundFeb, err := repo.FindDuplicate(searchFeb)
	if err != nil {
		t.Fatalf("FindDuplicate fallback: %v", err)
	}
	if !foundFeb {
		t.Error("expected to find duplicate (fallback to dateiname)")
	}
	if labelFallback != "invoice2.pdf" {
		t.Errorf("expected fallback Dateiname 'invoice2.pdf', got '%s'", labelFallback)
	}
}
