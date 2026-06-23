# E19.1 — Darstellung & Einstieg — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Steps use `- [ ]`.

**Goal:** Add the missing zoom feedback overlay, helpful empty-month state, and dismissible "missing configuration" hints.

**Architecture:** The fluid UI zoom (#6) is ALREADY fully implemented (`internal/ui/theme.go` `buchisyTheme.Size()×scale`; `app.go` `setUIScale`/`adjustUIScale`, `registerZoomShortcuts` Ctrl+/−/0, `registerCtrlScrollZoom`, persisted to `Settings.UIScale` + app Preferences, clamp 0.6–2.5). The ONLY missing #6 piece is a transient "125 %" feedback overlay. #5 (empty state + config hints) is new UI plus one unit-tested core helper.

**Tech Stack:** Go 1.25, Fyne. `internal/core`, `internal/ui`. Branch `feat/e19-1-darstellung-einstieg`.

## Global Constraints

- `go build ./...`, `go test ./...`. Commit per task. Co-author trailer: `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`.
- i18n: user-facing strings via `a.bundle.T(...)` with keys in BOTH `assets/i18n/de.json` and `assets/i18n/en.json`.
- Do NOT reimplement zoom — it works. Only add the overlay.
- Hints must be dismissible and non-intrusive; never block the table.

---

### Task 1: Zoom "%" feedback overlay (#6 remainder)

**Files:** `internal/ui/app.go` (`setUIScale`).

**Context:** `setUIScale(scale float32)` (app.go) already sets the theme scale, refreshes, and persists. READ it. `a.theme.Scale()` returns the current factor. `a.window.Canvas()` is available.

- [ ] **Step 1:** Add a helper `func (a *App) showScaleOverlay()` that displays a brief, centered, auto-dismissing overlay with the current zoom percent: build `txt := fmt.Sprintf("%.0f %%", a.theme.Scale()*100)`; show it via a `widget.NewModalPopUp` (or `*widget.PopUp` at canvas center) containing a padded bold `widget.NewLabel(txt)`; auto-hide after ~900ms using `time.AfterFunc(900*time.Millisecond, func() { fyne.Do(popup.Hide) })`. Keep a single reusable popup field on `App` (e.g. `scalePopup *widget.PopUp`) so rapid scaling reuses/repositions one popup and resets its timer rather than stacking many. Guard `a.window == nil`.
- [ ] **Step 2:** Call `a.showScaleOverlay()` at the end of `setUIScale` (after the refresh), so Ctrl+scroll / Ctrl+± / Ctrl+0 all show feedback.
- [ ] **Step 3:** `go build ./...`. Manually reason: zooming shows "110 %", "120 %", … and the overlay disappears shortly after. Commit `E19.1: brief zoom-percent feedback overlay`.

---

### Task 2: "Missing configuration" hints (core helper + dismissible banner)

**Files:** `internal/core/confighints.go` (new), `internal/core/confighints_test.go` (new), `internal/ui/app.go` (`buildUI` header), `assets/i18n/{de,en}.json`.

**Interfaces:** `core.MissingConfigHints(s Settings, hasAPIKey bool) []string` — returns hint keys for unmet setup preconditions.

- [ ] **Step 1: Test** (`internal/core/confighints_test.go`):

```go
package core

import "testing"

func TestMissingConfigHints(t *testing.T) {
	has := func(hs []string, k string) bool {
		for _, h := range hs {
			if h == k {
				return true
			}
		}
		return false
	}
	// Claude mode without an API key → warn.
	s := Settings{ProcessingMode: "claude", StorageRoot: "/x"}
	if !has(MissingConfigHints(s, false), "hint.no_api_key") {
		t.Error("expected no_api_key hint")
	}
	if has(MissingConfigHints(s, true), "hint.no_api_key") {
		t.Error("must not warn when API key present")
	}
	// Local mode never needs an API key.
	if has(MissingConfigHints(Settings{ProcessingMode: "local", StorageRoot: "/x"}, false), "hint.no_api_key") {
		t.Error("local mode must not warn about API key")
	}
	// Missing storage root → warn.
	if !has(MissingConfigHints(Settings{ProcessingMode: "local"}, false), "hint.no_storage") {
		t.Error("expected no_storage hint")
	}
	// Fully configured → no hints.
	if h := MissingConfigHints(Settings{ProcessingMode: "local", StorageRoot: "/x"}, true); len(h) != 0 {
		t.Errorf("expected no hints, got %v", h)
	}
}
```

- [ ] **Step 2:** run → fail.
- [ ] **Step 3:** Implement `internal/core/confighints.go`:

```go
package core

import "strings"

// MissingConfigHints returns i18n keys for unmet setup preconditions, shown as
// dismissible banners. hasAPIKey reports whether a Claude API key is stored.
func MissingConfigHints(s Settings, hasAPIKey bool) []string {
	var hints []string
	if s.ProcessingMode == "claude" && !hasAPIKey {
		hints = append(hints, "hint.no_api_key")
	}
	if strings.TrimSpace(s.StorageRoot) == "" {
		hints = append(hints, "hint.no_storage")
	}
	return hints
}
```

(If `Settings.StorageRoot` has a different field name, use the real one — read `core/types.go`.)

- [ ] **Step 4:** run → pass + full core. Add i18n keys to both JSONs: `"hint.no_api_key"` = "Kein Claude-API-Key hinterlegt — in den Einstellungen ergänzen, um KI-Extraktion zu nutzen." / "No Claude API key set — add one in Settings to use AI extraction."; `"hint.no_storage"` = "Kein Speicherpfad gesetzt — in den Einstellungen wählen, wo Belege abgelegt werden." / "No storage folder set — choose one in Settings."; `"hint.dismiss"` = "Ausblenden" / "Dismiss".
- [ ] **Step 5:** In `buildUI` (app.go, the Belege branch, before assembling `header` at ~line 606), build a hints banner: call `core.MissingConfigHints(a.settings, a.hasAPIKey())` (find how an API key presence is checked in this codebase — keyring/Settings; if a helper exists reuse it, else inline the check used elsewhere). For each non-dismissed hint, render a slim banner row (an icon + `a.bundle.T(hint)` + a "Settings" button opening settings + a "✕"/dismiss button). Track dismissed hints for the session in an `App` field (e.g. `dismissedHints map[string]bool`) so ✕ hides them until restart. Prepend the banner(s) to `header` only when non-empty. Keep everything above the existing `topBar`/`filterRow`.
- [ ] **Step 6:** `go build ./... && go test ./...`. Commit `E19.1: dismissible missing-configuration hints`.

---

### Task 3: Empty-month state

**Files:** `internal/ui/app.go` (`buildUI`) and/or `internal/ui/table.go`.

**Context:** `buildUI` builds the table into `a.mainContent` (`container.NewBorder(header, statusBar, nil, nil, a.invoiceTable.Container())`). After `a.loadInvoices()`, the table may have zero rows for the selected month. READ how the table exposes its row count (e.g. a `RowCount()`/`len(it.filtered)` accessor — add a small exported accessor on `InvoiceTable` if none exists).

- [ ] **Step 1:** When the current month has zero invoices, show a centered empty-state instead of the bare table: a `container.NewCenter(container.NewVBox(...))` with a muted headline `a.bundle.T("empty.title")` ("Noch keine Belege in diesem Monat"), a line `a.bundle.T("empty.hint")` ("PDFs hierher ziehen oder einen Beleg hinzufügen"), and a primary `widget.NewButtonWithIcon(a.bundle.T("empty.add"), theme.ContentAddIcon(), func(){ a.showCustomFilePicker() })`. Swap the center object of the main Border from the table to this empty-state when the row count is 0, and back to the table otherwise. Re-evaluate on month/year change and after add/delete (wherever `loadInvoices` runs and rebuilds, or refresh the center object there).
- [ ] **Step 2:** Add the three i18n keys to both JSONs.
- [ ] **Step 3:** `go build ./... && go test ./...`. Manually reason: an empty month shows the hint + button; the button opens the picker; adding a receipt brings back the table. Commit `E19.1: helpful empty-month state`.

## Self-Review

Spec coverage: #6 → Task 1 (only the missing overlay; the rest already shipped). #5 → Task 2 (config hints) + Task 3 (empty state). No placeholders. `MissingConfigHints` signature consistent across test + impl + caller. Zoom is NOT reimplemented.
