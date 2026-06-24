package db

import (
	"strings"
	"testing"

	"github.com/bergx2/buchisy/internal/core"
)

// TestAuditLogCreate verifies that Insert produces a "create" audit entry.
func TestAuditLogCreate(t *testing.T) {
	repo := newTestRepo(t)

	row := core.CSVRow{
		Dateiname:    "invoice-create.pdf",
		Jahr:         "2026",
		Monat:        "06",
		Auftraggeber: "Acme GmbH",
		Bruttobetrag: 119.0,
		Belegnummer:  "2026-0001",
	}
	if _, err := repo.Insert(row); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	entries, err := repo.AuditLog(10)
	if err != nil {
		t.Fatalf("AuditLog: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one audit entry after Insert, got none")
	}

	// Find a create entry for our dateiname.
	var found bool
	for _, e := range entries {
		if e.Aktion == "create" && strings.Contains(e.Schluessel, "invoice-create.pdf") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("no 'create' audit entry found for invoice-create.pdf; entries: %+v", entries)
	}
}

// TestAuditLogUpdate verifies that Update produces an "update" audit entry
// whose Details mention the changed field (Bruttobetrag).
func TestAuditLogUpdate(t *testing.T) {
	repo := newTestRepo(t)

	orig := core.CSVRow{
		Dateiname:    "invoice-update.pdf",
		Jahr:         "2026",
		Monat:        "06",
		Auftraggeber: "Beta AG",
		Bruttobetrag: 100.0,
	}
	if _, err := repo.Insert(orig); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	updated := orig
	updated.Bruttobetrag = 200.0

	if err := repo.Update("2026", "06", "invoice-update.pdf", updated); err != nil {
		t.Fatalf("Update: %v", err)
	}

	entries, err := repo.AuditLog(10)
	if err != nil {
		t.Fatalf("AuditLog: %v", err)
	}

	var found bool
	for _, e := range entries {
		if e.Aktion == "update" && strings.Contains(e.Schluessel, "invoice-update.pdf") {
			if strings.Contains(e.Details, "Bruttobetrag") {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("no 'update' audit entry with Bruttobetrag diff found; entries: %+v", entries)
	}
}

// TestAuditLogDelete verifies that Delete produces a "delete" audit entry.
func TestAuditLogDelete(t *testing.T) {
	repo := newTestRepo(t)

	row := core.CSVRow{
		Dateiname:    "invoice-delete.pdf",
		Jahr:         "2026",
		Monat:        "06",
		Auftraggeber: "Gamma AG",
		Bruttobetrag: 50.0,
	}
	if _, err := repo.Insert(row); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if err := repo.Delete("2026", "06", "invoice-delete.pdf"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	entries, err := repo.AuditLog(10)
	if err != nil {
		t.Fatalf("AuditLog: %v", err)
	}

	var found bool
	for _, e := range entries {
		if e.Aktion == "delete" && strings.Contains(e.Schluessel, "invoice-delete.pdf") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("no 'delete' audit entry found for invoice-delete.pdf; entries: %+v", entries)
	}
}

// TestAuditLogLimit verifies that AuditLog respects the limit parameter.
func TestAuditLogLimit(t *testing.T) {
	repo := newTestRepo(t)

	for i := range 5 {
		row := core.CSVRow{
			Dateiname: "invoice-limit-" + string(rune('a'+i)) + ".pdf",
			Jahr:      "2026",
			Monat:     "06",
		}
		if _, err := repo.Insert(row); err != nil {
			t.Fatalf("Insert %d: %v", i, err)
		}
	}

	entries, err := repo.AuditLog(3)
	if err != nil {
		t.Fatalf("AuditLog(3): %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("AuditLog(3) = %d entries, want 3", len(entries))
	}
}
