package db

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	_ "modernc.org/sqlite" // SQLite driver

	"github.com/bergx2/buchisy/internal/core"
)

// ErrPeriodLocked is returned when a mutation is attempted on a locked period.
var ErrPeriodLocked = errors.New("Periode ist festgeschrieben")

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
		"ALTER TABLE invoices ADD COLUMN buchung TEXT DEFAULT ''",
		"ALTER TABLE invoices ADD COLUMN exportiert INTEGER DEFAULT 0",
		"ALTER TABLE invoices ADD COLUMN wechselkurs REAL DEFAULT 0",
		"ALTER TABLE invoices ADD COLUMN gebuehr_prozent REAL DEFAULT 0",
		"ALTER TABLE invoices ADD COLUMN buchung_ref TEXT DEFAULT ''",
		"ALTER TABLE invoices ADD COLUMN belegnummer TEXT DEFAULT ''",
		"ALTER TABLE invoices ADD COLUMN ausgangsrechnung INTEGER DEFAULT 0",
	} {
		if _, err := r.db.Exec(col); err != nil &&
			!strings.Contains(err.Error(), "duplicate column name") {
			return fmt.Errorf("failed to add column: %w", err)
		}
	}

	// Indexes on migration-added columns must be created AFTER the ALTERs above,
	// otherwise opening a pre-belegnummer database fails with
	// "no such column: belegnummer".
	if _, err := r.db.Exec(
		"CREATE INDEX IF NOT EXISTS idx_invoices_belegnummer ON invoices(belegnummer)"); err != nil {
		return fmt.Errorf("failed to create belegnummer index: %w", err)
	}
	return nil
}

// Insert adds a new invoice to the database.
func (r *Repository) Insert(row core.CSVRow) (int64, error) {
	if locked, err := r.IsPeriodLocked(row.Jahr, row.Monat); err != nil {
		return 0, fmt.Errorf("period lock check: %w", err)
	} else if locked {
		return 0, ErrPeriodLocked
	}

	query := `
		INSERT INTO invoices (
			dateiname, rechnungsdatum, jahr, monat,
			auftraggeber, verwendungszweck, rechnungsnummer,
			betrag_netto, steuersatz_prozent, steuersatz_betrag, bruttobetrag,
			waehrung, gegenkonto, bankkonto, bezahldatum, teilzahlung,
			kommentar, betrag_netto_eur, gebuehr, hat_anhaenge, ustidnr,
			trinkgeld, steuerzeilen, buchung, exportiert,
			wechselkurs, gebuehr_prozent, buchung_ref, belegnummer, ausgangsrechnung
		) VALUES (
			?, ?, ?, ?,
			?, ?, ?,
			?, ?, ?, ?,
			?, ?, ?, ?, ?,
			?, ?, ?, ?, ?,
			?, ?, ?, ?,
			?, ?, ?, ?, ?
		)
	`

	result, err := r.db.Exec(query,
		row.Dateiname, row.Rechnungsdatum, row.Jahr, row.Monat,
		row.Auftraggeber, row.Verwendungszweck, row.Rechnungsnummer,
		row.BetragNetto, row.SteuersatzProzent, row.SteuersatzBetrag, row.Bruttobetrag,
		row.Waehrung, row.Gegenkonto, row.Bankkonto, row.Bezahldatum, row.Teilzahlung,
		row.Kommentar, row.BetragNetto_EUR, row.Gebuehr, row.HatAnhaenge, row.VATID,
		row.Trinkgeld, core.MarshalTaxLines(row.TaxLines), core.MarshalBooking(row.Buchung), 0,
		row.Wechselkurs, row.GebuehrProzent, row.BuchungRef, row.Belegnummer, row.Ausgangsrechnung,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to insert invoice: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to get last insert id: %w", err)
	}

	// Best-effort audit log: log a warning on failure but never fail the Insert.
	if auditErr := r.LogAudit(core.AuditEntry{
		Aktion:     "create",
		Entitaet:   "invoice",
		Schluessel: row.Belegnummer + " " + row.Dateiname,
		Details:    "",
	}); auditErr != nil {
		log.Printf("[WARN] audit_log create failed: %v", auditErr)
	}

	return id, nil
}

// Update updates an existing invoice located by (jahr, monat, oldDateiname).
// The row's own Jahr/Monat become the new filing period, so this also handles
// moving an invoice to a different month in a single statement (no
// delete-and-reinsert needed).
func (r *Repository) Update(jahr, monat string, oldDateiname string, row core.CSVRow) error {
	// Block if the OLD period is locked.
	if locked, err := r.IsPeriodLocked(jahr, monat); err != nil {
		return fmt.Errorf("period lock check (old): %w", err)
	} else if locked {
		return ErrPeriodLocked
	}
	// Block if the NEW period (may differ on cross-month move) is also locked.
	if row.Jahr != jahr || row.Monat != monat {
		if locked, err := r.IsPeriodLocked(row.Jahr, row.Monat); err != nil {
			return fmt.Errorf("period lock check (new): %w", err)
		} else if locked {
			return ErrPeriodLocked
		}
	}

	// Fetch the before-image for the audit diff (best-effort; silently skip on failure).
	var oldRow core.CSVRow
	var hasOld bool
	if before, found, fetchErr := r.getByKey(jahr, monat, oldDateiname); fetchErr == nil {
		oldRow = before
		hasOld = found
	}

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
			buchung = ?,
			exportiert = 0,
			wechselkurs = ?,
			gebuehr_prozent = ?,
			buchung_ref = ?,
			belegnummer = ?,
			ausgangsrechnung = ?,
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
		core.MarshalBooking(row.Buchung),
		row.Wechselkurs,
		row.GebuehrProzent,
		row.BuchungRef,
		row.Belegnummer,
		row.Ausgangsrechnung,
		row.Jahr, row.Monat,
		jahr, monat, oldDateiname,
	)

	if err != nil {
		return fmt.Errorf("failed to update invoice: %w", err)
	}

	// Best-effort audit log.
	diff := "{}"
	if hasOld {
		diff = core.DiffFields(oldRow, row)
	}
	if auditErr := r.LogAudit(core.AuditEntry{
		Aktion:     "update",
		Entitaet:   "invoice",
		Schluessel: row.Belegnummer + " " + oldDateiname,
		Details:    diff,
	}); auditErr != nil {
		log.Printf("[WARN] audit_log update failed: %v", auditErr)
	}

	return nil
}

// NextBelegnummer returns the next sequential receipt number for a filing year,
// formatted "YYYY-NNNN" (4-digit zero-padded). The sequence is per database
// (i.e. per profile) and per year: it keys purely on the "YYYY-" prefix of the
// stored belegnummer, so it stays correct regardless of a row's jahr column.
// Empty or non-conforming belegnummern are ignored. The value is read, not
// reserved, so it is stable across cancelled dialogs (no gaps).
//
// The lexical MAX equals the numeric max because the suffix is zero-padded to
// four digits — correct for up to 9999 receipts per year, far beyond any
// realistic per-profile volume.
func (r *Repository) NextBelegnummer(jahr string) (string, error) {
	var max sql.NullString
	if err := r.db.QueryRow(
		`SELECT MAX(belegnummer) FROM invoices WHERE belegnummer LIKE ?`,
		jahr+"-%",
	).Scan(&max); err != nil {
		return "", fmt.Errorf("failed to read max belegnummer: %w", err)
	}
	n := 0
	if max.Valid && max.String != "" {
		// "YYYY-NNNN" → take the numeric suffix after the first "-".
		if parts := strings.SplitN(max.String, "-", 2); len(parts) == 2 {
			if v, err := strconv.Atoi(parts[1]); err == nil {
				n = v
			}
		}
	}
	return fmt.Sprintf("%s-%04d", jahr, n+1), nil
}

// RenumberBelegnummern reassigns every invoice's Belegnummer per year, in
// chronological order (by Rechnungsdatum, ties broken by id), gap-free as
// "YYYY-NNNN". Backfills empty numbers AND closes gaps from deletions. Returns
// the number of invoices renumbered. Single-user desktop app, so a one-shot
// rewrite is safe.
func (r *Repository) RenumberBelegnummern() (int, error) {
	_, err := r.db.Exec(`
		WITH numbered AS (
			SELECT id, printf('%s-%04d', jahr,
				ROW_NUMBER() OVER (
					PARTITION BY jahr
					ORDER BY substr(rechnungsdatum,7,4)||substr(rechnungsdatum,4,2)||substr(rechnungsdatum,1,2), id
				)) AS bn
			FROM invoices
		)
		UPDATE invoices SET belegnummer = (SELECT bn FROM numbered WHERE numbered.id = invoices.id)
		WHERE id IN (SELECT id FROM numbered)
	`)
	if err != nil {
		return 0, fmt.Errorf("failed to renumber belegnummern: %w", err)
	}
	return r.Count()
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
	if locked, err := r.IsPeriodLocked(jahr, monat); err != nil {
		return fmt.Errorf("period lock check: %w", err)
	} else if locked {
		return ErrPeriodLocked
	}

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

	// Best-effort audit log.
	if auditErr := r.LogAudit(core.AuditEntry{
		Aktion:     "delete",
		Entitaet:   "invoice",
		Schluessel: dateiname,
		Details:    "",
	}); auditErr != nil {
		log.Printf("[WARN] audit_log delete failed: %v", auditErr)
	}

	return nil
}

// scanInvoiceRows reads all rows from a *sql.Rows result set produced by the
// standard SELECT column list (dateiname … ausgangsrechnung) and assembles the
// corresponding []core.CSVRow slice. The caller is responsible for closing rows.
func scanInvoiceRows(rows *sql.Rows) ([]core.CSVRow, error) {
	var results []core.CSVRow

	for rows.Next() {
		var row core.CSVRow
		// trinkgeld/steuerzeilen/buchung/exportiert are NULL for rows created before
		// those columns were added by the ALTER-TABLE migration, so read them
		// NULL-safely (NULL → 0 / "") instead of failing the whole scan.
		var steuerzeilen sql.NullString
		var trinkgeld sql.NullFloat64
		var buchung sql.NullString
		var exportiert sql.NullInt64
		var wechselkurs sql.NullFloat64
		var gebuehrProzent sql.NullFloat64
		var buchungRef sql.NullString
		var belegnummer sql.NullString
		var ausgangsrechnung sql.NullInt64
		err := rows.Scan(
			&row.Dateiname, &row.Rechnungsdatum, &row.Jahr, &row.Monat,
			&row.Auftraggeber, &row.Verwendungszweck, &row.Rechnungsnummer,
			&row.BetragNetto, &row.SteuersatzProzent, &row.SteuersatzBetrag, &row.Bruttobetrag,
			&row.Waehrung, &row.Gegenkonto, &row.Bankkonto, &row.Bezahldatum, &row.Teilzahlung,
			&row.Kommentar, &row.BetragNetto_EUR, &row.Gebuehr, &row.HatAnhaenge, &row.VATID,
			&trinkgeld, &steuerzeilen, &buchung, &exportiert,
			&wechselkurs, &gebuehrProzent, &buchungRef, &belegnummer, &ausgangsrechnung,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		row.Trinkgeld = trinkgeld.Float64
		row.Exportiert = exportiert.Int64 != 0
		row.Wechselkurs = wechselkurs.Float64
		row.GebuehrProzent = gebuehrProzent.Float64
		row.BuchungRef = buchungRef.String
		row.Belegnummer = belegnummer.String
		row.Ausgangsrechnung = ausgangsrechnung.Int64 != 0

		row.TaxLines = core.ParseTaxLines(steuerzeilen.String)
		if len(row.TaxLines) == 0 {
			// Pass brutto as the 4th arg so gross-only rows still get a usable line.
			row.TaxLines = core.ReconstructTaxLines(row.BetragNetto, row.SteuersatzProzent, row.SteuersatzBetrag, row.Bruttobetrag)
		}
		row.Buchung = core.ParseBooking(buchung.String)

		results = append(results, row)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return results, nil
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
			trinkgeld, steuerzeilen, buchung, exportiert,
			wechselkurs, gebuehr_prozent, buchung_ref, belegnummer, ausgangsrechnung
		FROM invoices
		WHERE jahr = ? AND monat = ?
		ORDER BY rechnungsdatum DESC, dateiname ASC
	`

	rows, err := r.db.Query(query, jahr, monat)
	if err != nil {
		return nil, fmt.Errorf("failed to query invoices: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanInvoiceRows(rows)
}

// SearchInvoices searches invoices across ALL months for a given query string.
// It matches (case-insensitively) against auftraggeber, verwendungszweck,
// rechnungsnummer, and belegnummer. Results are ordered by rechnungsdatum DESC,
// limited to 200 rows. An empty query returns nil, nil.
func (r *Repository) SearchInvoices(query string) ([]core.CSVRow, error) {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil, nil
	}
	like := "%" + q + "%"

	sqlQuery := `
		SELECT
			dateiname, rechnungsdatum, jahr, monat,
			auftraggeber, verwendungszweck, rechnungsnummer,
			betrag_netto, steuersatz_prozent, steuersatz_betrag, bruttobetrag,
			waehrung, gegenkonto, bankkonto, bezahldatum, teilzahlung,
			kommentar, betrag_netto_eur, gebuehr, hat_anhaenge, ustidnr,
			trinkgeld, steuerzeilen, buchung, exportiert,
			wechselkurs, gebuehr_prozent, buchung_ref, belegnummer, ausgangsrechnung
		FROM invoices
		WHERE LOWER(auftraggeber) LIKE ?
		   OR LOWER(verwendungszweck) LIKE ?
		   OR LOWER(rechnungsnummer) LIKE ?
		   OR LOWER(belegnummer) LIKE ?
		ORDER BY rechnungsdatum DESC
		LIMIT 200
	`

	rows, err := r.db.Query(sqlQuery, like, like, like, like)
	if err != nil {
		return nil, fmt.Errorf("failed to search invoices: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanInvoiceRows(rows)
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

// FindDuplicate searches across ALL months for an invoice with the same
// Auftraggeber (case-insensitive, trimmed) and non-empty Rechnungsnummer.
// Returns the existing invoice's Belegnummer (or Dateiname as fallback) as label.
// If row.Rechnungsnummer is blank, returns found=false (no early signal).
func (r *Repository) FindDuplicate(row core.CSVRow) (label string, found bool, err error) {
	// Guard: blank Rechnungsnummer → no early signal
	if strings.TrimSpace(row.Rechnungsnummer) == "" {
		return "", false, nil
	}

	query := `
		SELECT belegnummer, dateiname FROM invoices
		WHERE LOWER(TRIM(auftraggeber)) = LOWER(TRIM(?))
		AND rechnungsnummer = ?
		AND rechnungsnummer <> ''
		LIMIT 1
	`

	var belegnummer sql.NullString
	var dateiname string

	err = r.db.QueryRow(query, row.Auftraggeber, row.Rechnungsnummer).Scan(&belegnummer, &dateiname)

	if err == sql.ErrNoRows {
		// No match found
		return "", false, nil
	}

	if err != nil {
		return "", false, fmt.Errorf("failed to find duplicate: %w", err)
	}

	// Return belegnummer if available, else fallback to dateiname
	if belegnummer.Valid && belegnummer.String != "" {
		label = belegnummer.String
	} else {
		label = dateiname
	}

	return label, true, nil
}

// MarkExported flags an invoice as having been included in a booking export.
func (r *Repository) MarkExported(jahr, monat, dateiname string) error {
	_, err := r.db.Exec(`UPDATE invoices SET exportiert = 1 WHERE jahr = ? AND monat = ? AND dateiname = ?`, jahr, monat, dateiname)
	if err != nil {
		return fmt.Errorf("failed to mark exported: %w", err)
	}
	return nil
}

// getByKey fetches a single invoice by (jahr, monat, dateiname). Returns
// (row, true, nil) when found, (zero, false, nil) when not found, or
// (zero, false, err) on a query error.
func (r *Repository) getByKey(jahr, monat, dateiname string) (core.CSVRow, bool, error) {
	query := `
		SELECT
			dateiname, rechnungsdatum, jahr, monat,
			auftraggeber, verwendungszweck, rechnungsnummer,
			betrag_netto, steuersatz_prozent, steuersatz_betrag, bruttobetrag,
			waehrung, gegenkonto, bankkonto, bezahldatum, teilzahlung,
			kommentar, betrag_netto_eur, gebuehr, hat_anhaenge, ustidnr,
			trinkgeld, steuerzeilen, buchung, exportiert,
			wechselkurs, gebuehr_prozent, buchung_ref, belegnummer, ausgangsrechnung
		FROM invoices
		WHERE jahr = ? AND monat = ? AND dateiname = ?
		LIMIT 1
	`
	rows, err := r.db.Query(query, jahr, monat, dateiname)
	if err != nil {
		return core.CSVRow{}, false, fmt.Errorf("getByKey query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	results, err := scanInvoiceRows(rows)
	if err != nil {
		return core.CSVRow{}, false, err
	}
	if len(results) == 0 {
		return core.CSVRow{}, false, nil
	}
	return results[0], true, nil
}

// LogAudit appends an entry to the audit_log table. It is best-effort: callers
// log a warning and continue on error rather than propagating the failure.
func (r *Repository) LogAudit(e core.AuditEntry) error {
	_, err := r.db.Exec(
		`INSERT INTO audit_log (aktion, entitaet, schluessel, details) VALUES (?, ?, ?, ?)`,
		e.Aktion, e.Entitaet, e.Schluessel, e.Details,
	)
	if err != nil {
		return fmt.Errorf("audit_log insert: %w", err)
	}
	return nil
}

// AuditLog returns up to limit audit entries ordered newest-first.
func (r *Repository) AuditLog(limit int) ([]core.AuditEntry, error) {
	rows, err := r.db.Query(
		`SELECT ts, aktion, entitaet, schluessel, details
		 FROM audit_log
		 ORDER BY ts DESC, id DESC
		 LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("audit_log query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var entries []core.AuditEntry
	for rows.Next() {
		var e core.AuditEntry
		var ts, entitaet, schluessel, details sql.NullString
		if err := rows.Scan(&ts, &e.Aktion, &entitaet, &schluessel, &details); err != nil {
			return nil, fmt.Errorf("audit_log scan: %w", err)
		}
		e.TS = ts.String
		e.Entitaet = entitaet.String
		e.Schluessel = schluessel.String
		e.Details = details.String
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("audit_log iterate: %w", err)
	}
	return entries, nil
}

// LockPeriod marks a filing period (jahr/monat) as locked (festgeschrieben).
// Subsequent Insert/Update/Delete calls on this period will return ErrPeriodLocked.
// The action is recorded in the audit log.
func (r *Repository) LockPeriod(jahr, monat string) error {
	_, err := r.db.Exec(
		`INSERT OR IGNORE INTO period_locks (jahr, monat) VALUES (?, ?)`,
		jahr, monat,
	)
	if err != nil {
		return fmt.Errorf("lock period: %w", err)
	}
	if auditErr := r.LogAudit(core.AuditEntry{
		Aktion:   "lock",
		Entitaet: "period",
		Schluessel: jahr + "-" + monat,
	}); auditErr != nil {
		log.Printf("[WARN] audit_log lock failed: %v", auditErr)
	}
	return nil
}

// UnlockPeriod removes the lock on a filing period (jahr/monat).
// The action is recorded in the audit log.
func (r *Repository) UnlockPeriod(jahr, monat string) error {
	_, err := r.db.Exec(
		`DELETE FROM period_locks WHERE jahr = ? AND monat = ?`,
		jahr, monat,
	)
	if err != nil {
		return fmt.Errorf("unlock period: %w", err)
	}
	if auditErr := r.LogAudit(core.AuditEntry{
		Aktion:   "unlock",
		Entitaet: "period",
		Schluessel: jahr + "-" + monat,
	}); auditErr != nil {
		log.Printf("[WARN] audit_log unlock failed: %v", auditErr)
	}
	return nil
}

// IsPeriodLocked reports whether the given filing period is currently locked.
func (r *Repository) IsPeriodLocked(jahr, monat string) (bool, error) {
	var count int
	if err := r.db.QueryRow(
		`SELECT COUNT(*) FROM period_locks WHERE jahr = ? AND monat = ?`,
		jahr, monat,
	).Scan(&count); err != nil {
		return false, fmt.Errorf("is period locked: %w", err)
	}
	return count > 0, nil
}

// LockedPeriods returns the list of locked periods formatted as "YYYY-MM".
func (r *Repository) LockedPeriods() ([]string, error) {
	rows, err := r.db.Query(
		`SELECT jahr, monat FROM period_locks ORDER BY jahr, monat`,
	)
	if err != nil {
		return nil, fmt.Errorf("locked periods: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var periods []string
	for rows.Next() {
		var jahr, monat string
		if err := rows.Scan(&jahr, &monat); err != nil {
			return nil, fmt.Errorf("locked periods scan: %w", err)
		}
		periods = append(periods, jahr+"-"+monat)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("locked periods iterate: %w", err)
	}
	return periods, nil
}

// ensureDir ensures a directory exists.
func ensureDir(path string) error {
	return os.MkdirAll(path, 0755)
}
