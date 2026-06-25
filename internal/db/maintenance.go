package db

import (
	"database/sql"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"

	"github.com/bergx2/buchisy/internal/core"
)

// round2 rounds a float64 to 2 decimal places.
func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

// WipeDatabase deletes all data from the database and recreates the schema.
// This is a destructive operation - all invoice data will be lost!
func (r *Repository) WipeDatabase() error {
	// Drop all tables
	_, err := r.db.Exec(`DROP TABLE IF EXISTS invoices`)
	if err != nil {
		return fmt.Errorf("failed to drop tables: %w", err)
	}

	// Recreate schema
	if err := r.initSchema(); err != nil {
		return fmt.Errorf("failed to recreate schema: %w", err)
	}

	// Delete migration marker if it exists
	markerPath := r.getMigrationMarkerPath()
	if markerPath != "" {
		_ = os.Remove(markerPath)
	}

	return nil
}

// DeleteDatabase closes and deletes the database file completely.
// The repository will be unusable after this operation.
func (r *Repository) DeleteDatabase() error {
	// Close connection first
	if err := r.Close(); err != nil {
		return fmt.Errorf("failed to close database: %w", err)
	}

	// Delete the database file
	if err := os.Remove(r.dbPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete database file: %w", err)
	}

	// Delete migration marker
	markerPath := r.getMigrationMarkerPath()
	if markerPath != "" {
		_ = os.Remove(markerPath)
	}

	return nil
}

// getMigrationMarkerPath returns the path to the migration marker file.
func (r *Repository) getMigrationMarkerPath() string {
	if r.dbPath == "" {
		return ""
	}
	dir := filepath.Dir(r.dbPath)
	return filepath.Join(dir, ".migrated")
}

// RebookForeignToEUR rescales the stored booking of each foreign-currency invoice
// (Waehrung != "" && != "EUR", Wechselkurs > 0) to EUR by dividing every booking
// entry's Betrag by the rate (round2), then nudging the payment entry so
// Σ Soll = Σ Haben. Idempotent: a booking whose total already matches the EUR gross
// (Bruttobetrag/Wechselkurs) rather than the foreign gross is skipped. Foreign
// invoices without a rate are counted in rateMissing and left untouched. EUR
// invoices are never touched. Writes an audit entry per change.
func (r *Repository) RebookForeignToEUR() (converted, skipped, rateMissing int, err error) {
	// Fetch all foreign-currency invoices that have a booking.
	// We do NOT filter wechselkurs > 0 here so we can count rateMissing separately.
	sqlQuery := `
		SELECT id, dateiname, belegnummer, bruttobetrag, wechselkurs, buchung, ausgangsrechnung
		FROM invoices
		WHERE waehrung != '' AND waehrung != 'EUR' AND buchung != '' AND buchung IS NOT NULL
	`
	rows, qErr := r.db.Query(sqlQuery)
	if qErr != nil {
		err = fmt.Errorf("RebookForeignToEUR query: %w", qErr)
		return
	}
	defer func() { _ = rows.Close() }()

	type candidate struct {
		id               int64
		dateiname        string
		belegnummer      string
		bruttobetrag     float64
		wechselkurs      float64
		buchungJSON      string
		ausgangsrechnung bool
	}

	var candidates []candidate
	for rows.Next() {
		var c candidate
		var buchungNull sql.NullString
		var belegnummerNull sql.NullString
		var ausgangsNull sql.NullInt64
		if scanErr := rows.Scan(
			&c.id, &c.dateiname, &belegnummerNull,
			&c.bruttobetrag, &c.wechselkurs, &buchungNull, &ausgangsNull,
		); scanErr != nil {
			err = fmt.Errorf("RebookForeignToEUR scan: %w", scanErr)
			return
		}
		c.buchungJSON = buchungNull.String
		c.belegnummer = belegnummerNull.String
		c.ausgangsrechnung = ausgangsNull.Int64 != 0
		candidates = append(candidates, c)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		err = fmt.Errorf("RebookForeignToEUR iterate: %w", rowsErr)
		return
	}

	for _, c := range candidates {
		// No exchange rate → cannot convert; count and skip.
		if c.wechselkurs <= 0 {
			rateMissing++
			continue
		}

		b := core.ParseBooking(c.buchungJSON)
		if b.IsEmpty() {
			skipped++
			continue
		}

		eurGross := round2(c.bruttobetrag / c.wechselkurs)

		// Idempotency check: use the appropriate side sum to detect whether
		// the booking has already been rescaled to EUR.
		// For incoming invoices (Ausgangsrechnung=false) the "base" (payment) side
		// is the single Haben; for revenue invoices it is the single Soll.
		// We compare the base side's total against both the foreign gross and the
		// EUR gross to decide which state the booking is in.
		var currentTotal float64
		if c.ausgangsrechnung {
			currentTotal = b.SollSum()
		} else {
			currentTotal = b.HabenSum()
		}

		if math.Abs(currentTotal-eurGross) < 0.005 {
			// Already EUR-scaled.
			skipped++
			continue
		}
		if math.Abs(currentTotal-c.bruttobetrag) >= 0.005 {
			// Neither foreign nor EUR total matches — unusual state; skip to be safe.
			skipped++
			continue
		}

		// Rescale every entry by dividing Betrag / wechselkurs.
		newEntries := make([]core.BookingEntry, len(b.Entries))
		for i, e := range b.Entries {
			e.Betrag = round2(e.Betrag / c.wechselkurs)
			newEntries[i] = e
		}
		b.Entries = newEntries

		// Nudge the payment entry so Σ Soll == Σ Haben.
		// For incoming invoices the payment is the single Haben; for revenue
		// invoices it is the single Soll — use PaymentAndCounters to locate it.
		_, _, ok := b.PaymentAndCounters(c.ausgangsrechnung)
		if ok {
			// Recompute the base (payment) entry from the counter totals so the
			// booking balances exactly after rounding.
			var counterSum float64
			for _, e := range b.Entries {
				isCounter := (c.ausgangsrechnung && !e.Soll) || (!c.ausgangsrechnung && e.Soll)
				if isCounter {
					counterSum += e.Betrag
				}
			}
			counterSum = round2(counterSum)
			for i := range b.Entries {
				isBase := (c.ausgangsrechnung && b.Entries[i].Soll) || (!c.ausgangsrechnung && !b.Entries[i].Soll)
				if isBase {
					// There should be exactly one (validated by PaymentAndCounters).
					b.Entries[i].Betrag = counterSum
					break
				}
			}
		}
		// If PaymentAndCounters returns !ok the booking is irregular (multiple
		// payment entries, etc.) — the per-entry rescaling above is still an
		// improvement; leave nudging aside.

		newJSON := core.MarshalBooking(b)
		if _, updateErr := r.db.Exec(
			`UPDATE invoices SET buchung = ? WHERE id = ?`,
			newJSON, c.id,
		); updateErr != nil {
			err = fmt.Errorf("RebookForeignToEUR update id=%d: %w", c.id, updateErr)
			return
		}

		if auditErr := r.LogAudit(core.AuditEntry{
			Aktion:     "rebook-eur",
			Entitaet:   "invoice",
			Schluessel: c.belegnummer + " " + c.dateiname,
			Details:    fmt.Sprintf("rate=%.6f foreignGross=%.2f eurGross=%.2f", c.wechselkurs, c.bruttobetrag, eurGross),
		}); auditErr != nil {
			log.Printf("[WARN] audit_log rebook-eur failed: %v", auditErr)
		}

		converted++
	}

	return
}
