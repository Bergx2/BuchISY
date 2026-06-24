package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// BookingTemplate is the remembered booking pattern for a company: which
// category to use and (for "standard") which expense account.
type BookingTemplate struct {
	Kategorie    string `json:"kategorie"`
	ExpenseKonto int    `json:"expense_konto"`
	Autobook     bool   `json:"autobook,omitempty"`
}

// BookingTemplateStore persists company→BookingTemplate per profile.
type BookingTemplateStore struct {
	path      string
	templates map[string]BookingTemplate
}

// NewBookingTemplateStore creates a store rooted at configDir.
func NewBookingTemplateStore(configDir string) *BookingTemplateStore {
	return &BookingTemplateStore{
		path:      filepath.Join(configDir, "booking_templates.json"),
		templates: map[string]BookingTemplate{},
	}
}

// Load reads the persisted templates (a missing file is not an error).
func (s *BookingTemplateStore) Load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return nil // no file yet
	}
	m := map[string]BookingTemplate{}
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("failed to parse booking templates: %w", err)
	}
	s.templates = m
	return nil
}

// Get returns the template remembered for company.
func (s *BookingTemplateStore) Get(company string) (BookingTemplate, bool) {
	t, ok := s.templates[company]
	return t, ok
}

// List returns a snapshot of all stored company→BookingTemplate pairs,
// sorted by company name.
func (s *BookingTemplateStore) List() []struct {
	Company string
	Tpl     BookingTemplate
} {
	out := make([]struct {
		Company string
		Tpl     BookingTemplate
	}, 0, len(s.templates))
	for company, tpl := range s.templates {
		out = append(out, struct {
			Company string
			Tpl     BookingTemplate
		}{Company: company, Tpl: tpl})
	}
	// Stable sort by company name.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Company < out[j-1].Company; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// Set remembers and persists a template for company.
func (s *BookingTemplateStore) Set(company string, t BookingTemplate) error {
	s.templates[company] = t
	data, err := json.MarshalIndent(s.templates, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(s.path, data, 0644); err != nil {
		return fmt.Errorf("failed to save booking templates: %w", err)
	}
	return nil
}
