package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// StatementMetadata holds the per-statement metadata the user can edit:
// period, statement number, opening/closing balance, reviewed flag and
// a free-text note. Stored side-by-side with the actual PDF file as a
// per-account metadata.json keyed by relative file path.
type StatementMetadata struct {
	DateFrom       string  `json:"date_from"`       // DD.MM.YYYY
	DateTo         string  `json:"date_to"`         // DD.MM.YYYY
	Number         string  `json:"number"`          // e.g. "5/2026"
	OpeningBalance float64 `json:"opening_balance"` // start of period
	ClosingBalance float64 `json:"closing_balance"` // end of period
	Reviewed       bool    `json:"reviewed"`        // checked off
	Note           string  `json:"note"`            // free-text

	// BookingsParsedMtime is the source PDF's mtime (Unix seconds) at
	// the time Bookings was last refreshed. Used as a cheap cache key
	// so the parser only re-runs when the file changes.
	BookingsParsedMtime int64              `json:"bookings_parsed_mtime,omitempty"`
	Bookings            []StatementBooking `json:"bookings,omitempty"`
	// BookingsParserVersion is the StatementParserVersion that produced Bookings.
	// A mismatch forces a re-parse, so improving the parser automatically
	// refreshes already-cached statements without any manual cache clearing.
	BookingsParserVersion int `json:"bookings_parser_version,omitempty"`
}

// StatementMetadataMap maps a statement's relative path (within its
// account folder, e.g. "2026/Auszug-05.pdf") to its metadata.
type StatementMetadataMap map[string]StatementMetadata

// StatementMetaPath returns the metadata.json path for the given
// account-root folder.
func StatementMetaPath(accountFolder string) string {
	return filepath.Join(accountFolder, "metadata.json")
}

// LoadStatementMeta reads the metadata.json from the given account
// folder. Returns an empty (non-nil) map when the file doesn't exist.
func LoadStatementMeta(accountFolder string) (StatementMetadataMap, error) {
	path := StatementMetaPath(accountFolder)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return StatementMetadataMap{}, nil
		}
		return nil, fmt.Errorf("read statement metadata: %w", err)
	}
	var m StatementMetadataMap
	if err := json.Unmarshal(data, &m); err != nil {
		return StatementMetadataMap{}, fmt.Errorf("parse statement metadata: %w", err)
	}
	if m == nil {
		m = StatementMetadataMap{}
	}
	return m, nil
}

// SaveStatementMeta writes the metadata.json for an account folder.
// Creates the folder if missing.
func SaveStatementMeta(accountFolder string, m StatementMetadataMap) error {
	if err := os.MkdirAll(accountFolder, 0755); err != nil {
		return fmt.Errorf("create account folder: %w", err)
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal statement metadata: %w", err)
	}
	return os.WriteFile(StatementMetaPath(accountFolder), data, 0644)
}
