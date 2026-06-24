package core

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
)

// Asset represents a fixed asset in the asset register (Anlagenbuchhaltung).
type Asset struct {
	ID                 string  `json:"id"`
	Bezeichnung        string  `json:"bezeichnung"`
	Anschaffungsdatum  string  `json:"anschaffungsdatum"` // DD.MM.YYYY
	Anschaffungswert   float64 `json:"anschaffungswert"`
	NutzungsdauerJahre int     `json:"nutzungsdauer_jahre"`
	Konto              int     `json:"konto"`
	AfaKonto           int     `json:"afa_konto"`
}

// LoadAssets reads assets from a JSON file. A missing file yields an empty
// slice and no error (mirrors LoadCashBooks).
func LoadAssets(path string) ([]Asset, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return []Asset{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read assets: %w", err)
	}
	var assets []Asset
	if err := json.Unmarshal(data, &assets); err != nil {
		return nil, fmt.Errorf("failed to parse assets: %w", err)
	}
	return assets, nil
}

// SaveAssets writes assets to a JSON file, creating the containing directory
// if it does not exist yet (mirrors SaveCashBooks).
func SaveAssets(path string, assets []Asset) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create assets directory: %w", err)
	}
	data, err := json.MarshalIndent(assets, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal assets: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write assets: %w", err)
	}
	return nil
}

// IsGWG returns true when the asset qualifies as a Geringwertiges
// Wirtschaftsgut (GWG): net acquisition cost ≤ 800 €.
func IsGWG(a Asset) bool {
	return a.Anschaffungswert <= 800.0
}

// LinearAfA returns the depreciation amount for a given calendar year using
// the linear method with pro-rata-temporis in the acquisition year.
//
// Rules:
//   - Returns 0 for years before the acquisition year.
//   - GWG (≤ 800 €): full Anschaffungswert in the acquisition year, 0 thereafter.
//   - Acquisition year: Anschaffungswert / NutzungsdauerJahre * (remainingMonths/12),
//     where remainingMonths = 13 − acquisitionMonth (so July = 6 months remaining).
//   - Full years (year > acquisition): full annual rate = Anschaffungswert / ND.
//   - Last year: whatever Restbuchwert remains (to eliminate rounding drift).
//   - Returns 0 after the asset is fully depreciated.
func LinearAfA(a Asset, jahr int) float64 {
	if a.NutzungsdauerJahre <= 0 || a.Anschaffungswert <= 0 {
		return 0
	}
	t, ok := parseGermanDate(a.Anschaffungsdatum)
	if !ok {
		return 0
	}
	acqYear := t.Year()
	acqMonth := int(t.Month())

	if jahr < acqYear {
		return 0
	}

	if IsGWG(a) {
		if jahr == acqYear {
			return round2(a.Anschaffungswert)
		}
		return 0
	}

	annualRate := a.Anschaffungswert / float64(a.NutzungsdauerJahre)

	// Pro-rata months in acquisition year: from acquisition month to December.
	proRataMonths := 13 - acqMonth // e.g. July → 13-7 = 6
	firstYearAfa := round2(annualRate * float64(proRataMonths) / 12.0)

	// Last depreciation year: the year in which the asset is fully written off.
	// Remaining value after the first year = AW - firstYearAfa.
	// Full years thereafter until fully depreciated.
	remainingAfterFirst := a.Anschaffungswert - firstYearAfa
	fullYearsNeeded := int(math.Ceil(remainingAfterFirst / annualRate))
	// Edge case: if firstYearAfa already covers everything.
	if remainingAfterFirst <= 0 {
		fullYearsNeeded = 0
	}
	lastYear := acqYear + fullYearsNeeded

	if jahr > lastYear {
		return 0
	}

	if jahr == acqYear {
		return firstYearAfa
	}

	// After the asset is fully written off.
	rbw := Restbuchwert(a, jahr-1)
	if rbw <= 0 {
		return 0
	}

	if jahr == lastYear {
		// Return the exact remaining book value to eliminate rounding drift.
		return round2(rbw)
	}

	return round2(annualRate)
}

// Restbuchwert returns the remaining book value at the end of the given year
// (Anschaffungswert minus cumulative AfA up to and including that year, ≥ 0).
func Restbuchwert(a Asset, jahr int) float64 {
	t, ok := parseGermanDate(a.Anschaffungsdatum)
	if !ok {
		return a.Anschaffungswert
	}
	acqYear := t.Year()
	if jahr < acqYear {
		return a.Anschaffungswert
	}
	cumulative := 0.0
	for y := acqYear; y <= jahr; y++ {
		cumulative += LinearAfA(a, y)
	}
	rbw := a.Anschaffungswert - cumulative
	if rbw < 0 {
		return 0
	}
	return round2(rbw)
}

// AnlagenRow is one row in the Anlagenspiegel for a given year.
type AnlagenRow struct {
	Asset       Asset
	AfaJahr     float64
	Restbuchwert float64
	GWG         bool
}

// Anlagenspiegel computes the asset register for the given year, returning one
// row per asset with its AfA and remaining book value.
func Anlagenspiegel(assets []Asset, jahr int) []AnlagenRow {
	rows := make([]AnlagenRow, 0, len(assets))
	for _, a := range assets {
		rows = append(rows, AnlagenRow{
			Asset:        a,
			AfaJahr:      LinearAfA(a, jahr),
			Restbuchwert: Restbuchwert(a, jahr),
			GWG:          IsGWG(a),
		})
	}
	return rows
}
