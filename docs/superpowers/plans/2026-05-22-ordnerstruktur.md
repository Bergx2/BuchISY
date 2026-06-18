# Ordnerstruktur — Jahresordner + Kategorie-Unterordner Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Monatsordner unter einen Jahresordner legen, Bar-Belege und Ausgangsrechnungen in Kategorie-Unterordner einsortieren, und vorhandene Daten per Startup-Migration in die neue Struktur überführen.

**Architecture:** `GetMonthFolder` bekommt eine Jahresebene; ein `InvoiceFilePath`-Helfer löst Pfade inklusive Unterordner auf. Eine neue CSV-Spalte `Unterordner` hält die Kategorie je Rechnung. `saveInvoice`/`updateInvoice` legen die Datei im passenden Unterordner ab. Zwei idempotente Startup-Migrationen ziehen Bestandsdaten um.

**Tech Stack:** Go 1.25, Fyne v2.6.3, Standard-`testing`.

**Hinweis zu Commits:** Git-Commits sind in diesem Plan bewusst ausgelassen. Jede Aufgabe endet mit `go build`/`go vet`/`go test`.

---

### Task 1: Jahresebene in `GetMonthFolder` + `InvoiceFilePath`-Helfer (TDD)

**Files:**
- Modify: `internal/core/storage.go`
- Test: `internal/core/storage_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/core/storage_test.go`:

```go
package core

import (
	"path/filepath"
	"testing"
	"time"
)

func TestGetMonthFolderWithYear(t *testing.T) {
	s := Settings{StorageRoot: filepath.Join("X", "root"), UseMonthSubfolders: true}
	sm := NewStorageManager(&s)
	got := sm.GetMonthFolder(2026, time.April)
	want := filepath.Join("X", "root", "2026", "2026-04")
	if got != want {
		t.Errorf("GetMonthFolder = %q, want %q", got, want)
	}

	s.UseMonthSubfolders = false
	if got := sm.GetMonthFolder(2026, time.April); got != s.StorageRoot {
		t.Errorf("without subfolders: GetMonthFolder = %q, want %q", got, s.StorageRoot)
	}
}

func TestInvoiceFilePath(t *testing.T) {
	month := filepath.Join("X", "root", "2026", "2026-04")
	plain := InvoiceFilePath(month, CSVRow{Dateiname: "a.pdf"})
	if plain != filepath.Join(month, "a.pdf") {
		t.Errorf("plain = %q", plain)
	}
	bar := InvoiceFilePath(month, CSVRow{Dateiname: "a.pdf", Unterordner: "Bar"})
	if bar != filepath.Join(month, "Bar", "a.pdf") {
		t.Errorf("bar = %q", bar)
	}
}
```

(`CSVRow.Unterordner` does not exist yet — it is added in Task 2. For this task, the test references it; add the field minimally now if needed. To keep Task 1 self-contained, add `Unterordner string` to the `CSVRow` struct in `internal/core/types.go` as part of Step 3 — Task 2 then adds the CSV plumbing.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run "TestGetMonthFolderWithYear|TestInvoiceFilePath" -v`
Expected: FAIL — `GetMonthFolder` lacks the year level; `InvoiceFilePath` undefined; `CSVRow.Unterordner` undefined.

- [ ] **Step 3: Implement**

In `internal/core/types.go`, the `CSVRow` struct currently ends:

```go
	Teilzahlung       bool
	HatAnhaenge       bool
	AnzahlAnhaenge    int
}
```

Add the `Unterordner` field:

```go
	Teilzahlung       bool
	HatAnhaenge       bool
	AnzahlAnhaenge    int
	Unterordner       string // "" | "Bar" | "Ausgangsrechnungen"
}
```

In `internal/core/storage.go`, `GetMonthFolder` currently is:

```go
func (sm *StorageManager) GetMonthFolder(year int, month time.Month) string {
	if !sm.settings.UseMonthSubfolders {
		return sm.settings.StorageRoot
	}

	folderName := fmt.Sprintf("%04d-%02d", year, month)
	return filepath.Join(sm.settings.StorageRoot, folderName)
}
```

Replace it with (adds the year level):

```go
func (sm *StorageManager) GetMonthFolder(year int, month time.Month) string {
	if !sm.settings.UseMonthSubfolders {
		return sm.settings.StorageRoot
	}

	yearName := fmt.Sprintf("%04d", year)
	monthName := fmt.Sprintf("%04d-%02d", year, month)
	return filepath.Join(sm.settings.StorageRoot, yearName, monthName)
}

// InvoiceFilePath returns the full path of an invoice's main file inside
// its month folder, honouring the row's category subfolder.
func InvoiceFilePath(monthFolder string, row CSVRow) string {
	if row.Unterordner == "" {
		return filepath.Join(monthFolder, row.Dateiname)
	}
	return filepath.Join(monthFolder, row.Unterordner, row.Dateiname)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run "TestGetMonthFolderWithYear|TestInvoiceFilePath" -v`
Expected: PASS.

- [ ] **Step 5: Build and vet**

Run: `go build ./... && go vet ./...`
Expected: PASS — the deeper folder path and the new (still-unwired) `Unterordner` field compile cleanly.

---

### Task 2: CSV-Spalte `Unterordner` (TDD)

**Files:**
- Modify: `internal/core/csvrepo.go`
- Modify: `assets/i18n/de.json`, `assets/i18n/en.json`
- Test: `internal/core/csvrepo_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/core/csvrepo_test.go`:

```go
package core

import (
	"path/filepath"
	"testing"
)

func TestCSVUnterordnerRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invoices.csv")
	repo := NewCSVRepository()

	if err := repo.Append(path, CSVRow{Dateiname: "a.pdf", Unterordner: "Bar"}); err != nil {
		t.Fatalf("append: %v", err)
	}
	rows, err := repo.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(rows) != 1 || rows[0].Unterordner != "Bar" {
		t.Fatalf("round-trip mismatch: %+v", rows)
	}
}

func TestSetColumnOrderKeepsNewColumns(t *testing.T) {
	repo := NewCSVRepository()
	// A legacy saved order without the new column.
	repo.SetColumnOrder([]string{"Dateiname", "Firmenname"})
	header := repo.GetHeader()
	found := false
	for _, c := range header {
		if c == "Unterordner" {
			found = true
		}
	}
	if !found {
		t.Errorf("GetHeader() = %v, must include Unterordner even for a legacy column order", header)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run "TestCSVUnterordner|TestSetColumnOrder" -v`
Expected: FAIL — `Unterordner` is neither persisted nor reconciled into the column order.

- [ ] **Step 3: Add the column to the CSV repository**

In `internal/core/csvrepo.go`:

(a) Append `"Unterordner"` to `DefaultCSVColumns` (after `"AnzahlAnhaenge"`).

(b) Add to `ColumnDisplayNames`: `"Unterordner": "Unterordner",`.

(c) Add to `ColumnTranslationKeys`: `"Unterordner": "table.col.unterordner",`.

(d) In `Load`, add to the `CSVRow{...}` literal: `Unterordner: valueForColumn(record, headerMap, "Unterordner"),`.

(e) In `rowToRecord`, add to the `valueMap`: `"Unterordner": row.Unterordner,`.

(f) Replace `SetColumnOrder` so it always keeps every default column:

```go
// SetColumnOrder sets the column order for CSV operations. Any column
// from DefaultCSVColumns missing from order is appended, so a legacy
// saved order still includes columns added in newer versions.
func (r *CSVRepository) SetColumnOrder(order []string) {
	if len(order) == 0 {
		r.columnOrder = append([]string{}, DefaultCSVColumns...)
		return
	}
	result := append([]string{}, order...)
	present := make(map[string]struct{}, len(result))
	for _, c := range result {
		present[c] = struct{}{}
	}
	for _, c := range DefaultCSVColumns {
		if _, ok := present[c]; !ok {
			result = append(result, c)
		}
	}
	r.columnOrder = result
}
```

- [ ] **Step 4: Add the i18n key for the column header**

In `assets/i18n/de.json`, add (next to the other `table.col.*` keys): `"table.col.unterordner": "Unterordner",`.

In `assets/i18n/en.json`, add: `"table.col.unterordner": "Subfolder",`.

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/core/ -run "TestCSVUnterordner|TestSetColumnOrder" -v`
Expected: PASS.

- [ ] **Step 6: Build, vet, full core tests**

Run: `go build ./... && go vet ./internal/core/... && go test ./internal/core/...`
Expected: PASS.

---

### Task 3: Startup-Migrationen

**Files:**
- Modify: `internal/core/storage.go`
- Modify: `internal/ui/app.go`
- Test: `internal/core/storage_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/core/storage_test.go`:

```go
func TestMigrateToYearFolders(t *testing.T) {
	root := t.TempDir()
	monthDir := filepath.Join(root, "2026-04")
	if err := os.MkdirAll(monthDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(monthDir, "invoices.csv"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	s := Settings{StorageRoot: root, UseMonthSubfolders: true}
	sm := NewStorageManager(&s)

	if err := sm.MigrateToYearFolders(func(string) {}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	moved := filepath.Join(root, "2026", "2026-04", "invoices.csv")
	if _, err := os.Stat(moved); err != nil {
		t.Fatalf("expected file at %s: %v", moved, err)
	}
	if _, err := os.Stat(monthDir); !os.IsNotExist(err) {
		t.Errorf("old folder %s should be gone", monthDir)
	}
	// Idempotent: a second run does nothing and does not error.
	if err := sm.MigrateToYearFolders(func(string) {}); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
}
```

Add `"os"` to the test file's imports.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run TestMigrateToYearFolders -v`
Expected: FAIL — `MigrateToYearFolders` undefined.

- [ ] **Step 3: Implement the two migrations**

Append to `internal/core/storage.go` (and add `"regexp"` to its import block):

```go
// monthFolderPattern matches a bare YYYY-MM folder name.
var monthFolderPattern = regexp.MustCompile(`^(\d{4})-(\d{2})$`)

// MigrateToYearFolders moves bare YYYY-MM folders directly under the
// storage root into a YYYY year folder. Idempotent: a second run finds
// nothing to move. warn is called with a message for each skipped folder.
func (sm *StorageManager) MigrateToYearFolders(warn func(string)) error {
	if !sm.settings.UseMonthSubfolders {
		return nil
	}
	root := sm.settings.StorageRoot
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read storage root: %w", err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		m := monthFolderPattern.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		yearDir := filepath.Join(root, m[1])
		if err := os.MkdirAll(yearDir, 0755); err != nil {
			return fmt.Errorf("failed to create year folder %s: %w", yearDir, err)
		}
		target := filepath.Join(yearDir, e.Name())
		if _, err := os.Stat(target); err == nil {
			warn(fmt.Sprintf("Monatsordner %s übersprungen — Ziel existiert bereits", e.Name()))
			continue
		}
		if err := os.Rename(filepath.Join(root, e.Name()), target); err != nil {
			return fmt.Errorf("failed to move %s: %w", e.Name(), err)
		}
	}
	return nil
}

// MigrateCashToBar moves already-filed invoices that are booked to a cash
// account into a Bar/ subfolder and sets their Unterordner. cashAccounts
// is the set of cash-account names. Idempotent: rows with a non-empty
// Unterordner are skipped. warn is called for each unmovable file.
func (sm *StorageManager) MigrateCashToBar(repo *CSVRepository, cashAccounts map[string]struct{}, warn func(string)) error {
	if !sm.settings.UseMonthSubfolders || len(cashAccounts) == 0 {
		return nil
	}
	csvPaths, err := sm.ListAllCSVPaths()
	if err != nil {
		return err
	}
	for _, csvPath := range csvPaths {
		monthFolder := filepath.Dir(csvPath)
		rows, err := repo.Load(csvPath)
		if err != nil {
			warn(fmt.Sprintf("CSV %s übersprungen: %v", csvPath, err))
			continue
		}
		changed := false
		for i := range rows {
			if rows[i].Unterordner != "" {
				continue
			}
			if _, isCash := cashAccounts[rows[i].Bankkonto]; !isCash {
				continue
			}
			src := filepath.Join(monthFolder, rows[i].Dateiname)
			if _, err := os.Stat(src); err != nil {
				warn(fmt.Sprintf("Beleg %s nicht gefunden — übersprungen", src))
				continue
			}
			barDir := filepath.Join(monthFolder, "Bar")
			if err := os.MkdirAll(barDir, 0755); err != nil {
				return fmt.Errorf("failed to create %s: %w", barDir, err)
			}
			if err := os.Rename(src, filepath.Join(barDir, rows[i].Dateiname)); err != nil {
				warn(fmt.Sprintf("Verschieben von %s fehlgeschlagen: %v", src, err))
				continue
			}
			rows[i].Unterordner = "Bar"
			changed = true
		}
		if changed {
			if err := repo.Rewrite(csvPath, rows); err != nil {
				return fmt.Errorf("failed to rewrite %s: %w", csvPath, err)
			}
		}
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run TestMigrateToYearFolders -v`
Expected: PASS.

- [ ] **Step 5: Call the migrations on startup**

In `internal/ui/app.go`, the `New` function has, in sequence:

```go
	csvRepo := core.NewCSVRepository()
	csvRepo.SetColumnOrder(settings.ColumnOrder)
	storageManager := core.NewStorageManager(&settings)
```

Add the migration calls right after `storageManager` is created:

```go
	csvRepo := core.NewCSVRepository()
	csvRepo.SetColumnOrder(settings.ColumnOrder)
	storageManager := core.NewStorageManager(&settings)

	// One-time, idempotent storage migrations.
	warn := func(msg string) { logger.Warn("%s", msg) }
	if err := storageManager.MigrateToYearFolders(warn); err != nil {
		logger.Warn("Year-folder migration failed: %v", err)
	}
	cashAccounts := make(map[string]struct{})
	for _, ba := range settings.BankAccounts {
		if ba.AccountType == core.AccountTypeCash {
			cashAccounts[ba.Name] = struct{}{}
		}
	}
	if err := storageManager.MigrateCashToBar(csvRepo, cashAccounts, warn); err != nil {
		logger.Warn("Bar migration failed: %v", err)
	}
```

- [ ] **Step 6: Build, vet, test**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS.

---

### Task 4: „Ausgangsrechnung"-Häkchen + Einsortierung — „Rechnungsdaten prüfen"

**Files:**
- Modify: `internal/ui/invoicemodal.go`

- [ ] **Step 1: Add a cash-account helper**

In `internal/ui/invoicemodal.go`, add this method (anywhere at file scope):

```go
// invoiceSubfolder determines the category subfolder for an invoice from
// the "Ausgangsrechnung" flag and the chosen bank account.
func (a *App) invoiceSubfolder(bankAccount string, ausgangsrechnung bool) string {
	if ausgangsrechnung {
		return "Ausgangsrechnungen"
	}
	for _, ba := range a.settings.BankAccounts {
		if ba.Name == bankAccount && ba.AccountType == core.AccountTypeCash {
			return "Bar"
		}
	}
	return ""
}
```

- [ ] **Step 2: Add the "Ausgangsrechnung" checkbox**

In `showConfirmationModal`, near where `partialPaymentCheck` is created, add:

```go
	ausgangsrechnungCheck := widget.NewCheck("Ausgangsrechnung", nil)
```

In the `widget.NewForm(...)`, add a form item for it right after the `partialPaymentCheck` item:

```go
			widget.NewFormItem("", partialPaymentCheck),
			widget.NewFormItem("", ausgangsrechnungCheck),
```

- [ ] **Step 3: Thread the flag and route into a subfolder in `saveInvoice`**

`saveInvoice`'s signature currently ends `..., rememberMapping bool, filenameInput string) error`. Add a parameter:

```go
	rememberMapping bool,
	filenameInput string,
	ausgangsrechnung bool,
) error {
```

In `saveInvoice`, after `filename` is computed (the `SanitizeFilename`/`ReplaceExtension` block) and `targetFolder` is set, the code currently does `targetFolder := a.storageManager.GetMonthFolder(a.currentYear, a.currentMonth)`. Just below that line, add:

```go
	unterordner := a.invoiceSubfolder(bankAccount, ausgangsrechnung)
	if unterordner != "" {
		targetFolder = filepath.Join(targetFolder, unterordner)
	}
```

Where the new `CSVRow` (`newRow`) is built, set its `Unterordner`:

```go
	newRow.Unterordner = unterordner
```

(Attachments are moved with `MoveAndRename` into the same `targetFolder`, so they follow into the subfolder automatically.)

- [ ] **Step 4: Pass the checkbox value from the save button**

In `showConfirmationModal`, the `saveBtn.OnTapped` call to `a.saveInvoice(...)` currently ends with `filenameEntry.Text,` as the last argument. Add the checkbox value:

```go
			filenameEntry.Text,
			ausgangsrechnungCheck.Checked,
		)
```

- [ ] **Step 5: Build, vet, test**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS.

---

### Task 5: „Ausgangsrechnung"-Häkchen + Einsortierung + Pfad-Auflösung — „Rechnung bearbeiten"

**Files:**
- Modify: `internal/ui/tableedit.go`
- Modify: `internal/ui/tabledelete.go`
- Modify: `internal/ui/table.go`

- [ ] **Step 1: Resolve the original file path via the subfolder in `showEditDialog`**

In `internal/ui/tableedit.go`, `showEditDialog` currently builds:

```go
	sourceFolder := a.storageManager.GetMonthFolder(a.currentYear, a.currentMonth)
	originalPath := filepath.Join(sourceFolder, row.Dateiname)
```

Change the second line to use the helper:

```go
	sourceFolder := a.storageManager.GetMonthFolder(a.currentYear, a.currentMonth)
	originalPath := core.InvoiceFilePath(sourceFolder, row)
```

- [ ] **Step 2: Add the "Ausgangsrechnung" checkbox**

In `showEditDialog`, near `partialPaymentCheck`, add:

```go
	ausgangsrechnungCheck := widget.NewCheck("Ausgangsrechnung", nil)
	ausgangsrechnungCheck.SetChecked(row.Unterordner == "Ausgangsrechnungen")
```

In the `widget.NewForm(...)`, add it right after the `partialPaymentCheck` form item:

```go
			widget.NewFormItem("", partialPaymentCheck),
			widget.NewFormItem("", ausgangsrechnungCheck),
```

- [ ] **Step 3: Thread the flag into `updateInvoice` and route the file**

In `tableedit.go`, the `saveBtn.OnTapped` call `a.updateInvoice(...)` currently passes `filenameEntry.Text, targetYear, targetMonth,` as its last three arguments. Add the checkbox:

```go
			filenameEntry.Text,
			targetYear,
			targetMonth,
			ausgangsrechnungCheck.Checked,
		)
```

`updateInvoice`'s signature currently ends `..., filenameInput string, targetYear int, targetMonth time.Month) error`. Add:

```go
	filenameInput string,
	targetYear int,
	targetMonth time.Month,
	ausgangsrechnung bool,
) error {
```

In `updateInvoice`, the target folder is currently `targetFolder := a.storageManager.GetMonthFolder(targetYear, targetMonth)`. Right after that line add the subfolder:

```go
	unterordner := a.invoiceSubfolder(bankAccount, ausgangsrechnung)
	if unterordner != "" {
		targetFolder = filepath.Join(targetFolder, unterordner)
	}
```

And where `newRow` is built, set:

```go
	newRow.Unterordner = unterordner
```

The collision/move block in `updateInvoice` computes `finalPath` inside `targetFolder` and moves the file there — it now moves into the subfolder automatically. The `os.MkdirAll(targetFolder, 0755)` already in `updateInvoice` creates the subfolder.

- [ ] **Step 4: Resolve the path in delete and open**

In `internal/ui/tabledelete.go`, `deleteInvoice` builds the invoice file path from the month folder and `row.Dateiname`. Change that path construction to `core.InvoiceFilePath(monthFolder, row)` (where `monthFolder` is the existing month-folder variable in that function).

In `internal/ui/table.go`, the "open file" action builds the path from the month folder and the row's filename. Change it likewise to `core.InvoiceFilePath(<monthFolder>, row)`.

(Read both files to find the exact path-construction lines; each is a single `filepath.Join(<monthFolder>, <row>.Dateiname)` that becomes `core.InvoiceFilePath(<monthFolder>, <row>)`. If `core` is not yet imported in `table.go`, add it.)

- [ ] **Step 5: Build, vet, test**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS — `saveInvoice`/`updateInvoice` have the new `ausgangsrechnung` parameter and both call sites pass it; all invoice file paths resolve through `core.InvoiceFilePath`.

---

### Task 6: Build, Paketierung, Auslieferung

**Files:** none (build/deploy only)

- [ ] **Step 1: Final build + vet + tests**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: all succeed.

- [ ] **Step 2: Package the Windows executable**

Run (from `C:\Users\istok\Desktop\Dev\BuchISY`):
`fyne package -os windows -name BuchISY -src ./cmd/buchisy`
Expected: `cmd/buchisy/BuchISY.exe` produced.

- [ ] **Step 3: Stop the running app**

Terminate any running `BuchISY` process (PowerShell `wmic process where "ProcessId=<id>" call terminate` per running PID).

- [ ] **Step 4: Deploy and restart**

Copy `cmd/buchisy/BuchISY.exe` over `C:\Users\istok\Desktop\BuchISY.exe`, then launch
`C:\Users\istok\Desktop\BuchISY.exe` with working directory `C:\Users\istok\Desktop`.

- [ ] **Step 5: Manual smoke test**

1. Start: vorhandene `JJJJ-MM`-Ordner liegen danach unter `<Ablage>/JJJJ/`; bestehende Bar-Rechnungen liegen in `…/Bar/`.
2. Neue Rechnung auf ein Barkassen-Konto buchen → die PDF landet in `<Jahr>/<Monat>/Bar/`.
3. Rechnung mit gesetztem „Ausgangsrechnung"-Häkchen speichern → PDF in `<Jahr>/<Monat>/Ausgangsrechnungen/`.
4. Solche Rechnungen erscheinen weiter in der Monatstabelle; Öffnen, Bearbeiten, Löschen und Belegvorschau finden die Datei im Unterordner.
5. In „Rechnung bearbeiten" das Häkchen ändern → die Datei wird in den anderen Unterordner verschoben.

---

## Self-Review

**Spec coverage:**
- Jahresordner in `GetMonthFolder` → Task 1; Jahresordner-Migration → Task 3.
- `Unterordner`-Feld + CSV-Spalte (+ `SetColumnOrder`-Abgleich, i18n) → Task 1 (Feld) + Task 2 (CSV).
- `InvoiceFilePath`-Helfer → Task 1; Pfad-Auflösung an allen Stellen → Task 5 Steps 1+4 (showEditDialog, tabledelete, table); `buildDocumentPreview` erhält `originalPath` aus `showEditDialog` → durch Step 1 abgedeckt.
- Einsortierungs-Regel (Ausgangsrechnung vor Bar) → `invoiceSubfolder` (Task 4 Step 1), genutzt von `saveInvoice` (Task 4) und `updateInvoice` (Task 5).
- „Ausgangsrechnung"-Häkchen in beiden Dialogen → Task 4 Step 2, Task 5 Step 2.
- Bar-Migration → Task 3; Aufruf der Migrationen beim Start → Task 3 Step 5.
- Tests: `GetMonthFolder`, `InvoiceFilePath`, CSV-Round-Trip, `SetColumnOrder`, Jahresordner-Migration → Tasks 1-3.

**Placeholder scan:** Keine TBD/TODO; alle Code-Schritte enthalten vollständigen Code. Task 5 Step 4 verweist für die exakten Zeilen auf `tabledelete.go`/`table.go` — die Änderung ist je eine eindeutige `filepath.Join`-Ersetzung; der Implementer liest die Datei und ersetzt sie.

**Type consistency:** `CSVRow.Unterordner string` (Task 1) wird in Task 2 (CSV) und Tasks 4-5 verwendet. `InvoiceFilePath(string, CSVRow) string` (Task 1) wird in Task 5 genutzt. `invoiceSubfolder(string, bool) string` (Task 4 Step 1, Methode auf `*App`) wird in `saveInvoice` und `updateInvoice` aufgerufen — beide im selben Paket `ui`. `MigrateToYearFolders(func(string)) error` und `MigrateCashToBar(*CSVRepository, map[string]struct{}, func(string)) error` (Task 3) werden in `app.go` mit genau diesen Signaturen aufgerufen. `saveInvoice` erhält `ausgangsrechnung bool` als letzten Parameter; `updateInvoice` erhält `ausgangsrechnung bool` als letzten Parameter — beide Aufrufstellen aktualisiert (Task 4 Step 4, Task 5 Step 3).
