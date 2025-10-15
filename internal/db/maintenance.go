package db

import (
	"fmt"
	"os"
	"path/filepath"
)

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
