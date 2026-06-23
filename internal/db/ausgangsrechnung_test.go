package db

import (
	"testing"

	"github.com/bergx2/buchisy/internal/core"
)

func TestAusgangsrechnungPersists(t *testing.T) {
	repo := newTestRepo(t)
	if _, err := repo.Insert(core.CSVRow{Dateiname: "out.pdf", Jahr: "2026", Monat: "06", Ausgangsrechnung: true}); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	rows, err := repo.List("2026", "06")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || !rows[0].Ausgangsrechnung {
		t.Fatalf("Ausgangsrechnung not persisted: %+v", rows)
	}
}
