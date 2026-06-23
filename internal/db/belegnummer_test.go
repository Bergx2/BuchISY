package db

import (
	"testing"

	"github.com/bergx2/buchisy/internal/core"
)

// TestNextBelegnummer verifies the per-year sequential numbering and that the
// value survives a DB round-trip (Insert → List).
func TestNextBelegnummer(t *testing.T) {
	repo := newTestRepo(t)

	// Empty DB → first number of the year.
	first, err := repo.NextBelegnummer("2026")
	if err != nil {
		t.Fatalf("NextBelegnummer: %v", err)
	}
	if first != "2026-0001" {
		t.Fatalf("first = %q, want 2026-0001", first)
	}

	// Insert with that number, then the next one must increment.
	if _, err := repo.Insert(core.CSVRow{Dateiname: "a.pdf", Jahr: "2026", Monat: "01", Belegnummer: first}); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	second, err := repo.NextBelegnummer("2026")
	if err != nil {
		t.Fatal(err)
	}
	if second != "2026-0002" {
		t.Fatalf("second = %q, want 2026-0002", second)
	}

	// A different year is an independent sequence.
	other, err := repo.NextBelegnummer("2025")
	if err != nil {
		t.Fatal(err)
	}
	if other != "2025-0001" {
		t.Fatalf("2025 sequence = %q, want 2025-0001", other)
	}

	// The stored Belegnummer survives the round-trip via List.
	rows, err := repo.List("2026", "01")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Belegnummer != "2026-0001" {
		t.Fatalf("List Belegnummer = %+v, want one row with 2026-0001", rows)
	}

	// The sequence keys on the "YYYY-" prefix, not the jahr column: a row filed
	// under 2025 but numbered 2026-0009 advances the 2026 sequence.
	if _, err := repo.Insert(core.CSVRow{Dateiname: "b.pdf", Jahr: "2025", Monat: "06", Belegnummer: "2026-0009"}); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	next, err := repo.NextBelegnummer("2026")
	if err != nil {
		t.Fatal(err)
	}
	if next != "2026-0010" {
		t.Fatalf("after 2026-0009, next = %q, want 2026-0010", next)
	}
}
