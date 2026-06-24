# E19.5 — Erfassungs-Ergonomie — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Steps use `- [ ]`.

**Goal:** Speed up account picking with recently-used + favorites (search/fuzzy already exists), and bundle the modal's top info widgets into one calm row.

**Architecture:** The review modal (`invoicemodal.go`) is ALREADY sectioned (`section()`/`selectableForm()` helpers: Identifikation / Beträge & Datum / Ablage / Buchung) and the account picker (`accountpicker.go showAccountSearch`) ALREADY fuzzy-searches number+name (`chart.Search`). So #8 reduces to a small info-line tidy; #4 adds a per-profile recent/favorites store (modeled on `companymap.go`) surfaced at the top of the picker.

**Tech Stack:** Go 1.25, Fyne. `internal/core`, `internal/ui`. Branch `feat/e19-5-modal-kontopicker`.

## Global Constraints

- `go build ./...`, `go test ./...`. Commit per task. Co-author trailer: `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`.
- i18n via `a.bundle.T(...)`, keys in BOTH `assets/i18n/{de,en}.json`.
- Per-profile persistence like `core.CompanyAccountMap` (a JSON file in the profile dir). READ `internal/core/companymap.go` for the load/save pattern and how the profile dir is resolved.

---

### Task 1: Account prefs store (recent + favorites)

**Files:** `internal/core/accountprefs.go` (new), `internal/core/accountprefs_test.go` (new). Model on `internal/core/companymap.go`.

**Interface:** `type AccountPrefs struct { Recent []int; Favorites []int }` with a store exposing: `RecordUse(konto int)` (prepend, dedupe, cap at 8), `IsFavorite(konto int) bool`, `ToggleFavorite(konto int)`, `RecentList() []int`, `FavoriteList() []int`, plus `Load`/`Save` to a per-profile `account_prefs.json` (mirror CompanyAccountMap's constructor/path/save).

- [ ] **Step 1: Test** (`accountprefs_test.go`): construct an in-memory store (mirror how companymap_test builds one with a temp path); `RecordUse(4663); RecordUse(4920); RecordUse(4663)` → `RecentList()` == `[4663,4920]` (most-recent-first, deduped). Record 9 distinct → length capped at 8, oldest dropped. `ToggleFavorite(8400)` → `IsFavorite(8400)` true; toggle again → false. Save then load → values persist.
- [ ] **Step 2:** run → fail.
- [ ] **Step 3:** Implement per the interface + the companymap persistence pattern (same dir resolution, JSON marshal, error handling).
- [ ] **Step 4:** run → pass + full core. Commit `E19.5: per-profile account prefs store (recent + favorites)`.

---

### Task 2: Surface recent + favorites in the picker

**Files:** `internal/ui/accountpicker.go`, `internal/ui/app.go` (construct the store on the App, like `companyAccounts`), `assets/i18n/{de,en}.json`.

**Context:** READ `showAccountSearch` (accountpicker.go) — a search `Entry` + a `widget.List` over `results []SKRAccount`, rebuilt on query change; `OnSelected` calls `onPick(number)`. READ how `a.companyAccounts` is constructed/stored on the App in `app.go` and replicate for `a.accountPrefs`.

- [ ] **Step 1:** Construct `a.accountPrefs` (load) where `a.companyAccounts` is set up; nil-guard everywhere it's used.
- [ ] **Step 2:** When the search query is EMPTY, build `results` as: the resolved Recent accounts (chart.Find each `RecentList()` entry), then Favorites not already in Recent, then the full `chart.All()` minus duplicates already shown — each group preceded by a non-selectable header row ("Zuletzt benutzt", "★ Favoriten", "Alle Konten"). When the query is non-empty, keep the current flat `chart.Search(q)` behavior. Use a small `pickerRow` model (`{account SKRAccount; header string}`) so the list can render headers vs accounts; header rows are non-selectable (OnSelected ignores them).
- [ ] **Step 3:** Render each account row as an HBox of a label (`accountLabel`) + a trailing ★/☆ toggle `widget.Button` (filled when `accountPrefs.IsFavorite`); the toggle calls `a.accountPrefs.ToggleFavorite(n)` + `Save()` + rebuilds the list. The row's main tap still calls `onPick`. (If an HBox-with-button list item is awkward in Fyne, a right-click/secondary menu "Als Favorit" is an acceptable alternative — but prefer the inline ★.)
- [ ] **Step 4:** In the `onPick` path (where the dialog closes with the chosen number), call `a.accountPrefs.RecordUse(chosen.Number)` + `Save()` so the choice bubbles to "Zuletzt benutzt" next time.
- [ ] **Step 5:** i18n keys `picker.recent` ("Zuletzt benutzt"/"Recently used"), `picker.favorites` ("★ Favoriten"/"★ Favorites"), `picker.all` ("Alle Konten"/"All accounts") in both JSONs. `go build ./... && go test ./...`. Commit `E19.5: recent + favorites in the account picker`.

---

### Task 3: Bundle the modal info-line (#8 polish)

**Files:** `internal/ui/invoicemodal.go`.

**Context:** `formItems` (invoicemodal.go ~703) currently starts with `quelleLabel`, `dupBanner`, `warningsLabel` as THREE separately stacked objects. The modal is otherwise already nicely sectioned — do NOT restructure the sections.

- [ ] **Step 1:** Combine `quelleLabel`, `dupBanner`, `warningsLabel` into ONE compact info container at the top (e.g. a `container.NewVBox` is fine, but group them under a single subtle area / or an HBox where they fit on one line when short). The goal is visual grouping, not three full-width stacked rows. Keep each widget's identity + Show/Hide logic intact (they are referenced elsewhere — `refreshWarnings`, `checkDuplicate`, the quelle badge) — only change how they're CONTAINED, not their wiring. Verify by reading every reference to these three variables before moving them.
- [ ] **Step 2:** `go build ./... && go test ./...`. Manually reason: the three info widgets still show/hide correctly (warnings live, dup banner on match, quelle when set); the sections below are unchanged. Commit `E19.5: bundle the modal info-line (quelle/dup/warnings)`.

## Self-Review

Spec coverage: #4 → Task 1 (store) + Task 2 (picker UI; search/fuzzy already existed). #8 → Task 3 (info-line tidy; sectioning already existed — honestly scoped down). Store unit-tested + mirrors companymap. No modal behavior change.
