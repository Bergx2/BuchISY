package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite" // SQLite driver

	"github.com/bergx2/buchisy/internal/core"
)

// Repository manages invoice data in SQLite database.
type Repository struct {
	db     *sql.DB
	dbPath string
}

// NewRepository creates a new database repository.
// dbPath should be the full path to the SQLite database file (e.g., /path/to/invoices.db)
func NewRepository(dbPath string) (*Repository, error) {
	// Ensure directory exists
	dir := filepath.Dir(dbPath)
	if err := ensureDir(dir); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	// Open database
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	repo := &Repository{
		db:     db,
		dbPath: dbPath,
	}

	// Initialize schema
	if err := repo.initSchema(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return repo, nil
}

// Close closes the database connection.
func (r *Repository) Close() error {
	if r.db != nil {
		return r.db.Close()
	}
	return nil
}

// initSchema initializes the database schema.
func (r *Repository) initSchema() error {
	if _, err := r.db.Exec(schemaSQL); err != nil {
		return err
	}

	// Add columns introduced after the initial schema (idempotent). Defaults
	// keep pre-existing rows non-NULL; List also reads them NULL-safely for DBs
	// already migrated without a default.
	for _, col := range []string{
		"ALTER TABLE invoices ADD COLUMN trinkgeld REAL DEFAULT 0",
		"ALTER TABLE invoices ADD COLUMN steuerzeilen TEXT DEFAULT ''",
	} {
		if _, err := r.db.Exec(col); err != nil &&
			!strings.Contains(err.Error(), "duplicate column name") {
			return fmt.Errorf("failed to add column: %w", err)
		}
	}
	return nil
}

// Insert adds a new invoice to the database.
func (r *Repository) Insert(row core.CSVRow) (int64, error) {
	query := `
		INSERT INTO invoices (
			dateiname, rechnungsdatum, jahr, monat,
			auftraggeber, verwendungszweck, rechnungsnummer,
			betrag_netto, steuersatz_prozent, steuersatz_betrag, bruttobetrag,
			waehrung, gegenkonto, bankkonto, bezahldatum, teilzahlung,
			kommentar, betrag_netto_eur, gebuehr, hat_anhaenge, ustidnr,
			trinkgeld, steuerzeilen
		) VALUES (
			?, ?, ?, ?,
			?, ?, ?,
			?, ?, ?, ?,
			?, ?, ?, ?, ?,
			?, ?, ?, ?, ?,
			?, ?
		)
	`

	result, err := r.db.Exec(query,
		row.Dateiname, row.Rechnungsdatum, row.Jahr, row.Monat,
		row.Auftraggeber, row.Verwendungszweck, row.Rechnungsnummer,
		row.BetragNetto, row.SteuersatzProzent, row.SteuersatzBetrag, row.Bruttobetrag,
		row.Waehrung, row.Gegenkonto, row.Bankkonto, row.Bezahldatum, row.Teilzahlung,
		row.Kommentar, row.BetragNetto_EUR, row.Gebuehr, row.HatAnhaenge, row.VATID,
		row.Trinkgeld, core.MarshalTaxLines(row.TaxLines),
	)
	if err != nil {
		return 0, fmt.Errorf("failed to insert invoice: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to get last insert id: %w", err)
	}

	return id, nil
}

// Update updates an existing invoice located by (jahr, monat, oldDateiname).
// The row's own Jahr/Monat become the new filing period, so this also handles
// moving an invoice to a different month in a single statement (no
// delete-and-reinsert needed).
func (r *Repository) Update(jahr, monat string, oldDateiname string, row core.CSVRow) error {
	query := `
		UPDATE invoices SET
			dateiname = ?,
			rechnungsdatum = ?,
			auftraggeber = ?,
			verwendungszweck = ?,
			rechnungsnummer = ?,
			betrag_netto = ?,
			steuersatz_prozent = ?,
			steuersatz_betrag = ?,
			bruttobetrag = ?,
			waehrung = ?,
			gegenkonto = ?,
			bankkonto = ?,
			bezahldatum = ?,
			teilzahlung = ?,
			kommentar = ?,
			betrag_netto_eur = ?,
			gebuehr = ?,
			hat_anhaenge = ?,
			ustidnr = ?, -- stores the issuer VAT-ID (core.CSVRow.VATID)
			trinkgeld = ?,
			steuerzeilen = ?,
			jahr = ?,
			monat = ?
		WHERE jahr = ? AND monat = ? AND dateiname = ?
	`

	_, err := r.db.Exec(query,
		row.Dateiname,
		row.Rechnungsdatum,
		row.Auftraggeber,
		row.Verwendungszweck,
		row.Rechnungsnummer,
		row.BetragNetto,
		row.SteuersatzProzent,
		row.SteuersatzBetrag,
		row.Bruttobetrag,
		row.Waehrung,
		row.Gegenkonto,
		row.Bankkonto,
		row.Bezahldatum,
		row.Teilzahlung,
		row.Kommentar,
		row.BetragNetto_EUR,
		row.Gebuehr,
		row.HatAnhaenge,
		row.VATID,
		row.Trinkgeld,
		core.MarshalTaxLines(row.TaxLines),
		row.Jahr, row.Monat,
		jahr, monat, oldDateiname,
	)

	if err != nil {
		return fmt.Errorf("failed to update invoice: %w", err)
	}

	return nil
}

// Count returns the total number of invoices stored in the database.
func (r *Repository) Count() (int, error) {
	var n int
	if err := r.db.QueryRow(`SELECT count(*) FROM invoices`).Scan(&n); err != nil {
		return 0, fmt.Errorf("failed to count invoices: %w", err)
	}
	return n, nil
}

// Delete removes an invoice from the database.
func (r *Repository) Delete(jahr, monat, dateiname string) error {
	query := `DELETE FROM invoices WHERE jahr = ? AND monat = ? AND dateiname = ?`

	result, err := r.db.Exec(query, jahr, monat, dateiname)
	if err != nil {
		return fmt.Errorf("failed to delete invoice: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("invoice not found")
	}

	return nil
}

// List retrieves all invoices for a specific month.
func (r *Repository) List(jahr, monat string) ([]core.CSVRow, error) {
	query := `
		SELECT
			dateiname, rechnungsdatum, jahr, monat,
			auftraggeber, verwendungszweck, rechnungsnummer,
			betrag_netto, steuersatz_prozent, steuersatz_betrag, bruttobetrag,
			waehrung, gegenkonto, bankkonto, bezahldatum, teilzahlung,
			kommentar, betrag_netto_eur, gebuehr, hat_anhaenge, ustidnr,
			trinkgeld, steuerzeilen
		FROM invoices
		WHERE jahr = ? AND monat = ?
		ORDER BY rechnungsdatum DESC, dateiname ASC
	`

	rows, err := r.db.Query(query, jahr, monat)
	if err != nil {
		return nil, fmt.Errorf("failed to query invoices: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []core.CSVRow

	for rows.Next() {
		var row core.CSVRow
		// trinkgeld/steuerzeilen are NULL for rows created before those columns
		// were added by the ALTER-TABLE migration, so read them NULL-safely
		// (NULL → 0 / "") instead of failing the whole List.
		var steuerzeilen sql.NullString
		var trinkgeld sql.NullFloat64
		err := rows.Scan(
			&row.Dateiname, &row.Rechnungsdatum, &row.Jahr, &row.Monat,
			&row.Auftraggeber, &row.Verwendungszweck, &row.Rechnungsnummer,
			&row.BetragNetto, &row.SteuersatzProzent, &row.SteuersatzBetrag, &row.Bruttobetrag,
			&row.Waehrung, &row.Gegenkonto, &row.Bankkonto, &row.Bezahldatum, &row.Teilzahlung,
			&row.Kommentar, &row.BetragNetto_EUR, &row.Gebuehr, &row.HatAnhaenge, &row.VATID,
			&trinkgeld, &steuerzeilen,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		row.Trinkgeld = trinkgeld.Float64

		row.TaxLines = core.ParseTaxLines(steuerzeilen.String)
		if len(row.TaxLines) == 0 {
			// Pass brutto as the 4th arg so gross-only rows still get a usable line.
			row.TaxLines = core.ReconstructTaxLines(row.BetragNetto, row.SteuersatzProzent, row.SteuersatzBetrag, row.Bruttobetrag)
		}

		results = append(results, row)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return results, nil
}

// IsDuplicate checks if an invoice already exists with the same key fields.
func (r *Repository) IsDuplicate(jahr, monat string, row core.CSVRow) (bool, error) {
	query := `
		SELECT COUNT(*) FROM invoices
		WHERE jahr = ? AND monat = ?
		AND LOWER(TRIM(auftraggeber)) = LOWER(TRIM(?))
		AND rechnungsnummer = ?
		AND rechnungsdatum = ?
		AND ABS(bruttobetrag - ?) < 0.01
		AND teilzahlung = ?
	`

	var count int
	err := r.db.QueryRow(query,
		jahr, monat,
		row.Auftraggeber,
		row.Rechnungsnummer,
		row.Rechnungsdatum,
		row.Bruttobetrag,
		row.Teilzahlung,
	).Scan(&count)

	if err != nil {
		return false, fmt.Errorf("failed to check duplicate: %w", err)
	}

	return count > 0, nil
}

// ensureDir ensures a directory exists.
func ensureDir(path string) error {
	return os.MkdirAll(path, 0755)
}
