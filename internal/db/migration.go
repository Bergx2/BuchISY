package db

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/bergx2/buchisy/internal/core"
	"github.com/bergx2/buchisy/internal/logging"
)

// MigrateCSVToDatabase imports all existing CSV files into the SQLite database.
// This is run once on first startup to migrate from CSV-based storage to database.
func (r *Repository) MigrateCSVToDatabase(storageRoot string, csvRepo *core.CSVRepository, logger *logging.Logger) (int, error) {
	logger.Info("Starting CSV to SQLite migration...")

	totalImported := 0

	// Check if migration marker exists
	markerPath := filepath.Join(filepath.Dir(r.getDBPath()), ".migrated")
	if _, err := os.Stat(markerPath); err == nil {
		logger.Info("Migration already completed (marker file exists)")
		return 0, nil
	}

	// Find all CSV files in storage root
	var csvFiles []string
	err := filepath.Walk(storageRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && filepath.Base(path) == "invoices.csv" {
			csvFiles = append(csvFiles, path)
		}
		return nil
	})

	if err != nil {
		return 0, fmt.Errorf("failed to scan for CSV files: %w", err)
	}

	logger.Info("Found %d CSV files to migrate", len(csvFiles))

	// Import each CSV file
	for _, csvPath := range csvFiles {
		count, err := r.ImportFromCSV(csvPath, csvRepo)
		if err != nil {
			logger.Warn("Failed to import %s: %v", csvPath, err)
			continue
		}
		totalImported += count
		logger.Info("Imported %d invoices from %s", count, filepath.Base(csvPath))
	}

	// Create migration marker file
	if err := os.WriteFile(markerPath, []byte("migrated"), 0644); err != nil {
		logger.Warn("Failed to create migration marker: %v", err)
	}

	logger.Info("Migration complete! Imported %d total invoices", totalImported)

	return totalImported, nil
}

// getDBPath returns the database file path.
func (r *Repository) getDBPath() string {
	return r.dbPath
}
