package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLocateCacheKey_Stable(t *testing.T) {
	key := LocateCacheKey("2026/x.pdf", "573.15")
	if key != "2026/x.pdf|573.15" {
		t.Errorf("unexpected key: %q", key)
	}
	// Calling again must return the same value.
	if LocateCacheKey("2026/x.pdf", "573.15") != key {
		t.Error("LocateCacheKey is not stable")
	}
}

func TestLoadLocateCache_MissingFile_ReturnsEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "nonexistent_locate_cache.json")

	cache, err := LoadLocateCache(path)
	if err != nil {
		t.Fatalf("expected nil error for missing file, got: %v", err)
	}
	if cache == nil {
		t.Fatal("expected non-nil empty map")
	}
	if len(cache) != 0 {
		t.Fatalf("expected empty map, got %d entries", len(cache))
	}
}

func TestSaveAndLoadLocateCache_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "locate_cache.json")

	entry := LocateCacheEntry{
		Found: true,
		Page:  0,
		X0:    0.1,
		Y0:    0.2,
		X1:    0.4,
		Y1:    0.5,
	}
	key := LocateCacheKey("2026/invoice.pdf", "573.15")

	original := LocateCache{key: entry}

	if err := SaveLocateCache(path, original); err != nil {
		t.Fatalf("SaveLocateCache failed: %v", err)
	}

	// Verify file was written.
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file to exist after save: %v", err)
	}

	loaded, err := LoadLocateCache(path)
	if err != nil {
		t.Fatalf("LoadLocateCache failed: %v", err)
	}

	got, ok := loaded[key]
	if !ok {
		t.Fatalf("key %q not found in loaded cache", key)
	}
	if got != entry {
		t.Errorf("loaded entry mismatch: got %+v, want %+v", got, entry)
	}
}

func TestLoadLocateCache_NotFoundEntry(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "locate_cache.json")

	key := LocateCacheKey("2026/scan.pdf", "99.00")
	notFound := LocateCacheEntry{Found: false}

	original := LocateCache{key: notFound}
	if err := SaveLocateCache(path, original); err != nil {
		t.Fatalf("SaveLocateCache failed: %v", err)
	}

	loaded, err := LoadLocateCache(path)
	if err != nil {
		t.Fatalf("LoadLocateCache failed: %v", err)
	}

	got, ok := loaded[key]
	if !ok {
		t.Fatalf("key %q not found in loaded cache", key)
	}
	if got.Found {
		t.Error("expected Found=false for cached miss entry")
	}
}
