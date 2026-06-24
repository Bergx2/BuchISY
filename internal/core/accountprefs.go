package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// AccountPrefs stores per-profile account preferences: recently-used and favorite accounts.
type AccountPrefs struct {
	filePath  string
	Recent    []int `json:"recent"`
	Favorites []int `json:"favorites"`
}

// NewAccountPrefs creates a new account preferences store.
func NewAccountPrefs(configDir string) *AccountPrefs {
	filePath := filepath.Join(configDir, "account_prefs.json")
	return &AccountPrefs{
		filePath:  filePath,
		Recent:    make([]int, 0),
		Favorites: make([]int, 0),
	}
}

// Load loads the preferences from disk.
func (ap *AccountPrefs) Load() error {
	if _, err := os.Stat(ap.filePath); os.IsNotExist(err) {
		return nil // File doesn't exist yet, that's ok
	}

	data, err := os.ReadFile(ap.filePath)
	if err != nil {
		return fmt.Errorf("failed to read account preferences: %w", err)
	}

	if err := json.Unmarshal(data, ap); err != nil {
		return fmt.Errorf("failed to parse account preferences: %w", err)
	}

	return nil
}

// Save saves the preferences to disk.
func (ap *AccountPrefs) Save() error {
	// Ensure directory exists
	dir := filepath.Dir(ap.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(ap, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal account preferences: %w", err)
	}

	if err := os.WriteFile(ap.filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write account preferences: %w", err)
	}

	return nil
}

// RecordUse records the use of an account (prepend, dedupe, cap at 8).
func (ap *AccountPrefs) RecordUse(konto int) {
	// Remove if exists (to avoid duplicates)
	newRecent := make([]int, 0, len(ap.Recent)+1)
	for _, k := range ap.Recent {
		if k != konto {
			newRecent = append(newRecent, k)
		}
	}

	// Prepend the most recent
	ap.Recent = append([]int{konto}, newRecent...)

	// Cap at 8
	if len(ap.Recent) > 8 {
		ap.Recent = ap.Recent[:8]
	}
}

// IsFavorite checks if an account is marked as favorite.
func (ap *AccountPrefs) IsFavorite(konto int) bool {
	for _, k := range ap.Favorites {
		if k == konto {
			return true
		}
	}
	return false
}

// ToggleFavorite toggles the favorite status of an account.
func (ap *AccountPrefs) ToggleFavorite(konto int) {
	if ap.IsFavorite(konto) {
		// Remove from favorites
		newFavs := make([]int, 0, len(ap.Favorites)-1)
		for _, k := range ap.Favorites {
			if k != konto {
				newFavs = append(newFavs, k)
			}
		}
		ap.Favorites = newFavs
	} else {
		// Add to favorites
		ap.Favorites = append(ap.Favorites, konto)
	}
}

// RecentList returns a copy of the recent list.
func (ap *AccountPrefs) RecentList() []int {
	result := make([]int, len(ap.Recent))
	copy(result, ap.Recent)
	return result
}

// FavoriteList returns a copy of the favorites list.
func (ap *AccountPrefs) FavoriteList() []int {
	result := make([]int, len(ap.Favorites))
	copy(result, ap.Favorites)
	return result
}
