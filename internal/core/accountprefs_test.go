package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAccountPrefsRecordUse(t *testing.T) {
	tmp := t.TempDir()
	ap := NewAccountPrefs(tmp)

	// Record use: 4663, 4920, 4663 (duplicate)
	ap.RecordUse(4663)
	ap.RecordUse(4920)
	ap.RecordUse(4663) // duplicate should move to front

	recent := ap.RecentList()
	if len(recent) != 2 {
		t.Errorf("RecentList length = %d, want 2", len(recent))
	}
	if recent[0] != 4663 || recent[1] != 4920 {
		t.Errorf("RecentList = %v, want [4663 4920]", recent)
	}
}

func TestAccountPrefsCap(t *testing.T) {
	tmp := t.TempDir()
	ap := NewAccountPrefs(tmp)

	// Record 9 distinct accounts
	for i := 1; i <= 9; i++ {
		ap.RecordUse(4000 + i)
	}

	recent := ap.RecentList()
	if len(recent) != 8 {
		t.Errorf("RecentList capped at 8, got %d", len(recent))
	}
	// Most recent should be 4009, oldest (4001) should be dropped
	if recent[0] != 4009 {
		t.Errorf("RecentList[0] = %d, want 4009", recent[0])
	}
	if recent[7] != 4002 {
		t.Errorf("RecentList[7] = %d, want 4002", recent[7])
	}
}

func TestAccountPrefsToggleFavorite(t *testing.T) {
	tmp := t.TempDir()
	ap := NewAccountPrefs(tmp)

	// Initially not favorite
	if ap.IsFavorite(8400) {
		t.Error("8400 should not be favorite initially")
	}

	// Toggle on
	ap.ToggleFavorite(8400)
	if !ap.IsFavorite(8400) {
		t.Error("8400 should be favorite after toggle")
	}

	// Toggle off
	ap.ToggleFavorite(8400)
	if ap.IsFavorite(8400) {
		t.Error("8400 should not be favorite after second toggle")
	}
}

func TestAccountPrefsPersistence(t *testing.T) {
	tmp := t.TempDir()

	// Create and save
	ap1 := NewAccountPrefs(tmp)
	ap1.RecordUse(4663)
	ap1.RecordUse(4920)
	ap1.ToggleFavorite(8400)
	ap1.ToggleFavorite(8341)

	if err := ap1.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Load fresh
	ap2 := NewAccountPrefs(tmp)
	if err := ap2.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Verify recent
	recent := ap2.RecentList()
	if len(recent) != 2 {
		t.Errorf("loaded RecentList length = %d, want 2", len(recent))
	}
	if recent[0] != 4920 || recent[1] != 4663 {
		t.Errorf("loaded RecentList = %v, want [4920 4663]", recent)
	}

	// Verify favorites
	if !ap2.IsFavorite(8400) {
		t.Error("loaded: 8400 should be favorite")
	}
	if !ap2.IsFavorite(8341) {
		t.Error("loaded: 8341 should be favorite")
	}
}

func TestAccountPrefsFileCreation(t *testing.T) {
	tmp := t.TempDir()
	ap := NewAccountPrefs(tmp)
	ap.RecordUse(4663)
	ap.ToggleFavorite(8400)

	if err := ap.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// File should exist
	filePath := filepath.Join(tmp, "account_prefs.json")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Errorf("account_prefs.json not created at %s", filePath)
	}
}

func TestAccountPrefsLoadNonexistent(t *testing.T) {
	tmp := t.TempDir()
	ap := NewAccountPrefs(tmp)

	// Load from nonexistent file should not error
	if err := ap.Load(); err != nil {
		t.Errorf("Load from nonexistent file should not error, got %v", err)
	}

	// Should have empty lists
	if len(ap.RecentList()) != 0 {
		t.Error("RecentList should be empty")
	}
	if len(ap.FavoriteList()) != 0 {
		t.Error("FavoriteList should be empty")
	}
}
