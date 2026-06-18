package db

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/bergx2/buchisy/internal/core"
	"github.com/bergx2/buchisy/internal/logging"
)

// MigrateCSVToDatabase imports existing invoices.csv files found under
// storageRoot into the SQLite database — but ONLY when the database is still
// empty. This back-fills the database for users coming from the pre-SQLite,
// CSV-only storage (where the per-month invoices.csv files are the data), and
// is a cheap no-op once the database holds invoices. Import is idempotent:
// rows already present are skipped (see ImportFromCSV / IsDuplicate).
func (r *Repository) MigrateCSVToDatabase(storageRoot string, csvRepo *core.CSVRepository, logger *logging.Logger) (int, error) {
	existing, err := r.Count()
	if err != nil {
		return 0, err
	}
	if existing > 0 {
		// Database already populated — nothing to migrate.
		return 0, nil
	}
	if storageRoot == "" {
		return 0, nil
	}

	logger.Info("Database is empty — importing existing CSV invoices from %s", storageRoot)

	var csvFiles []string
	walkErr := filepath.Walk(storageRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip unreadable entries, keep scanning
		}
		if !info.IsDir() && filepath.Base(path) == "invoices.csv" {
			csvFiles = append(csvFiles, path)
		}
		return nil
	})
	if walkErr != nil {
		return 0, fmt.Errorf("failed to scan for CSV files: %w", walkErr)
	}

	total := 0
	for _, csvPath := range csvFiles {
		count, err := r.ImportFromCSV(csvPath, csvRepo)
		if err != nil {
			logger.Warn("Failed to import %s: %v", csvPath, err)
			continue
		}
		total += count
		logger.Info("Imported %d invoices from %s", count, filepath.Base(filepath.Dir(csvPath)))
	}

	logger.Info("CSV import complete: %d invoices imported", total)
	return total, nil
}
