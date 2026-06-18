package core

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGetMonthFolderWithYear(t *testing.T) {
	s := Settings{StorageRoot: filepath.Join("X", "root"), UseMonthSubfolders: true}
	sm := NewStorageManager(&s)
	got := sm.GetMonthFolder(2026, time.April)
	want := filepath.Join("X", "root", "2026", "2026-04")
	if got != want {
		t.Errorf("GetMonthFolder = %q, want %q", got, want)
	}

	s.UseMonthSubfolders = false
	if got := sm.GetMonthFolder(2026, time.April); got != s.StorageRoot {
		t.Errorf("without subfolders: GetMonthFolder = %q, want %q", got, s.StorageRoot)
	}
}

func TestInvoiceFilePath(t *testing.T) {
	month := filepath.Join("X", "root", "2026", "2026-04")
	plain := InvoiceFilePath(month, CSVRow{Dateiname: "a.pdf"})
	if plain != filepath.Join(month, "a.pdf") {
		t.Errorf("plain = %q", plain)
	}
	bar := InvoiceFilePath(month, CSVRow{Dateiname: "a.pdf", Unterordner: "Bar"})
	if bar != filepath.Join(month, "Bar", "a.pdf") {
		t.Errorf("bar = %q", bar)
	}
}

func TestMigrateToYearFolders(t *testing.T) {
	root := t.TempDir()
	monthDir := filepath.Join(root, "2026-04")
	if err := os.MkdirAll(monthDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(monthDir, "invoices.csv"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	s := Settings{StorageRoot: root, UseMonthSubfolders: true}
	sm := NewStorageManager(&s)

	if err := sm.MigrateToYearFolders(func(string) {}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	moved := filepath.Join(root, "2026", "2026-04", "invoices.csv")
	if _, err := os.Stat(moved); err != nil {
		t.Fatalf("expected file at %s: %v", moved, err)
	}
	if _, err := os.Stat(monthDir); !os.IsNotExist(err) {
		t.Errorf("old folder %s should be gone", monthDir)
	}
	if err := sm.MigrateToYearFolders(func(string) {}); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
}

func TestMigrateCashToBar(t *testing.T) {
	root := t.TempDir()
	monthDir := filepath.Join(root, "2026", "2026-04")
	if err := os.MkdirAll(monthDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(monthDir, "bar.pdf"), []byte("p"), 0644); err != nil {
		t.Fatal(err)
	}

	s := Settings{StorageRoot: root, UseMonthSubfolders: true}
	sm := NewStorageManager(&s)
	repo := NewCSVRepository()
	csvPath := filepath.Join(monthDir, "invoices.csv")
	if err := repo.Append(csvPath, CSVRow{Dateiname: "bar.pdf", Bankkonto: "Barkasse"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.Append(csvPath, CSVRow{Dateiname: "x.pdf", Bankkonto: "Sparkasse"}); err != nil {
		t.Fatal(err)
	}

	cashAccounts := map[string]struct{}{"Barkasse": {}}
	if err := sm.MigrateCashToBar(repo, cashAccounts, func(string) {}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	if _, err := os.Stat(filepath.Join(monthDir, "Bar", "bar.pdf")); err != nil {
		t.Errorf("bar.pdf should be in Bar/: %v", err)
	}
	rows, err := repo.Load(csvPath)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	for _, r := range rows {
		if r.Dateiname == "bar.pdf" && r.Unterordner != "Bar" {
			t.Errorf("bar.pdf row: Unterordner = %q, want Bar", r.Unterordner)
		}
		if r.Bankkonto == "Sparkasse" && r.Unterordner != "" {
			t.Errorf("non-cash row should not be moved: %+v", r)
		}
	}
	// Idempotent: a second run does nothing and does not error.
	if err := sm.MigrateCashToBar(repo, cashAccounts, func(string) {}); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
}

func TestCopyAndRename(t *testing.T) {
	src := filepath.Join(t.TempDir(), "quelle.pdf")
	if err := os.WriteFile(src, []byte("inhalt"), 0644); err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	sm := NewStorageManager(&Settings{})

	name, err := sm.CopyAndRename(src, target, "ziel.pdf")
	if err != nil {
		t.Fatalf("CopyAndRename: %v", err)
	}
	if name != "ziel.pdf" {
		t.Errorf("name = %q, want ziel.pdf", name)
	}
	if _, err := os.Stat(filepath.Join(target, "ziel.pdf")); err != nil {
		t.Errorf("target file missing: %v", err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Errorf("source file must still exist after copy: %v", err)
	}

	// Collision -> _2 suffix.
	name2, err := sm.CopyAndRename(src, target, "ziel.pdf")
	if err != nil {
		t.Fatalf("CopyAndRename collision: %v", err)
	}
	if name2 != "ziel_2.pdf" {
		t.Errorf("collision name = %q, want ziel_2.pdf", name2)
	}
}

func TestMoveAndRenameRemovesSource(t *testing.T) {
	src := filepath.Join(t.TempDir(), "quelle.pdf")
	if err := os.WriteFile(src, []byte("inhalt"), 0644); err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	sm := NewStorageManager(&Settings{})

	name, err := sm.MoveAndRename(src, target, "ziel.pdf")
	if err != nil {
		t.Fatalf("MoveAndRename: %v", err)
	}
	if name != "ziel.pdf" {
		t.Errorf("name = %q, want ziel.pdf", name)
	}
	if _, err := os.Stat(filepath.Join(target, "ziel.pdf")); err != nil {
		t.Errorf("target file missing: %v", err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Errorf("source file should be gone after move")
	}
}
