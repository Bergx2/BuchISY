package db

import (
	"fmt"

	"github.com/bergx2/buchisy/internal/core"
)

// ExportToCSV exports all invoices for a specific month to a CSV file.
// This function uses the CSVRepository to write the file with all configured settings
// (separator, encoding, decimal separator, quotes, etc.)
//
// Main amount columns (BetragNetto, Steuersatz_Betrag, Bruttobetrag, BetragNetto_EUR)
// are always written in EUR. For foreign-currency rows the original currency code and
// gross amount are preserved in the documentation columns Originalwaehrung and
// Originalbetrag_Brutto so the source data is not lost.
func (r *Repository) ExportToCSV(jahr, monat, csvPath string, csvRepo *core.CSVRepository) error {
	// Get all invoices for this month from database
	rows, err := r.List(jahr, monat)
	if err != nil {
		return fmt.Errorf("failed to list invoices from database: %w", err)
	}

	// Stamp documentation columns BEFORE EUR normalisation so the original
	// foreign-currency data is preserved in the export even after conversion.
	for i, row := range rows {
		if row.Waehrung != "" && row.Waehrung != "EUR" {
			rows[i].Originalwaehrung = row.Waehrung
			rows[i].Originalbetrag_Brutto = row.Bruttobetrag
		}
	}

	// Normalise all money fields to EUR (foreign rows divided by Wechselkurs).
	// EUR rows and rows with missing rates are returned unchanged.
	rows = core.RowsEUR(rows)

	// Rewrite the CSV file with all rows
	if err := csvRepo.Rewrite(csvPath, rows); err != nil {
		return fmt.Errorf("failed to write CSV file: %w", err)
	}

	return nil
}

// ImportFromCSV imports invoices from a CSV file into the database.
// This is used for migrating existing CSV files to SQLite.
func (r *Repository) ImportFromCSV(csvPath string, csvRepo *core.CSVRepository) (int, error) {
	// Load existing CSV
	rows, err := csvRepo.Load(csvPath)
	if err != nil {
		return 0, fmt.Errorf("failed to load CSV: %w", err)
	}

	// Insert all rows into database
	count := 0
	for _, row := range rows {
		// Check if already exists (avoid duplicates during import)
		exists, err := r.IsDuplicate(row.Jahr, row.Monat, row)
		if err != nil {
			// Log but continue
			continue
		}
		if exists {
			// Skip duplicates
			continue
		}

		_, err = r.Insert(row)
		if err != nil {
			// Log error but continue with other rows
			continue
		}
		count++
	}

	return count, nil
}
