package core

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLinearAfA_Standard tests the worked example from the spec:
// 1200 € net, ND 3 years, Anschaffung 01.07.2024
// → AfA 2024 = 200, 2025 = 400, 2026 = 400, 2027 = 200, 2028 = 0
// → Restbuchwert 2024 = 1000
func TestLinearAfA_Standard(t *testing.T) {
	a := Asset{
		ID:                 "test-1",
		Bezeichnung:        "Laptop",
		Anschaffungsdatum:  "01.07.2024",
		Anschaffungswert:   1200.0,
		NutzungsdauerJahre: 3,
		Konto:              420,
		AfaKonto:           4830,
	}

	tests := []struct {
		jahr    int
		wantAfa float64
	}{
		{2023, 0},
		{2024, 200},
		{2025, 400},
		{2026, 400},
		{2027, 200},
		{2028, 0},
	}

	for _, tt := range tests {
		got := LinearAfA(a, tt.jahr)
		if got != tt.wantAfa {
			t.Errorf("LinearAfA(%d) = %.2f, want %.2f", tt.jahr, got, tt.wantAfa)
		}
	}

	// Restbuchwert at end of 2024 should be 1200 - 200 = 1000
	rbw := Restbuchwert(a, 2024)
	if rbw != 1000.0 {
		t.Errorf("Restbuchwert(2024) = %.2f, want 1000.00", rbw)
	}

	// Restbuchwert at end of 2027 should be 0
	rbw2027 := Restbuchwert(a, 2027)
	if rbw2027 != 0.0 {
		t.Errorf("Restbuchwert(2027) = %.2f, want 0.00", rbw2027)
	}
}

// TestLinearAfA_GWG tests that a GWG (≤ 800 €) is fully expensed in the
// acquisition year and yields 0 in all subsequent years.
func TestLinearAfA_GWG(t *testing.T) {
	a := Asset{
		ID:                 "gwg-1",
		Bezeichnung:        "Drucker",
		Anschaffungsdatum:  "15.03.2024",
		Anschaffungswert:   700.0,
		NutzungsdauerJahre: 3,
		Konto:              420,
		AfaKonto:           4830,
	}

	if !IsGWG(a) {
		t.Fatal("IsGWG: expected true for 700 € asset")
	}

	// Acquisition year: full value
	got2024 := LinearAfA(a, 2024)
	if got2024 != 700.0 {
		t.Errorf("LinearAfA GWG 2024 = %.2f, want 700.00", got2024)
	}

	// Subsequent years: 0
	for _, yr := range []int{2025, 2026, 2027} {
		got := LinearAfA(a, yr)
		if got != 0.0 {
			t.Errorf("LinearAfA GWG %d = %.2f, want 0.00", yr, got)
		}
	}

	// Restbuchwert after acquisition year should be 0
	rbw := Restbuchwert(a, 2024)
	if rbw != 0.0 {
		t.Errorf("Restbuchwert GWG 2024 = %.2f, want 0.00", rbw)
	}
}

// TestIsGWG tests the 800 € threshold (inclusive).
func TestIsGWG(t *testing.T) {
	cases := []struct {
		wert float64
		want bool
	}{
		{799.99, true},
		{800.0, true},
		{800.01, false},
		{1200.0, false},
	}
	for _, c := range cases {
		a := Asset{Anschaffungswert: c.wert}
		if got := IsGWG(a); got != c.want {
			t.Errorf("IsGWG(%.2f) = %v, want %v", c.wert, got, c.want)
		}
	}
}

// TestAnlagenspiegel tests the aggregation helper.
func TestAnlagenspiegel(t *testing.T) {
	assets := []Asset{
		{
			ID: "a1", Bezeichnung: "Laptop",
			Anschaffungsdatum: "01.07.2024", Anschaffungswert: 1200,
			NutzungsdauerJahre: 3,
		},
		{
			ID: "gwg1", Bezeichnung: "Drucker",
			Anschaffungsdatum: "15.03.2024", Anschaffungswert: 700,
			NutzungsdauerJahre: 3,
		},
	}

	rows := Anlagenspiegel(assets, 2024)
	if len(rows) != 2 {
		t.Fatalf("Anlagenspiegel: expected 2 rows, got %d", len(rows))
	}

	// Row 0: Laptop
	if rows[0].AfaJahr != 200.0 {
		t.Errorf("Laptop AfaJahr = %.2f, want 200.00", rows[0].AfaJahr)
	}
	if rows[0].Restbuchwert != 1000.0 {
		t.Errorf("Laptop Restbuchwert = %.2f, want 1000.00", rows[0].Restbuchwert)
	}
	if rows[0].GWG {
		t.Error("Laptop: expected GWG=false")
	}

	// Row 1: GWG Drucker
	if rows[1].AfaJahr != 700.0 {
		t.Errorf("Drucker AfaJahr = %.2f, want 700.00", rows[1].AfaJahr)
	}
	if rows[1].Restbuchwert != 0.0 {
		t.Errorf("Drucker Restbuchwert = %.2f, want 0.00", rows[1].Restbuchwert)
	}
	if !rows[1].GWG {
		t.Error("Drucker: expected GWG=true")
	}
}

// TestBuildAnlagenspiegelPDF tests that the PDF builder returns a non-empty
// valid %PDF document without error.
func TestBuildAnlagenspiegelPDF(t *testing.T) {
	assets := []Asset{
		{ID: "1", Bezeichnung: "Laptop", Anschaffungsdatum: "01.07.2024",
			Anschaffungswert: 1200, NutzungsdauerJahre: 3},
		{ID: "2", Bezeichnung: "Drucker", Anschaffungsdatum: "15.03.2024",
			Anschaffungswert: 700, NutzungsdauerJahre: 3},
	}
	rows := Anlagenspiegel(assets, 2024)
	data, err := BuildAnlagenspiegelPDF(rows, 2024, "Anlagenspiegel 2024", "Test GmbH")
	if err != nil {
		t.Fatalf("BuildAnlagenspiegelPDF: %v", err)
	}
	if len(data) < 100 {
		t.Fatalf("BuildAnlagenspiegelPDF: output too small (%d bytes)", len(data))
	}
	if string(data[:4]) != "%PDF" {
		t.Errorf("BuildAnlagenspiegelPDF: output does not start with %%PDF")
	}
}

// TestLoadSaveAssets tests the JSON round-trip (mirrors kassenbuch pattern).
func TestLoadSaveAssets(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "assets.json")

	// Missing file → empty slice, no error.
	got, err := LoadAssets(path)
	if err != nil {
		t.Fatalf("LoadAssets (missing): unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("LoadAssets (missing): expected empty, got %d", len(got))
	}

	assets := []Asset{
		{ID: "1", Bezeichnung: "Laptop", Anschaffungsdatum: "01.07.2024",
			Anschaffungswert: 1200, NutzungsdauerJahre: 3, Konto: 420, AfaKonto: 4830},
	}
	if err := SaveAssets(path, assets); err != nil {
		t.Fatalf("SaveAssets: %v", err)
	}

	// Verify file exists.
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("SaveAssets: file not created: %v", err)
	}

	loaded, err := LoadAssets(path)
	if err != nil {
		t.Fatalf("LoadAssets: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("LoadAssets: expected 1 asset, got %d", len(loaded))
	}
	if loaded[0].Bezeichnung != "Laptop" {
		t.Errorf("LoadAssets: Bezeichnung = %q, want \"Laptop\"", loaded[0].Bezeichnung)
	}
}

func TestFindAssetByBeleg(t *testing.T) {
	assets := []Asset{
		{ID: "x1", BelegRef: "2026-0006", Anschaffungswert: 1066.85},
		{ID: "x2", BelegRef: ""},
	}
	if a, ok := FindAssetByBeleg(assets, "2026-0006"); !ok || a.ID != "x1" {
		t.Fatalf("expected to find x1, got %+v ok=%v", a, ok)
	}
	if _, ok := FindAssetByBeleg(assets, "2026-9999"); ok {
		t.Error("must not match an unknown Belegnummer")
	}
	if _, ok := FindAssetByBeleg(assets, ""); ok {
		t.Error("empty Belegnummer must not match")
	}
}
