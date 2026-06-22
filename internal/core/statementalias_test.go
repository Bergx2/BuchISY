package core

import (
	"testing"
)

func TestStatementAliasLearn(t *testing.T) {
	dir := t.TempDir()
	s := NewStatementAliasStore(dir)
	s.Learn("AWS", "14.01. AMAZON WEB SERVICES EMEA 78,53")
	m, _ := s.Load()
	got := m["aws"]
	found := false
	for _, tok := range got {
		if tok == "amazon" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected learned token 'amazon' for aws, got %v", got)
	}
	// Persisted across instances.
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	m2, _ := NewStatementAliasStore(dir).Load()
	if len(m2["aws"]) == 0 {
		t.Errorf("aliases not persisted")
	}
}
