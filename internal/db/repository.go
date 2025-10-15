package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

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
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	repo := &Repository{
		db:     db,
		dbPath: dbPath,
	}

	// Initialize schema
	if err := repo.initSchema(); err != nil {
		db.Close()
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
	_, err := r.db.Exec(schemaSQL)
	return err
}

// Insert adds a new invoice to the database.
func (r *Repository) Insert(row core.CSVRow) (int64, error) {
	query := `
		INSERT INTO invoices (
			dateiname, rechnungsdatum, jahr, monat,
			auftraggeber, verwendungszweck, rechnungsnummer,
			betrag_netto, steuersatz_prozent, steuersatz_betrag, bruttobetrag,
			waehrung, gegenkonto, bankkonto, bezahldatum, teilzahlung,
			kommentar, betrag_netto_eur, gebuehr, hat_anhaenge, ustidnr
		) VALUES (
			?, ?, ?, ?,
			?, ?, ?,
			?, ?, ?, ?,
			?, ?, ?, ?, ?,
			?, ?, ?, ?, ?
		)
	`

	result, err := r.db.Exec(query,
		row.Dateiname, row.Rechnungsdatum, row.Jahr, row.Monat,
		row.Auftraggeber, row.Verwendungszweck, row.Rechnungsnummer,
		row.BetragNetto, row.SteuersatzProzent, row.SteuersatzBetrag, row.Bruttobetrag,
		row.Waehrung, row.Gegenkonto, row.Bankkonto, row.Bezahldatum, row.Teilzahlung,
		row.Kommentar, row.BetragNetto_EUR, row.Gebuehr, row.HatAnhaenge, row.UStIdNr,
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

// Update updates an existing invoice by dateiname (within a specific month).
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
			ustidnr = ?
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
		row.UStIdNr,
		jahr, monat, oldDateiname,
	)

	if err != nil {
		return fmt.Errorf("failed to update invoice: %w", err)
	}

	return nil
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
			kommentar, betrag_netto_eur, gebuehr, hat_anhaenge, ustidnr
		FROM invoices
		WHERE jahr = ? AND monat = ?
		ORDER BY rechnungsdatum DESC, dateiname ASC
	`

	rows, err := r.db.Query(query, jahr, monat)
	if err != nil {
		return nil, fmt.Errorf("failed to query invoices: %w", err)
	}
	defer rows.Close()

	var results []core.CSVRow

	for rows.Next() {
		var row core.CSVRow
		err := rows.Scan(
			&row.Dateiname, &row.Rechnungsdatum, &row.Jahr, &row.Monat,
			&row.Auftraggeber, &row.Verwendungszweck, &row.Rechnungsnummer,
			&row.BetragNetto, &row.SteuersatzProzent, &row.SteuersatzBetrag, &row.Bruttobetrag,
			&row.Waehrung, &row.Gegenkonto, &row.Bankkonto, &row.Bezahldatum, &row.Teilzahlung,
			&row.Kommentar, &row.BetragNetto_EUR, &row.Gebuehr, &row.HatAnhaenge, &row.UStIdNr,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
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
