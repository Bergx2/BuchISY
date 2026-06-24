package db

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// TestInitSchemaOnPreBelegnummerDB reproduces the "Boomstraat" crash: opening a
// database created BEFORE the belegnummer column existed must migrate cleanly,
// not fail with "no such column: belegnummer" while creating its index.
func TestInitSchemaOnPreBelegnummerDB(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")

	// Pre-belegnummer schema: only the columns that existed before the E14
	// ALTER-TABLE migrations (no belegnummer / buchung_ref / ausgangsrechnung /
	// trinkgeld / steuerzeilen / buchung / exportiert / wechselkurs / gebuehr_prozent).
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`CREATE TABLE invoices (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		dateiname TEXT NOT NULL,
		rechnungsdatum TEXT, jahr TEXT, monat TEXT,
		auftraggeber TEXT, verwendungszweck TEXT, rechnungsnummer TEXT,
		betrag_netto REAL, steuersatz_prozent REAL, steuersatz_betrag REAL,
		bruttobetrag REAL, waehrung TEXT, gegenkonto INTEGER, bankkonto TEXT,
		bezahldatum TEXT, teilzahlung BOOLEAN, kommentar TEXT,
		betrag_netto_eur REAL, gebuehr REAL, hat_anhaenge BOOLEAN, ustidnr TEXT,
		created_at DATETIME, updated_at DATETIME
	);`); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO invoices (dateiname, jahr, monat) VALUES ('alt.pdf','2024','03')`); err != nil {
		t.Fatal(err)
	}
	_ = raw.Close()

	// Opening through NewRepository must migrate + create the belegnummer index
	// without error.
	repo, err := NewRepository(path)
	if err != nil {
		t.Fatalf("NewRepository on pre-belegnummer DB failed: %v", err)
	}
	defer func() { _ = repo.Close() }()

	// The belegnummer column AND its index must now exist (this is the exact
	// thing that crashed Boomstraat). Query the column + the index directly.
	var n int
	if err := repo.db.QueryRow("SELECT COUNT(belegnummer) FROM invoices").Scan(&n); err != nil {
		t.Fatalf("belegnummer column missing after migration: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 migrated row, got %d", n)
	}
	var idx string
	if err := repo.db.QueryRow(
		"SELECT name FROM sqlite_master WHERE type='index' AND name='idx_invoices_belegnummer'").Scan(&idx); err != nil {
		t.Fatalf("belegnummer index not created: %v", err)
	}
}
