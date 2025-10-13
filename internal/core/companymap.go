package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// CompanyAccountMap stores the mapping of company names to account codes.
type CompanyAccountMap struct {
	filePath string
	mapping  map[string]int // normalized company name -> account code
}

// NewCompanyAccountMap creates a new company account map.
func NewCompanyAccountMap(configDir string) *CompanyAccountMap {
	filePath := filepath.Join(configDir, "company_accounts.json")
	return &CompanyAccountMap{
		filePath: filePath,
		mapping:  make(map[string]int),
	}
}

// Load loads the mapping from disk.
func (cam *CompanyAccountMap) Load() error {
	if _, err := os.Stat(cam.filePath); os.IsNotExist(err) {
		return nil // File doesn't exist yet, that's ok
	}

	data, err := os.ReadFile(cam.filePath)
	if err != nil {
		return fmt.Errorf("failed to read company accounts: %w", err)
	}

	if err := json.Unmarshal(data, &cam.mapping); err != nil {
		return fmt.Errorf("failed to parse company accounts: %w", err)
	}

	return nil
}

// Save saves the mapping to disk.
func (cam *CompanyAccountMap) Save() error {
	// Ensure directory exists
	dir := filepath.Dir(cam.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(cam.mapping, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal company accounts: %w", err)
	}

	if err := os.WriteFile(cam.filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write company accounts: %w", err)
	}

	return nil
}

// Get retrieves the account code for a company, if it exists.
func (cam *CompanyAccountMap) Get(companyName string) (int, bool) {
	normalized := NormalizeCompanyName(companyName)
	code, ok := cam.mapping[normalized]
	return code, ok
}

// Set stores the account code for a company.
func (cam *CompanyAccountMap) Set(companyName string, accountCode int) {
	normalized := NormalizeCompanyName(companyName)
	cam.mapping[normalized] = accountCode
}

// SuggestAccountForCompany returns the suggested account code for a company.
// If no mapping exists, returns the default account and false.
func SuggestAccountForCompany(cam *CompanyAccountMap, companyName string, defaultAccount int) (int, bool) {
	if code, ok := cam.Get(companyName); ok {
		return code, true
	}
	return defaultAccount, false
}
