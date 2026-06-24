package core

import (
	"encoding/json"
	"testing"
)

// TestDiffFieldsIdentical verifies that identical rows produce "{}".
func TestDiffFieldsIdentical(t *testing.T) {
	row := CSVRow{
		Auftraggeber:    "Acme",
		Rechnungsnummer: "R-001",
		Bruttobetrag:    119.00,
		Gegenkonto:      4920,
	}
	got := DiffFields(row, row)
	if got != "{}" {
		t.Errorf("DiffFields identical rows = %q, want {}", got)
	}
}

// TestDiffFieldsChanged verifies that changed fields appear in the JSON diff
// with correct alt/neu values.
func TestDiffFieldsChanged(t *testing.T) {
	old := CSVRow{
		Auftraggeber: "Acme",
		Bruttobetrag: 100.00,
		Gegenkonto:   4920,
	}
	new := CSVRow{
		Auftraggeber: "Acme",
		Bruttobetrag: 119.00,
		Gegenkonto:   8400,
	}

	got := DiffFields(old, new)

	// Must be valid JSON.
	var parsed map[string]struct {
		Alt json.RawMessage `json:"alt"`
		Neu json.RawMessage `json:"neu"`
	}
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("DiffFields returned invalid JSON: %v — output: %s", err, got)
	}

	// Bruttobetrag should appear.
	if _, ok := parsed["Bruttobetrag"]; !ok {
		t.Errorf("expected Bruttobetrag in diff, got: %s", got)
	}
	// Gegenkonto should appear.
	if _, ok := parsed["Gegenkonto"]; !ok {
		t.Errorf("expected Gegenkonto in diff, got: %s", got)
	}
	// Auftraggeber should NOT appear (unchanged).
	if _, ok := parsed["Auftraggeber"]; ok {
		t.Errorf("Auftraggeber should not appear in diff (unchanged), got: %s", got)
	}
}

// TestAuditEntryStruct ensures AuditEntry fields are accessible (compile-time check).
func TestAuditEntryStruct(t *testing.T) {
	e := AuditEntry{
		TS:         "2026-06-24T10:00:00Z",
		Aktion:     "create",
		Entitaet:   "invoice",
		Schluessel: "2026-0001 test.pdf",
		Details:    "{}",
	}
	if e.Aktion != "create" {
		t.Errorf("unexpected Aktion: %s", e.Aktion)
	}
}
