package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ChartStore loads the chart of accounts (bundled seed merged with a saved
// per-profile import) and persists imports.
type ChartStore struct {
	importPath string
	bundled    []byte
}

// NewChartStore creates a store rooted at configDir (the active profile dir),
// using bundled as the embedded starter chart.
func NewChartStore(configDir string, bundled []byte) *ChartStore {
	return &ChartStore{
		importPath: filepath.Join(configDir, "chart_skr04.json"),
		bundled:    bundled,
	}
}

// Load returns the merged chart: bundled accounts first, then the saved import
// (so imported entries override/extend bundled ones).
func (s *ChartStore) Load() (*ChartOfAccounts, error) {
	bundled, err := ParseChartJSON(s.bundled)
	if err != nil {
		return nil, err
	}
	imported := []SKRAccount{}
	if data, err := os.ReadFile(s.importPath); err == nil {
		imported, err = ParseChartJSON(data)
		if err != nil {
			return nil, err
		}
	}
	return NewChartOfAccounts(append(bundled, imported...)), nil
}

// SaveImport persists the imported accounts as the per-profile chart override.
func (s *ChartStore) SaveImport(accs []SKRAccount) error {
	data, err := json.MarshalIndent(accs, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(s.importPath, data, 0644); err != nil {
		return fmt.Errorf("failed to save chart import: %w", err)
	}
	return nil
}
