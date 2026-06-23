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
// otherwise the bundled defaults. A saved file's values win, but categories /
// Vorsteuer rates introduced in a newer bundled base are merged in, so an
// existing profile still gains new booking categories without losing its
// overrides.
func (s *BookingRulesStore) Load() (*BookingRules, error) {
	bundled, berr := ParseBookingRules(s.bundled)
	if data, err := os.ReadFile(s.path); err == nil {
		if saved, perr := ParseBookingRules(data); perr == nil {
			if berr == nil {
				return mergeBundledIntoSaved(saved, bundled), nil
			}
			return saved, nil
		}
		// A corrupt profile file must not break all auto-bookings — fall back
		// to the bundled defaults rather than returning empty rules.
	}
	return bundled, berr
}

// mergeBundledIntoSaved keeps every value from saved and fills in only what the
// bundled base adds: Vorsteuer rates, Umsatzsteuer rates, and category rules the saved file lacks.
func mergeBundledIntoSaved(saved, bundled *BookingRules) *BookingRules {
	if saved.VorsteuerKonten == nil {
		saved.VorsteuerKonten = map[string]int{}
	}
	for satz, konto := range bundled.VorsteuerKonten {
		if _, ok := saved.VorsteuerKonten[satz]; !ok {
			saved.VorsteuerKonten[satz] = konto
		}
	}
	if saved.UmsatzsteuerKonten == nil {
		saved.UmsatzsteuerKonten = map[string]int{}
	}
	for k, v := range bundled.UmsatzsteuerKonten {
		if _, ok := saved.UmsatzsteuerKonten[k]; !ok {
			saved.UmsatzsteuerKonten[k] = v
		}
	}
	have := map[string]bool{}
	for _, r := range saved.Regeln {
		have[r.Kategorie] = true
	}
	for _, r := range bundled.Regeln {
		if !have[r.Kategorie] {
			saved.Regeln = append(saved.Regeln, r)
		}
	}
	return saved
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
