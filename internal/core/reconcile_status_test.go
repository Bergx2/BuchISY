package core

import "testing"

func TestReconcileSummary_basic(t *testing.T) {
	lines := []LineRef{
		{Key: "key0", Betrag: 100.0, IstGutschrift: false}, // debit, linked
		{Key: "key1", Betrag: 50.0, IstGutschrift: false},  // debit, open
		{Key: "key2", Betrag: 200.0, IstGutschrift: true},  // credit, linked
		{Key: "key3", Betrag: 75.0, IstGutschrift: true},   // credit, open
	}
	linked := map[string]bool{
		"key0": true,
		"key2": true,
	}

	result := ReconcileSummary(lines, linked)

	if result.LinesTotal != 4 {
		t.Errorf("LinesTotal: got %d, want 4", result.LinesTotal)
	}
	if result.LinesMatched != 2 {
		t.Errorf("LinesMatched: got %d, want 2", result.LinesMatched)
	}
	if result.LinesOpen != 2 {
		t.Errorf("LinesOpen: got %d, want 2", result.LinesOpen)
	}
	if result.OpenBelastung != 50.0 {
		t.Errorf("OpenBelastung: got %f, want 50.0", result.OpenBelastung)
	}
	if result.OpenGutschrift != 75.0 {
		t.Errorf("OpenGutschrift: got %f, want 75.0", result.OpenGutschrift)
	}
}

func TestReconcileSummary_empty(t *testing.T) {
	result := ReconcileSummary(nil, nil)

	if result.LinesTotal != 0 {
		t.Errorf("LinesTotal: got %d, want 0", result.LinesTotal)
	}
	if result.LinesMatched != 0 {
		t.Errorf("LinesMatched: got %d, want 0", result.LinesMatched)
	}
	if result.LinesOpen != 0 {
		t.Errorf("LinesOpen: got %d, want 0", result.LinesOpen)
	}
	if result.OpenBelastung != 0 {
		t.Errorf("OpenBelastung: got %f, want 0", result.OpenBelastung)
	}
	if result.OpenGutschrift != 0 {
		t.Errorf("OpenGutschrift: got %f, want 0", result.OpenGutschrift)
	}
}
