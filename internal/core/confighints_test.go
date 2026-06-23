package core

import "testing"

func TestMissingConfigHints(t *testing.T) {
	has := func(hs []string, k string) bool {
		for _, h := range hs {
			if h == k {
				return true
			}
		}
		return false
	}
	// Claude mode without an API key → warn.
	s := Settings{ProcessingMode: "claude", StorageRoot: "/x"}
	if !has(MissingConfigHints(s, false), "hint.no_api_key") {
		t.Error("expected no_api_key hint")
	}
	if has(MissingConfigHints(s, true), "hint.no_api_key") {
		t.Error("must not warn when API key present")
	}
	// Local mode never needs an API key.
	if has(MissingConfigHints(Settings{ProcessingMode: "local", StorageRoot: "/x"}, false), "hint.no_api_key") {
		t.Error("local mode must not warn about API key")
	}
	// Missing storage root → warn.
	if !has(MissingConfigHints(Settings{ProcessingMode: "local"}, false), "hint.no_storage") {
		t.Error("expected no_storage hint")
	}
	// Fully configured → no hints.
	if h := MissingConfigHints(Settings{ProcessingMode: "local", StorageRoot: "/x"}, true); len(h) != 0 {
		t.Errorf("expected no hints, got %v", h)
	}
}
