package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// StatementAliasStore persists per-supplier statement-text tokens to disk so that
// cryptic bank-statement descriptions can be matched to the right supplier after
// the user (or the auto-linker) confirms the link at least once.
//
// File: <configDir>/statement_aliases.json
// Schema: map[lowercase_supplier][]token
type StatementAliasStore struct {
	filePath string
	aliases  map[string][]string // lowercase supplier → deduped learned tokens
}

// NewStatementAliasStore creates a store backed by statement_aliases.json in configDir.
func NewStatementAliasStore(configDir string) *StatementAliasStore {
	return &StatementAliasStore{
		filePath: filepath.Join(configDir, "statement_aliases.json"),
		aliases:  make(map[string][]string),
	}
}

// Load reads the JSON file into the in-memory map and returns a copy of the map.
// If the file does not exist, Load returns the current in-memory map (which may
// already contain tokens learned in this session) and no error.
func (s *StatementAliasStore) Load() (map[string][]string, error) {
	if _, err := os.Stat(s.filePath); !os.IsNotExist(err) {
		data, err := os.ReadFile(s.filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read statement aliases: %w", err)
		}
		if err := json.Unmarshal(data, &s.aliases); err != nil {
			return nil, fmt.Errorf("failed to parse statement aliases: %w", err)
		}
	}

	// Return a copy so callers cannot mutate internal state.
	out := make(map[string][]string, len(s.aliases))
	for k, v := range s.aliases {
		cp := make([]string, len(v))
		copy(cp, v)
		out[k] = cp
	}
	return out, nil
}

// Learn extracts distinctive tokens from lineText and adds them to the stored
// alias set for the given supplier. It is a no-op when lineText yields no new tokens.
//
// Rules:
//   - key = strings.ToLower(strings.TrimSpace(supplier))
//   - tokens = tokenize(lineText)  (reuses the matcher's tokenizer, len ≥ 3)
//   - keep only tokens where len(tok) >= 4, not pure digits, and NOT already present
//     in the supplier's own name tokens (to avoid circular matches)
//   - union into stored slice (dedupe)
func (s *StatementAliasStore) Learn(supplier, lineText string) {
	key := strings.ToLower(strings.TrimSpace(supplier))
	if key == "" {
		return
	}

	// Tokens from the supplier name itself — these are redundant to store.
	supplierTokenSet := make(map[string]bool)
	for _, t := range tokenize(supplier) {
		supplierTokenSet[t] = true
	}

	// Tokens already stored for this supplier.
	existing := make(map[string]bool)
	for _, t := range s.aliases[key] {
		existing[t] = true
	}

	for _, tok := range tokenize(lineText) {
		if len(tok) < 4 {
			continue
		}
		if isPureDigits(tok) {
			continue
		}
		if supplierTokenSet[tok] {
			continue
		}
		if existing[tok] {
			continue
		}
		s.aliases[key] = append(s.aliases[key], tok)
		existing[tok] = true
	}
}

// Save writes the in-memory alias map to disk as indented JSON (mode 0644).
func (s *StatementAliasStore) Save() error {
	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(s.aliases, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal statement aliases: %w", err)
	}

	if err := os.WriteFile(s.filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write statement aliases: %w", err)
	}

	return nil
}

// isPureDigits returns true when every rune in s is an ASCII digit.
func isPureDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
