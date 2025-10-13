package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

// GetDocumentsDir returns the user's Documents directory.
func GetDocumentsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	// On macOS and Windows, Documents is typically in the home directory
	return filepath.Join(home, "Documents"), nil
}
