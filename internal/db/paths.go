package db

import (
	"fmt"
	"path/filepath"
)

// GetDBPath returns the path to the SQLite database file for a specific month.
// Format: <storageRoot>/YYYY-MM/invoices.db
func GetDBPath(storageRoot string, jahr int, monat int) string {
	folderName := fmt.Sprintf("%04d-%02d", jahr, monat)
	return filepath.Join(storageRoot, folderName, "invoices.db")
}

// GetGlobalDBPath returns the path to a global SQLite database (alternative approach).
// This would store all invoices in a single database instead of per-month databases.
// Format: <configDir>/invoices.db
func GetGlobalDBPath(configDir string) string {
	return filepath.Join(configDir, "invoices.db")
}
