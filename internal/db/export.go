package db

import (
	"fmt"

	"github.com/bergx2/buchisy/internal/core"
)

// ExportToCSV exports all invoices for a specific month to a CSV file.
// This function uses the CSVRepository to write the file with all configured settings
// (separator, encoding, decimal separator, quotes, etc.)
func (r *Repository) ExportToCSV(jahr, monat, csvPath string, csvRepo *core.CSVRepository) error {
	// Get all invoices for this month from database
	rows, err := r.List(jahr, monat)
	if err != nil {
		return fmt.Errorf("failed to list invoices from database: %w", err)
	}

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
