package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SettingsManager handles loading and saving application settings.
type SettingsManager struct {
	configPath string
}

// NewSettingsManager creates a new settings manager.
// configPath should be the full path to the settings.json file.
func NewSettingsManager(configPath string) *SettingsManager {
	return &SettingsManager{
		configPath: configPath,
	}
}

// Load loads settings from disk. If the file doesn't exist, returns default settings.
func (sm *SettingsManager) Load() (Settings, error) {
	// If file doesn't exist, return defaults
	if _, err := os.Stat(sm.configPath); os.IsNotExist(err) {
		return DefaultSettings(), nil
	}

	data, err := os.ReadFile(sm.configPath)
	if err != nil {
		return DefaultSettings(), fmt.Errorf("failed to read settings: %w", err)
	}

	var settings Settings
	if err := json.Unmarshal(data, &settings); err != nil {
		return DefaultSettings(), fmt.Errorf("failed to parse settings: %w", err)
	}

	settings.BankAccounts = normalizeBankAccounts(settings.BankAccounts)
	return settings, nil
}

// Save saves settings to disk.
func (sm *SettingsManager) Save(settings Settings) error {
	// Ensure directory exists
	dir := filepath.Dir(sm.configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal settings: %w", err)
	}

	if err := os.WriteFile(sm.configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write settings: %w", err)
	}

	return nil
}

// GetConfigDir returns the OS-specific application data directory.
// macOS: ~/Library/Application Support/BuchISY
// Windows: %APPDATA%/BuchISY
func GetConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	var configDir string
	switch {
	case os.Getenv("APPDATA") != "": // Windows
		configDir = filepath.Join(os.Getenv("APPDATA"), "BuchISY")
	default: // macOS/Linux
		configDir = filepath.Join(home, "Library", "Application Support", "BuchISY")
	}

	return configDir, nil
}

// GetProfileConfigDir returns the config directory for a named profile,
// i.e. <AppData>/BuchISY/profiles/<profile>.
func GetProfileConfigDir(profile string) (string, error) {
	root, err := GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "profiles", profile), nil
}

// ListProfiles returns the names of all existing profiles (the directory
// names under <AppData>/BuchISY/profiles). A missing profiles directory
// yields an empty list, not an error.
//
// Automatic backup snapshots (directories named "<profile>.backup-<timestamp>",
// e.g. left behind by a config migration) are NOT real company profiles and are
// excluded so they don't clutter the profile picker. The backup directories
// stay on disk as a safety net — they are just hidden from the selectable list.
func ListProfiles() ([]string, error) {
	root, err := GetConfigDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(root, "profiles"))
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	names := []string{}
	for _, e := range entries {
		if e.IsDir() && !isBackupProfileDir(e.Name()) {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

// isBackupProfileDir reports whether a profiles/ subdirectory is an automatic
// backup snapshot rather than a real profile. Convention: the name contains
// ".backup-" (followed by a timestamp), e.g. "Bergx2 GmbH.backup-20260623-080533".
func isBackupProfileDir(name string) bool {
	return strings.Contains(name, ".backup-")
}

// GetDocumentsDir returns the user's Documents directory.
func GetDocumentsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	// On macOS and Windows, Documents is typically in the home directory
	return filepath.Join(home, "Documents"), nil
}

// normalizeBankAccounts assigns a valid AccountType to every account,
// migrating the legacy IsCreditCard flag. An empty or unknown type
// becomes "creditcard" when the legacy flag is set, otherwise "bank".
func normalizeBankAccounts(accounts []BankAccount) []BankAccount {
	for i := range accounts {
		switch accounts[i].AccountType {
		case AccountTypeBank, AccountTypeCreditCard, AccountTypeCash, AccountTypePayroll:
			// already a valid type
		default:
			if accounts[i].IsCreditCard {
				accounts[i].AccountType = AccountTypeCreditCard
			} else {
				accounts[i].AccountType = AccountTypeBank
			}
		}
		accounts[i].IsCreditCard = false
	}
	return accounts
}
