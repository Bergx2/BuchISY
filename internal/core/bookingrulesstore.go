package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// BookingRulesStore loads the booking rules for one profile: a per-profile
// buchungsregeln.json overrides the bundled defaults.
type BookingRulesStore struct {
	path    string
	bundled []byte
}

// NewBookingRulesStore creates a store rooted at configDir with the bundled
// default rules as fallback.
func NewBookingRulesStore(configDir string, bundled []byte) *BookingRulesStore {
	return &BookingRulesStore{
		path:    filepath.Join(configDir, "buchungsregeln.json"),
		bundled: bundled,
	}
}

// Load returns the profile's rules (its buchungsregeln.json) if present,
// otherwise the bundled defaults.
func (s *BookingRulesStore) Load() (*BookingRules, error) {
	if data, err := os.ReadFile(s.path); err == nil {
		return ParseBookingRules(data)
	}
	return ParseBookingRules(s.bundled)
}

// Save writes the rules to the profile's buchungsregeln.json.
func (s *BookingRulesStore) Save(r *BookingRules) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(s.path, data, 0644); err != nil {
		return fmt.Errorf("failed to save booking rules: %w", err)
	}
	return nil
}
