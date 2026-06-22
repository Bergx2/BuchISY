package core

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteBackupZip(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	b := filepath.Join(dir, "b.csv")
	if err := os.WriteFile(a, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("x,y"), 0644); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	n, err := WriteBackupZip(&buf, map[string]string{
		"invoices.db":      a,
		"2026-06/data.csv": b,
		"missing.txt":      filepath.Join(dir, "nope.txt"), // skipped
	})
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("wrote %d files, want 2", n)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
	}
	if !names["invoices.db"] || !names["2026-06/data.csv"] || names["missing.txt"] {
		t.Errorf("zip entries wrong: %+v", names)
	}
}
