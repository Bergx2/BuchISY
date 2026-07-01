package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReconcileAccountFolders(t *testing.T) {
	accts := []BankAccount{
		{Name: "KSKMSE ...0712 Sparkasse", FolderName: "KSMSE ...0712 Sparkasse"}, // drifted → move
		{Name: "Wise ...9503", FolderName: "Wise ...9503"},                         // aligned → ensure only
		{Name: "Barkasse"},                                                         // empty pointer → backfill
		{Name: ""},                                                                 // no name → skipped
	}
	got := ReconcileAccountFolders(accts)
	if len(got) != 3 {
		t.Fatalf("want 3 actions (empty-name skipped), got %d: %+v", len(got), got)
	}
	// [0] drifted → move From→To
	want0 := SanitizeFilename("KSKMSE ...0712 Sparkasse")
	if got[0].Index != 0 || !got[0].Move || got[0].From != "KSMSE ...0712 Sparkasse" || got[0].To != want0 {
		t.Errorf("action0 = %+v; want move from %q to %q", got[0], "KSMSE ...0712 Sparkasse", want0)
	}
	// [1] aligned → no move
	if got[1].Move || got[1].From != "" || got[1].To != SanitizeFilename("Wise ...9503") {
		t.Errorf("action1 = %+v; want no-move ensure", got[1])
	}
	// [2] backfill → no move, To = name-derived
	if got[2].Move || got[2].From != "" || got[2].To != SanitizeFilename("Barkasse") {
		t.Errorf("action2 = %+v; want backfill", got[2])
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestMoveStatementFolder_SimpleRename(t *testing.T) {
	root := t.TempDir()
	from := filepath.Join(root, "old")
	to := filepath.Join(root, "new")
	writeFile(t, filepath.Join(from, "a.pdf"), "x")
	writeFile(t, filepath.Join(from, "metadata.json"), `{"a.pdf":{"note":"n"}}`)

	if err := MoveStatementFolder(from, to); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(to, "a.pdf")); err != nil {
		t.Errorf("a.pdf not moved: %v", err)
	}
	if _, err := os.Stat(from); !os.IsNotExist(err) {
		t.Errorf("source folder should be gone, err=%v", err)
	}
}

func TestMoveStatementFolder_MergeIntoExisting(t *testing.T) {
	root := t.TempDir()
	from := filepath.Join(root, "old")
	to := filepath.Join(root, "new")
	writeFile(t, filepath.Join(from, "a.pdf"), "fromA")
	writeFile(t, filepath.Join(from, "shared.pdf"), "fromShared")
	writeFile(t, filepath.Join(from, "metadata.json"), `{"a.pdf":{"note":"fa"},"shared.pdf":{"note":"fs"}}`)
	writeFile(t, filepath.Join(to, "shared.pdf"), "toShared")
	writeFile(t, filepath.Join(to, "metadata.json"), `{"shared.pdf":{"note":"ts"}}`)

	if err := MoveStatementFolder(from, to); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(to, "a.pdf")); string(b) != "fromA" {
		t.Errorf("a.pdf = %q, want fromA", b)
	}
	if b, _ := os.ReadFile(filepath.Join(to, "shared.pdf")); string(b) != "toShared" {
		t.Errorf("shared.pdf = %q, want toShared (destination wins)", b)
	}
	m, _ := LoadStatementMeta(to)
	if m["a.pdf"].Note != "fa" {
		t.Errorf("metadata a.pdf note = %q, want fa", m["a.pdf"].Note)
	}
	if m["shared.pdf"].Note != "ts" {
		t.Errorf("metadata shared.pdf note = %q, want ts (destination wins)", m["shared.pdf"].Note)
	}
}

func TestMoveStatementFolder_MissingSourceNoop(t *testing.T) {
	root := t.TempDir()
	if err := MoveStatementFolder(filepath.Join(root, "nope"), filepath.Join(root, "to")); err != nil {
		t.Errorf("missing source should be a no-op, got %v", err)
	}
}
