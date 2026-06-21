package core

import (
	"path/filepath"
	"testing"
)

func TestChartStoreMergeAndPersist(t *testing.T) {
	dir := t.TempDir()
	bundled := []byte(`[{"number":6640,"name":"Bewirtungskosten","type":"expense"}]`)
	s := NewChartStore(dir, bundled)

	c, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Find(6640); !ok {
		t.Fatal("bundled account missing before import")
	}

	// Import overrides the bundled name and adds a new account.
	if err := s.SaveImport([]SKRAccount{
		{Number: 6640, Name: "Bewirtung (eigene Liste)", Type: "expense"},
		{Number: 1800, Name: "Sparkasse", Type: "asset"},
	}); err != nil {
		t.Fatal(err)
	}
	c2, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	a, _ := c2.Find(6640)
	if a.Name != "Bewirtung (eigene Liste)" {
		t.Errorf("import did not override: %+v", a)
	}
	if _, ok := c2.Find(1800); !ok {
		t.Error("imported account 1800 missing")
	}
	_ = filepath.Join(dir, "chart_skr04.json")
}
