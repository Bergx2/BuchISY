package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBookingRulesStore(t *testing.T) {
	dir := t.TempDir()
	bundled := []byte(`{"vorsteuer_konten":{"19":1406,"7":1401},"regeln":[{"kategorie":"standard","name":"Standard"},{"kategorie":"bewirtung","name":"Bewirtung","abziehbar_prozent":70,"konto_abziehbar":6640,"konto_nicht_abziehbar":6644}]}`)
	s := NewBookingRulesStore(dir, bundled)

	// No profile file yet → bundled defaults.
	r, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if k, _ := r.VorsteuerKonto(19); k != 1406 {
		t.Errorf("bundled VSt 19%% = %d, want 1406", k)
	}

	// Override for this profile (Boomstraat-style) and persist.
	r.VorsteuerKonten["19"] = 1576
	r.VorsteuerKonten["7"] = 1571
	if err := s.Save(r); err != nil {
		t.Fatal(err)
	}

	// A fresh store over the same dir must read the profile file, not the bundled.
	s2 := NewBookingRulesStore(dir, bundled)
	r2, err := s2.Load()
	if err != nil {
		t.Fatal(err)
	}
	if k, _ := r2.VorsteuerKonto(19); k != 1576 {
		t.Errorf("profile VSt 19%% = %d, want 1576", k)
	}
	if k, _ := r2.VorsteuerKonto(7); k != 1571 {
		t.Errorf("profile VSt 7%% = %d, want 1571", k)
	}
	_ = filepath.Join(dir, "buchungsregeln.json")
}

func TestBookingRulesStoreCorruptFileFallsBackToBundled(t *testing.T) {
	dir := t.TempDir()
	bundled := []byte(`{"vorsteuer_konten":{"19":1406,"7":1401},"regeln":[{"kategorie":"standard","name":"Standard"}]}`)
	// A corrupt profile file must not break all bookings — fall back to bundled.
	if err := os.WriteFile(filepath.Join(dir, "buchungsregeln.json"), []byte("{ not valid json"), 0644); err != nil {
		t.Fatal(err)
	}
	r, err := NewBookingRulesStore(dir, bundled).Load()
	if err != nil {
		t.Fatalf("corrupt profile file should fall back, not error: %v", err)
	}
	if k, _ := r.VorsteuerKonto(19); k != 1406 {
		t.Errorf("fallback VSt 19%% = %d, want 1406 (bundled)", k)
	}
	if _, ok := r.Rule("standard"); !ok {
		t.Error("fallback should expose the bundled standard rule")
	}
}

func TestBookingRulesStoreMergesNewBundledCategories(t *testing.T) {
	dir := t.TempDir()
	// A previously-saved profile (Boomstraat-style): custom Vorsteuer 19→1576,
	// only the "standard" rule, none of the newer bundled categories.
	saved := `{"vorsteuer_konten":{"19":1576},"regeln":[{"kategorie":"standard","name":"Standard"}]}`
	if err := os.WriteFile(filepath.Join(dir, "buchungsregeln.json"), []byte(saved), 0644); err != nil {
		t.Fatal(err)
	}
	bundled := []byte(`{"vorsteuer_konten":{"19":1406,"7":1401},"regeln":[` +
		`{"kategorie":"standard","name":"S"},` +
		`{"kategorie":"geschenke","name":"G","schwelle":35,"konto_abziehbar":6610,"konto_nicht_abziehbar":6620}]}`)
	r, err := NewBookingRulesStore(dir, bundled).Load()
	if err != nil {
		t.Fatal(err)
	}
	if k, _ := r.VorsteuerKonto(19); k != 1576 {
		t.Errorf("saved override VSt 19%% = %d, want 1576", k)
	}
	if k, _ := r.VorsteuerKonto(7); k != 1401 {
		t.Errorf("bundled-only VSt 7%% not merged: got %d, want 1401", k)
	}
	if g, ok := r.Rule("geschenke"); !ok || g.KontoAbziehbar != 6610 {
		t.Errorf("new bundled category 'geschenke' not merged: %+v ok=%v", g, ok)
	}
	n := 0
	for _, x := range r.Regeln {
		if x.Kategorie == "standard" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("standard rule duplicated: %d copies", n)
	}
}
