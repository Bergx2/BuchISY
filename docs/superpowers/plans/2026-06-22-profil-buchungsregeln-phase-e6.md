# Profil-spezifische Buchungsregeln (Phase E6) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the booking rules base (Vorsteuer accounts, Bewirtung split accounts) per-profile, so each company books to its own SKR04 accounts (e.g. Boomstraat Vorsteuer 19 %→1576 / 7 %→1571 instead of the bundled 1406/1401).

**Architecture:** A `BookingRulesStore` (mirroring `ChartStore`) loads a per-profile `buchungsregeln.json` from the profile config dir when present, else the bundled defaults; `Save` writes the profile file. Startup loads `a.bookingRules` through the store. A Settings "Buchungsregeln" section lets the user set the Vorsteuer accounts (19 %/7 %) and the Bewirtung accounts + deductible-% per profile; saving writes the profile rules and reloads them. Existing categories/names are preserved by editing the loaded rules in place.

**Tech Stack:** Go 1.25, Fyne v2. Reuses `core.BookingRules`/`ParseBookingRules`, the `ChartStore` pattern, Phase B `a.chart`/`showAccountSearch`, the D2 SKR04-per-account settings control as the picker pattern.

## Global Constraints

- Per-profile file is `<configDir>/buchungsregeln.json`; a missing file falls back to the bundled `assets.BuchungsregelnJSON`. The profile file fully defines that profile's rules (it is the complete `BookingRules`, not a partial overlay).
- NEVER invent account numbers — defaults stay the bundled ones (Vorsteuer 19 %→1406, 7 %→1401; Bewirtung 6640/6644, 70 %); the user overrides per profile via account pickers.
- Editing rules preserves the existing category list and names (mutate the loaded `BookingRules` — set `VorsteuerKonten["19"/"7"]` and the `bewirtung` rule's accounts/percent — rather than rebuilding from scratch, so any extra categories survive).
- `a.bookingRules` is never nil (bundled fallback on any error).
- All user-facing strings via `a.bundle.T(...)` in both JSONs (valid JSON).
- `go build ./... && go test ./... && go vet ./internal/ui/` clean (ignore the pre-existing `clipboard_windows.go:206` warning). Commit per task.

---

### Task 1: BookingRulesStore

**Files:**
- Create: `internal/core/bookingrulesstore.go`
- Test: `internal/core/bookingrulesstore_test.go`

**Interfaces:**
- Consumes: `ParseBookingRules`, `BookingRules`.
- Produces: `type BookingRulesStore struct {…}`; `NewBookingRulesStore(configDir string, bundled []byte) *BookingRulesStore`;
  `(s *BookingRulesStore) Load() (*BookingRules, error)` — parses `<configDir>/buchungsregeln.json` if it exists, else the bundled bytes;
  `(s *BookingRulesStore) Save(r *BookingRules) error` — writes the rules as indented JSON to `<configDir>/buchungsregeln.json`.

- [ ] **Step 1: Write the failing test**

```go
package core

import (
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run TestBookingRulesStore`
Expected: FAIL (undefined NewBookingRulesStore).

- [ ] **Step 3: Implement**

Create `internal/core/bookingrulesstore.go`:

```go
package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// BookingRulesStore loads the booking rules for one profile: a per-profile
// buchungsregeln.json overrides the bundled defaults.
type BookingRulesStore struct {
	path    string
	bundled []byte
}

// NewBookingRulesStore creates a store rooted at configDir with the bundled
// default rules as fallback.
func NewBookingRulesStore(configDir string, bundled []byte) *BookingRulesStore {
	return &BookingRulesStore{
		path:    filepath.Join(configDir, "buchungsregeln.json"),
		bundled: bundled,
	}
}

// Load returns the profile's rules (its buchungsregeln.json) if present,
// otherwise the bundled defaults.
func (s *BookingRulesStore) Load() (*BookingRules, error) {
	if data, err := os.ReadFile(s.path); err == nil {
		return ParseBookingRules(data)
	}
	return ParseBookingRules(s.bundled)
}

// Save writes the rules to the profile's buchungsregeln.json.
func (s *BookingRulesStore) Save(r *BookingRules) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(s.path, data, 0644); err != nil {
		return fmt.Errorf("failed to save booking rules: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run TestBookingRulesStore && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/core/bookingrulesstore.go internal/core/bookingrulesstore_test.go
git commit -m "Add per-profile BookingRulesStore"
```

---

### Task 2: Load rules through the store at startup

**Files:**
- Modify: `internal/ui/app.go` (App struct + startProfile)
- Test: none (wiring; covered by build). 

**Interfaces:**
- Consumes: `core.NewBookingRulesStore`, `assets.BuchungsregelnJSON`, the existing `configDir`.
- Produces: `a.bookingRulesStore *core.BookingRulesStore`; `a.bookingRules` loaded via the store.

- [ ] **Step 1: Add the field**

In the `App` struct, next to `bookingRules *core.BookingRules` (~line 50), add:

```go
	bookingRulesStore *core.BookingRulesStore
```

- [ ] **Step 2: Load via the store**

In `startProfile`, replace the existing booking-rules block (currently `if rules, err := core.ParseBookingRules(assets.BuchungsregelnJSON); err != nil { … a.bookingRules = &core.BookingRules{} } else { a.bookingRules = rules }`, ~line 240-245) with:

```go
	a.bookingRulesStore = core.NewBookingRulesStore(configDir, assets.BuchungsregelnJSON)
	if rules, err := a.bookingRulesStore.Load(); err != nil {
		logger.Warn("Failed to load booking rules: %v", err)
		a.bookingRules = &core.BookingRules{}
	} else {
		a.bookingRules = rules
	}
```

- [ ] **Step 3: Build**

Run: `go build ./...`
Expected: clean.

- [ ] **Step 4: Commit**

```bash
git add internal/ui/app.go
git commit -m "Load booking rules through the per-profile store"
```

---

### Task 3: Settings — Buchungsregeln per profile

**Files:**
- Modify: `internal/ui/settings.go`
- Test: none (UI; covered by build + manual). 

**Interfaces:**
- Consumes: `a.bookingRules`, `a.bookingRulesStore.Save`, `a.showAccountSearch`, `a.chart`, `paymentSKR04Label` (the D2 helper that renders an account for a settings row — reuse it).
- Produces: a "Buchungsregeln" settings section editing Vorsteuer 19 %/7 % accounts + Bewirtung accounts + deductible-%; on save the profile rules are written and `a.bookingRules` reloaded.

- [ ] **Step 1: Build the section controls**

In `internal/ui/settings.go`, near the SKR04 section, read the current values from `a.bookingRules` and build controls. Hold edit state in locals:

```go
	vst19 := 1406
	if k, ok := a.bookingRules.VorsteuerKonto(19); ok {
		vst19 = k
	}
	vst7 := 1401
	if k, ok := a.bookingRules.VorsteuerKonto(7); ok {
		vst7 = k
	}
	bewAbz, bewNicht, bewProzent := 6640, 6644, 70.0
	if rule, ok := a.bookingRules.Rule("bewirtung"); ok {
		bewAbz, bewNicht, bewProzent = rule.KontoAbziehbar, rule.KontoNichtAbziehbar, rule.AbziehbarProzent
	}

	vst19Lbl := widget.NewLabel(paymentSKR04Label(a, vst19))
	vst19Btn := widget.NewButton(a.bundle.T("settings.rules.pick"), func() {
		a.showAccountSearch(vst19, func(n int) { vst19 = n; vst19Lbl.SetText(paymentSKR04Label(a, n)) })
	})
	vst7Lbl := widget.NewLabel(paymentSKR04Label(a, vst7))
	vst7Btn := widget.NewButton(a.bundle.T("settings.rules.pick"), func() {
		a.showAccountSearch(vst7, func(n int) { vst7 = n; vst7Lbl.SetText(paymentSKR04Label(a, n)) })
	})
	bewAbzLbl := widget.NewLabel(paymentSKR04Label(a, bewAbz))
	bewAbzBtn := widget.NewButton(a.bundle.T("settings.rules.pick"), func() {
		a.showAccountSearch(bewAbz, func(n int) { bewAbz = n; bewAbzLbl.SetText(paymentSKR04Label(a, n)) })
	})
	bewNichtLbl := widget.NewLabel(paymentSKR04Label(a, bewNicht))
	bewNichtBtn := widget.NewButton(a.bundle.T("settings.rules.pick"), func() {
		a.showAccountSearch(bewNicht, func(n int) { bewNicht = n; bewNichtLbl.SetText(paymentSKR04Label(a, n)) })
	})
	bewProzentEntry := widget.NewEntry()
	bewProzentEntry.SetText(strings.Replace(fmt.Sprintf("%g", bewProzent), ".", ",", 1))
```

Lay these out under an i18n `settings.rules.section` heading with labels `settings.rules.vst19`/`vst7`/`bewAbz`/`bewNicht`/`bewProzent` (use `container.NewBorder` rows like the D2 SKR04 row: label left, account label stretches, pick button right). Confirm `strings`, `fmt` are imported in settings.go (add if missing).

- [ ] **Step 2: Persist on save**

In the settings save action, after the existing persistence, build and save the profile rules by editing the loaded rules in place (preserving categories/names):

```go
	rules := a.bookingRules
	if rules.VorsteuerKonten == nil {
		rules.VorsteuerKonten = map[string]int{}
	}
	rules.VorsteuerKonten["19"] = vst19
	rules.VorsteuerKonten["7"] = vst7
	for i := range rules.Regeln {
		if rules.Regeln[i].Kategorie == "bewirtung" {
			rules.Regeln[i].KontoAbziehbar = bewAbz
			rules.Regeln[i].KontoNichtAbziehbar = bewNicht
			if p := parseDecimal(bewProzentEntry.Text); p > 0 {
				rules.Regeln[i].AbziehbarProzent = p
			}
		}
	}
	if err := a.bookingRulesStore.Save(rules); err != nil {
		a.logger.Warn("Failed to save booking rules: %v", err)
	}
	a.bookingRules = rules
```

(`parseDecimal` already exists in package `ui` from E1's `bookingeditor.go`.)

Add i18n keys to BOTH JSONs (valid): `settings.rules.section` (de "Buchungsregeln" / en "Booking rules"), `settings.rules.pick` (de "Konto…" / en "Account…"), `settings.rules.vst19` (de "Vorsteuer 19 %" / en "Input VAT 19%"), `settings.rules.vst7` (de "Vorsteuer 7 %" / en "Input VAT 7%"), `settings.rules.bewAbz` (de "Bewirtung abziehbar" / en "Hospitality deductible"), `settings.rules.bewNicht` (de "Bewirtung nicht abziehbar" / en "Hospitality non-deductible"), `settings.rules.bewProzent` (de "Bewirtung abziehbar %" / en "Hospitality deductible %").

- [ ] **Step 3: Build + vet + test**

Run: `go build ./... && go vet ./internal/ui/ && go test ./...`
Expected: clean. Validate both JSONs parse.

- [ ] **Step 4: Commit**

```bash
git add internal/ui/settings.go assets/i18n/de.json assets/i18n/en.json
git commit -m "Settings: edit per-profile booking rules (Vorsteuer + Bewirtung accounts)"
```

---

## Self-Review

- **Spec coverage:** per-profile rules file with bundled fallback (Task 1), loaded at startup (Task 2), editable per profile in Settings (Task 3). A profile (Boomstraat) can now set Vorsteuer 19 %→1576 / 7 %→1571 and Bewirtung accounts, and auto-bookings use them.
- **Placeholder scan:** Task 1 is fully coded; Tasks 2/3 reference concrete anchors (`startProfile` rules block, the D2 `paymentSKR04Label` row pattern, `parseDecimal`) with exact code.
- **Type consistency:** `BookingRulesStore` with `NewBookingRulesStore(configDir,bundled)`, `Load()(*BookingRules,error)`, `Save(*BookingRules)error`; `a.bookingRulesStore`; edit-in-place of `a.bookingRules` — consistent across tasks.
- **Data integrity:** defaults are the bundled accounts; the profile file fully defines its rules; editing preserves category names + extra categories; `a.bookingRules` never nil.
- **Out of scope:** new categories (§13b etc.) are E5; this phase only makes the EXISTING rule accounts profile-specific.
