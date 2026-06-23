package db

import (
	"testing"

	"github.com/bergx2/buchisy/internal/core"
)

func TestRenumberBelegnummern(t *testing.T) {
	repo := newTestRepo(t)
	// 2026 out of date order; one already has a (wrong) number; one empty.
	if _, err := repo.Insert(core.CSVRow{Dateiname: "b.pdf", Jahr: "2026", Monat: "03", Rechnungsdatum: "15.03.2026", Belegnummer: "2026-0099"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Insert(core.CSVRow{Dateiname: "a.pdf", Jahr: "2026", Monat: "01", Rechnungsdatum: "10.01.2026"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Insert(core.CSVRow{Dateiname: "c.pdf", Jahr: "2025", Monat: "12", Rechnungsdatum: "31.12.2025"}); err != nil {
		t.Fatal(err)
	}

	n, err := repo.RenumberBelegnummern()
	if err != nil {
		t.Fatalf("RenumberBelegnummern: %v", err)
	}
	if n != 3 {
		t.Fatalf("count = %d, want 3", n)
	}

	get := func(jahr, monat string) string {
		rows, err := repo.List(jahr, monat)
		if err != nil || len(rows) != 1 {
			t.Fatalf("List(%s,%s): %v rows=%d", jahr, monat, err, len(rows))
		}
		return rows[0].Belegnummer
	}
	// 2026 chronological: Jan → 0001, Mar → 0002 (the stale 0099 is replaced).
	if got := get("2026", "01"); got != "2026-0001" {
		t.Errorf("Jan = %q, want 2026-0001", got)
	}
	if got := get("2026", "03"); got != "2026-0002" {
		t.Errorf("Mar = %q, want 2026-0002", got)
	}
	// 2025 independent sequence.
	if got := get("2025", "12"); got != "2025-0001" {
		t.Errorf("Dec = %q, want 2025-0001", got)
	}
}
