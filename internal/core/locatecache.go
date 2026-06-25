package core

import (
	"encoding/json"
	"fmt"
	"os"
)

// LocateCacheEntry records the result of a Claude Vision amount-location lookup.
// Found=false means the query was made but the amount was not located; this is
// cached too so a miss is never re-queried.
type LocateCacheEntry struct {
	Found bool    `json:"found"`
	Page  int     `json:"page"`
	X0    float64 `json:"x0"`
	Y0    float64 `json:"y0"`
	X1    float64 `json:"x1"`
	Y1    float64 `json:"y1"`
}

// LocateCache is a sidecar JSON cache mapping composite keys to location results.
// Key format is produced by LocateCacheKey.
type LocateCache map[string]LocateCacheEntry

// LocateCacheKey returns the canonical map key for a (relpath, value) pair.
func LocateCacheKey(relpath, value string) string {
	return relpath + "|" + value
}

// LoadLocateCache reads a LocateCache from the JSON file at path.
// If the file does not exist, an empty cache and nil error are returned.
func LoadLocateCache(path string) (LocateCache, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(LocateCache), nil
		}
		return nil, fmt.Errorf("failed to read locate cache: %w", err)
	}

	var c LocateCache
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("failed to parse locate cache: %w", err)
	}

	return c, nil
}

// SaveLocateCache writes the LocateCache to the JSON file at path.
// Parent directories are created if they do not exist.
func SaveLocateCache(path string, c LocateCache) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal locate cache: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write locate cache: %w", err)
	}

	return nil
}
