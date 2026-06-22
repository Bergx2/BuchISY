# Backup & Plausibilitätswarnungen (Phase E5) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A one-click data backup (database + config + CSV exports as a ZIP) and plausibility warnings that catch bad invoice data before saving (gross ≠ net + VAT, missing Gegenkonto, foreign currency without a rate).

**Architecture:** A pure `WriteBackupZip` zips a name→path map; a "Backup erstellen" menu action collects the DB file, the profile config JSONs, and every `invoices.csv` under the storage root and saves the ZIP. A pure `InvoiceWarnings(row)` returns human-readable warnings; the confirmation modal checks them on save and asks the user to confirm if any are present.

**Tech Stack:** Go 1.25, `archive/zip` (stdlib), Fyne v2. Reuses the storage-root scan, the profile config dir, and the file-save dialog pattern.

## Global Constraints

- `WriteBackupZip` is pure (`internal/core`, `io.Writer` + a map), no UI/DB. The backup is the app's DATA (DB + config JSONs + CSVs) — NOT the original PDFs (those are the user's files in their folders); label it accordingly.
- `InvoiceWarnings` is pure (`internal/core`), returns `[]string` (empty = no warnings). Warnings never block — the user can always proceed; they are advisory.
- Warning thresholds: gross-mismatch tolerance 0.02 (gross vs net+VAT+tip); a 0-Gegenkonto is "no account"; a non-empty, non-"EUR" currency with `Wechselkurs <= 0` is "foreign without rate".
- All new user-facing strings via `a.bundle.T(...)` in both JSONs (valid JSON).
- `go build ./... && go test ./... && go vet ./internal/ui/` clean (ignore the pre-existing `clipboard_windows.go:206` warning). Commit per task.

---

### Task 1: WriteBackupZip (core)

**Files:**
- Create: `internal/core/backup.go`
- Test: `internal/core/backup_test.go`

**Interfaces:**
- Consumes: `archive/zip`, `io`, `os`.
- Produces: `WriteBackupZip(w io.Writer, files map[string]string) (int, error)` — writes a ZIP to `w` containing each `files[zipName] = absolutePath` whose source file is readable; returns the count of files written. A missing/unreadable source is skipped (not fatal); a zip-write error is returned.

- [ ] **Step 1: Write the failing test**

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run TestWriteBackupZip`
Expected: FAIL (undefined WriteBackupZip).

- [ ] **Step 3: Implement**

Create `internal/core/backup.go`:

```go
package core

import (
	"archive/zip"
	"io"
	"os"
)

// WriteBackupZip writes a ZIP to w containing each files[zipName]=sourcePath
// whose source is readable. Unreadable/missing sources are skipped. Returns the
// number of files written.
func WriteBackupZip(w io.Writer, files map[string]string) (int, error) {
	zw := zip.NewWriter(w)
	count := 0
	for name, path := range files {
		src, err := os.Open(path)
		if err != nil {
			continue // skip missing/unreadable
		}
		fw, err := zw.Create(name)
		if err != nil {
			_ = src.Close()
			_ = zw.Close()
			return count, err
		}
		if _, err := io.Copy(fw, src); err != nil {
			_ = src.Close()
			_ = zw.Close()
			return count, err
		}
		_ = src.Close()
		count++
	}
	if err := zw.Close(); err != nil {
		return count, err
	}
	return count, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run TestWriteBackupZip && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/core/backup.go internal/core/backup_test.go
git commit -m "Add WriteBackupZip: zip a name->path file map"
```

---

### Task 2: Backup menu action (UI)

**Files:**
- Create: `internal/ui/backup.go`
- Modify: `internal/ui/app.go` (menu item)
- Test: none (UI; covered by build + manual). 

**Interfaces:**
- Consumes: `core.WriteBackupZip`, `db.GetGlobalDBPath`, `core.GetProfileConfigDir`, the storage root, `dialog.NewFileSave`.
- Produces: `func (a *App) showBackup()` opened from a "Backup erstellen" menu item; saves a ZIP of the DB + config JSONs + all month CSVs.

- [ ] **Step 1: Build the action**

Create `internal/ui/backup.go`:

```go
package ui

import (
	"bytes"
	"os"
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"

	"github.com/bergx2/buchisy/internal/core"
	"github.com/bergx2/buchisy/internal/db"
)

// showBackup collects the app's data (database, profile config JSONs, and the
// month CSVs under the storage root) into a ZIP and asks where to save it.
func (a *App) showBackup() {
	configDir, err := core.GetProfileConfigDir(a.profile)
	if err != nil {
		a.showError(a.bundle.T("error.processing.title"), err.Error())
		return
	}
	files := map[string]string{}
	files["invoices.db"] = db.GetGlobalDBPath(configDir)
	for _, name := range []string{"settings.json", "chart_skr04.json", "buchungsregeln.json", "booking_templates.json", "company_accounts.json"} {
		files["config/"+name] = filepath.Join(configDir, name)
	}
	// All invoices.csv under the storage root, keyed by their relative path.
	root := a.settings.StorageRoot
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, werr error) error {
		if werr != nil || d.IsDir() || d.Name() != "invoices.csv" {
			return nil
		}
		if rel, rerr := filepath.Rel(root, path); rerr == nil {
			files["csv/"+filepath.ToSlash(rel)] = path
		}
		return nil
	})

	var buf bytes.Buffer
	n, err := core.WriteBackupZip(&buf, files)
	if err != nil {
		a.showError(a.bundle.T("error.processing.title"), err.Error())
		return
	}
	d := dialog.NewFileSave(func(wc fyne.URIWriteCloser, ferr error) {
		if wc == nil {
			return
		}
		defer wc.Close()
		if ferr != nil {
			a.showError(a.bundle.T("error.processing.title"), ferr.Error())
			return
		}
		if _, werr := wc.Write(buf.Bytes()); werr != nil {
			a.showError(a.bundle.T("error.processing.title"), werr.Error())
			return
		}
		a.showInfo(a.bundle.T("backup.title"), a.bundle.T("backup.done", n))
	}, a.window)
	d.SetFileName("BuchISY-Backup.zip")
	d.Show()
}
```

(Confirm `core.GetProfileConfigDir`, `db.GetGlobalDBPath`, `a.profile`, `a.settings.StorageRoot`, `a.showInfo`/`a.showError` exist — they do, used elsewhere. Adjust the config filename list if any file is named differently.)

- [ ] **Step 2: Menu item + i18n**

In `internal/ui/app.go`, next to the other menu items (~line 716-722), add `fyne.NewMenuItem("Backup erstellen", func() { a.showBackup() })` (literal German, matching neighbours). Add i18n keys `backup.title` (de "Backup"/en "Backup") + `backup.done` (de "Backup erstellt (%d Dateien)."/en "Backup created (%d files).") to both JSONs.

- [ ] **Step 3: Build + vet + test**

Run: `go build ./... && go vet ./internal/ui/ && go test ./...`
Expected: clean. Validate both JSONs.

- [ ] **Step 4: Commit**

```bash
git add internal/ui/backup.go internal/ui/app.go assets/i18n/de.json assets/i18n/en.json
git commit -m "Add 'Backup erstellen' menu: zip DB + config + CSVs"
```

---

### Task 3: InvoiceWarnings (core)

**Files:**
- Create: `internal/core/warnings.go`
- Test: `internal/core/warnings_test.go`

**Interfaces:**
- Consumes: `CSVRow`, `math`.
- Produces: `InvoiceWarnings(row CSVRow) []string` — advisory warnings: gross ≠ net+VAT+tip (|Δ|>0.02 and gross>0), Gegenkonto==0, non-EUR currency with Wechselkurs<=0.

- [ ] **Step 1: Write the failing test**

```go
package core

import (
	"strings"
	"testing"
)

func hasWarn(ws []string, sub string) bool {
	for _, w := range ws {
		if strings.Contains(w, sub) {
			return true
		}
	}
	return false
}

func TestInvoiceWarnings(t *testing.T) {
	good := CSVRow{BetragNetto: 100, SteuersatzBetrag: 19, Bruttobetrag: 119, Gegenkonto: 6815, Waehrung: "EUR"}
	if w := InvoiceWarnings(good); len(w) != 0 {
		t.Fatalf("expected no warnings, got %v", w)
	}
	mismatch := CSVRow{BetragNetto: 100, SteuersatzBetrag: 19, Bruttobetrag: 200, Gegenkonto: 6815, Waehrung: "EUR"}
	if !hasWarn(InvoiceWarnings(mismatch), "Brutto") {
		t.Error("expected a gross-mismatch warning")
	}
	noAccount := CSVRow{BetragNetto: 100, SteuersatzBetrag: 19, Bruttobetrag: 119, Gegenkonto: 0, Waehrung: "EUR"}
	if !hasWarn(InvoiceWarnings(noAccount), "Gegenkonto") {
		t.Error("expected a missing-account warning")
	}
	fxNoRate := CSVRow{BetragNetto: 100, SteuersatzBetrag: 19, Bruttobetrag: 119, Gegenkonto: 6815, Waehrung: "USD", Wechselkurs: 0}
	if !hasWarn(InvoiceWarnings(fxNoRate), "Wechselkurs") {
		t.Error("expected a foreign-without-rate warning")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run TestInvoiceWarnings`
Expected: FAIL (undefined InvoiceWarnings).

- [ ] **Step 3: Implement**

Create `internal/core/warnings.go`:

```go
package core

import "math"

// InvoiceWarnings returns advisory (non-blocking) plausibility warnings for an
// invoice row.
func InvoiceWarnings(row CSVRow) []string {
	var w []string
	if row.Bruttobetrag > 0 {
		expected := row.BetragNetto + row.SteuersatzBetrag + row.Trinkgeld
		if math.Abs(row.Bruttobetrag-expected) > 0.02 {
			w = append(w, "Brutto stimmt nicht mit Netto + MwSt + Trinkgeld überein")
		}
	}
	if row.Gegenkonto == 0 {
		w = append(w, "Kein Gegenkonto gewählt")
	}
	if row.Waehrung != "" && row.Waehrung != "EUR" && row.Wechselkurs <= 0 {
		w = append(w, "Fremdwährung ohne Wechselkurs")
	}
	return w
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run TestInvoiceWarnings && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/core/warnings.go internal/core/warnings_test.go
git commit -m "Add InvoiceWarnings: advisory plausibility checks"
```

---

### Task 4: Show warnings on save in the confirmation modal

**Files:**
- Modify: `internal/ui/invoicemodal.go`
- Test: none (UI; covered by build + manual). 

**Interfaces:**
- Consumes: `core.InvoiceWarnings`, the save handler (where the `core.CSVRow`/`Meta` is assembled).
- Produces: on save, if the assembled row has warnings, a confirm dialog lists them and the save proceeds only on "Trotzdem speichern".

- [ ] **Step 1: Insert the warning gate in the save handler**

In `internal/ui/invoicemodal.go`, find the save button handler (`saveBtn.OnTapped` / the place that calls `a.saveInvoice(...)`). Assemble a `core.CSVRow` (or reuse the values already gathered) reflecting the current form: `BetragNetto` = `ed.SumNetto()`-equivalent (use `core.SumNetto(ed.Lines())`), `SteuersatzBetrag` = `core.SumMwSt(ed.Lines())`, `Bruttobetrag` = `ed.Brutto()`, `Trinkgeld` = `ed.Trinkgeld()`, `Gegenkonto` = `selectedAccount`, `Waehrung` = `core.CurrencyCodeFromOption(currencySelect.Selected)`, `Wechselkurs` = `parseDecimal(kursEntry.Text)`. Then:

```go
	warnings := core.InvoiceWarnings(core.CSVRow{
		BetragNetto:      core.SumNetto(ed.Lines()),
		SteuersatzBetrag: core.SumMwSt(ed.Lines()),
		Bruttobetrag:     ed.Brutto(),
		Trinkgeld:        ed.Trinkgeld(),
		Gegenkonto:       selectedAccount,
		Waehrung:         core.CurrencyCodeFromOption(currencySelect.Selected),
		Wechselkurs:      parseDecimal(kursEntry.Text),
	})
	doSave := func() { /* the existing save body — call a.saveInvoice(...) etc. */ }
	if len(warnings) > 0 {
		msg := a.bundle.T("warnings.intro") + "\n• " + strings.Join(warnings, "\n• ")
		dialog.NewConfirm(a.bundle.T("warnings.title"), msg, func(ok bool) {
			if ok {
				doSave()
			}
		}, confirmWin).Show()
		return
	}
	doSave()
```

Wrap the EXISTING save body (the duplicate-check + `a.saveInvoice(...)` + close) into `doSave` so it runs either directly (no warnings) or after the user confirms. Do not duplicate logic — move it into the closure. Use the modal's actual window variable (e.g. `confirmWin`) for the dialog parent.

Add i18n keys `warnings.title` (de "Plausibilitätswarnungen"/en "Plausibility warnings") + `warnings.intro` (de "Bitte prüfen — trotzdem speichern?"/en "Please review — save anyway?") to both JSONs. Confirm `core` and `strings` and `dialog` are imported.

- [ ] **Step 2: Build + vet + test + manual smoke**

Run: `go build ./... && go vet ./internal/ui/ && go test ./...`
Expected: clean. Validate both JSONs. Smoke: a receipt with no Gegenkonto → save shows the warning dialog; confirming still saves.

- [ ] **Step 3: Commit**

```bash
git add internal/ui/invoicemodal.go assets/i18n/de.json assets/i18n/en.json
git commit -m "Warn on implausible invoice data before saving"
```

---

## Self-Review

- **Spec coverage:** data backup as ZIP (Tasks 1/2 — DB + config + CSVs); plausibility warnings (Tasks 3/4 — gross mismatch, missing account, foreign without rate), advisory + non-blocking. Covered.
- **Placeholder scan:** core tasks fully coded; UI tasks reference concrete anchors (the menu list, the save handler, `ed.*`) with explicit "wrap the existing save body" guidance.
- **Type consistency:** `WriteBackupZip(io.Writer, map[string]string)(int,error)`, `InvoiceWarnings(CSVRow)[]string`, `showBackup()`/the save gate — consistent.
- **Data integrity:** backup is best-effort (skips unreadable, returns count, no silent total loss claim); warnings never block the user; gross tolerance 0.02 matches the app's rounding.
- **Out of scope:** more booking categories (§13b/Reisekosten/Geschenke/Skonto) — proposed as a separate follow-up phase; the E1 manual booking editor already covers §13b by hand. Warnings in the edit dialog (this phase does the confirmation modal) can mirror later.
