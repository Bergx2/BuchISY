# Beleg öffnen Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Die Beleg-Datei einer Rechnung lässt sich aus der Haupttabelle (per 👁-Symbol) und aus „Rechnung bearbeiten" (per Button) im Standardprogramm öffnen.

**Architecture:** Ein gemeinsamer Helfer `openFileInOS` öffnet eine Datei über `OpenURL` (wie der bestehende „Original öffnen"-Knopf). Die Tabelle bekommt eine dritte Aktionsspalte; das Bearbeiten-Fenster einen Button.

**Tech Stack:** Go 1.25, Fyne v2.6.3.

**Hinweis zu Commits:** Git-Commits sind in diesem Plan bewusst ausgelassen. Jede Aufgabe endet mit `go build`/`go vet`/`go test`.

---

### Task 1: `openFileInOS`-Helfer + Prüf-Dialog umstellen

**Files:**
- Modify: `internal/ui/openfolder.go`
- Modify: `internal/ui/invoicemodal.go`

- [ ] **Step 1: Add the helper**

In `internal/ui/openfolder.go`, the import block currently is:

```go
import (
	"os/exec"
	"runtime"
)
```

Replace it with:

```go
import (
	"net/url"
	"os/exec"
	"runtime"

	"fyne.io/fyne/v2/storage"
)
```

At the end of `internal/ui/openfolder.go`, append:

```go
// openFileInOS opens a file in the operating system's default application.
func (a *App) openFileInOS(path string) {
	parsed, err := url.Parse(storage.NewFileURI(path).String())
	if err != nil {
		a.logger.Warn("Failed to parse file URI: %v", err)
		a.showError(
			a.bundle.T("error.processing.title"),
			a.bundle.T("error.openOriginal", err.Error()),
		)
		return
	}
	if err := a.app.OpenURL(parsed); err != nil {
		a.logger.Warn("Failed to open file: %v", err)
		a.showError(
			a.bundle.T("error.processing.title"),
			a.bundle.T("error.openOriginal", err.Error()),
		)
	}
}
```

- [ ] **Step 2: Use the helper in the confirmation dialog**

In `internal/ui/invoicemodal.go`, the `openOriginalBtn` is currently:

```go
	openOriginalBtn := widget.NewButton(a.bundle.T("modal.openOriginal"), func() {
		fileURI := storage.NewFileURI(originalPath)
		parsed, err := url.Parse(fileURI.String())
		if err != nil {
			a.logger.Warn("Failed to parse file URI: %v", err)
			a.showError(
				a.bundle.T("error.processing.title"),
				a.bundle.T("error.openOriginal", err.Error()),
			)
			return
		}

		if err := a.app.OpenURL(parsed); err != nil {
			a.logger.Warn("Failed to open original PDF: %v", err)
			a.showError(
				a.bundle.T("error.processing.title"),
				a.bundle.T("error.openOriginal", err.Error()),
			)
		}
	})
```

Replace it with:

```go
	openOriginalBtn := widget.NewButton(a.bundle.T("modal.openOriginal"), func() {
		a.openFileInOS(originalPath)
	})
```

- [ ] **Step 3: Build and vet**

Run: `go build ./... && go vet ./...`
Expected: PASS. If the build reports `"net/url"` or `"fyne.io/fyne/v2/storage"` as now-unused imports in `invoicemodal.go`, remove those import lines from `invoicemodal.go` (they moved to `openfolder.go`). Re-run `go build ./...` until clean.

---

### Task 2: Dritte Aktionsspalte „👁" in der Tabelle

**Files:**
- Modify: `internal/ui/table.go`

- [ ] **Step 1: Account for a third action column in the cell count**

In `internal/ui/table.go`, change:

```go
			return len(it.filtered), len(it.columnOrder) + 2 // +2 for edit and delete columns
```

to:

```go
			return len(it.filtered), len(it.columnOrder) + 3 // +3 for edit, delete, open columns
```

- [ ] **Step 2: Render the open-file button column**

In `internal/ui/table.go`, the cell-update closure currently transitions from the delete column to the data columns like this:

```go
				} else {
					// Regular text columns (shift by -2 since edit and delete are first)
					btn.Hide()
					hoverLabel.Show()
					hoverLabel.TextStyle.Bold = false

					colIndex := id.Col - 2
```

Replace that with (inserts the open column, shifts data columns by -3):

```go
				} else if id.Col == 2 {
					// Open-file button column (THIRD column)
					hoverLabel.Hide()
					btn.SetText("👁")
					btn.Importance = widget.MediumImportance
					btn.Show()

					dataRow := id.Row
					btn.OnTapped = func() {
						if dataRow >= 0 && dataRow < len(it.filtered) && it.app != nil {
							row := it.filtered[dataRow]
							monthFolder := it.app.storageManager.GetMonthFolder(it.app.currentYear, it.app.currentMonth)
							it.app.openFileInOS(core.InvoiceFilePath(monthFolder, row))
						}
					}
				} else {
					// Regular text columns (shift by -3 since edit, delete, open are first)
					btn.Hide()
					hoverLabel.Show()
					hoverLabel.TextStyle.Bold = false

					colIndex := id.Col - 3
```

- [ ] **Step 3: Add the header for the open column**

In `internal/ui/table.go`, the `UpdateHeader` switch is currently:

```go
		switch id.Col {
		case 0:
			label.Alignment = fyne.TextAlignCenter
			label.SetText("✏️")
		case 1:
			label.Alignment = fyne.TextAlignCenter
			label.SetText("🗑")
		default:
			label.Alignment = fyne.TextAlignLeading
			colIndex := id.Col - 2
```

Replace it with:

```go
		switch id.Col {
		case 0:
			label.Alignment = fyne.TextAlignCenter
			label.SetText("✏️")
		case 1:
			label.Alignment = fyne.TextAlignCenter
			label.SetText("🗑")
		case 2:
			label.Alignment = fyne.TextAlignCenter
			label.SetText("👁")
		default:
			label.Alignment = fyne.TextAlignLeading
			colIndex := id.Col - 3
```

- [ ] **Step 4: Shift the selected-column index**

In `internal/ui/table.go`, `OnSelected` currently has:

```go
			colIdx := id.Col - 2 // columns 0,1 are the edit/delete actions
```

Change it to:

```go
			colIdx := id.Col - 3 // columns 0,1,2 are the edit/delete/open actions
```

- [ ] **Step 5: Set the open column's width**

In `internal/ui/table.go`, `applyColumnWidths` currently is:

```go
	it.table.SetColumnWidth(0, 50) // Edit button column
	it.table.SetColumnWidth(1, 50) // Delete button column
	for idx, colID := range it.columnOrder {
		width, ok := columnWidthMap[colID]
		if !ok {
			width = 140
		}
		it.table.SetColumnWidth(idx+2, width) // +2 for edit and delete columns
	}
```

Replace it with:

```go
	it.table.SetColumnWidth(0, 50) // Edit button column
	it.table.SetColumnWidth(1, 50) // Delete button column
	it.table.SetColumnWidth(2, 50) // Open button column
	for idx, colID := range it.columnOrder {
		width, ok := columnWidthMap[colID]
		if !ok {
			width = 140
		}
		it.table.SetColumnWidth(idx+3, width) // +3 for edit, delete, open columns
	}
```

- [ ] **Step 6: Build, vet, test**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS. (`core` is already imported in `table.go`; `it.app.storageManager` / `currentYear` / `currentMonth` are existing `App` fields.)

---

### Task 3: Button „Beleg öffnen" in „Rechnung bearbeiten"

**Files:**
- Modify: `internal/ui/tableedit.go`

- [ ] **Step 1: Add the button and place it next to the "Datei:" label**

In `internal/ui/tableedit.go`, `showEditDialog` currently creates the cancel/save buttons and then the form:

```go
	cancelBtn := widget.NewButton(a.bundle.T("btn.cancel"), func() {
		editWin.Close()
	})
	saveBtn := widget.NewButton(a.bundle.T("btn.save"), nil)
	saveBtn.Importance = widget.HighImportance

	form := container.NewVBox(
		container.NewBorder(nil, nil,
			widget.NewLabel("Datei: "+row.Dateiname),
			container.NewHBox(cancelBtn, saveBtn)),
		widget.NewSeparator(),
```

Replace that with (adds `openBelegBtn` and puts it next to the "Datei:" label):

```go
	cancelBtn := widget.NewButton(a.bundle.T("btn.cancel"), func() {
		editWin.Close()
	})
	saveBtn := widget.NewButton(a.bundle.T("btn.save"), nil)
	saveBtn.Importance = widget.HighImportance

	openBelegBtn := widget.NewButton("Beleg öffnen", func() {
		a.openFileInOS(originalPath)
	})
	openBelegBtn.Importance = widget.LowImportance

	form := container.NewVBox(
		container.NewBorder(nil, nil,
			container.NewHBox(widget.NewLabel("Datei: "+row.Dateiname), openBelegBtn),
			container.NewHBox(cancelBtn, saveBtn)),
		widget.NewSeparator(),
```

- [ ] **Step 2: Build, vet, test**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS — `openFileInOS` is in package `ui`; `originalPath` is already defined in `showEditDialog`.

---

### Task 4: Build, Paketierung, Auslieferung

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

1. Haupttabelle: jede Zeile hat eine dritte Aktionsspalte 👁; Klick öffnet den Beleg im Standardprogramm — auch für Belege in `Bar/` / `Ausgangsrechnungen/`.
2. „Rechnung bearbeiten": der Button „Beleg öffnen" neben „Datei: …" öffnet die Datei.
3. „Rechnungsdaten prüfen": „Original öffnen" funktioniert weiterhin.
4. Eine Zeile, deren Datei fehlt → Fehlermeldung statt Absturz.

---

## Self-Review

**Spec coverage:**
- `openFileInOS`-Helfer (`OpenURL`-Mechanismus) → Task 1 Step 1; Prüf-Dialog auf den Helfer umgestellt → Task 1 Step 2.
- Dritte Aktionsspalte 👁 in der Tabelle, Pfad über `core.InvoiceFilePath` → Task 2.
- Button „Beleg öffnen" in „Rechnung bearbeiten" → Task 3.
- Edge Case „Datei fehlt" → `openFileInOS` zeigt `a.showError` (Task 1 Step 1).

**Placeholder scan:** Keine TBD/TODO; alle Code-Schritte enthalten vollständigen Code. Task 1 Step 3 enthält eine konditionale Import-Bereinigung — der konkrete Schritt (unbenutzte Imports entfernen, bis `go build` sauber ist) ist eindeutig.

**Type consistency:** `(a *App) openFileInOS(path string)` (Task 1) wird in Task 2 (`it.app.openFileInOS`) und Task 3 (`a.openFileInOS`) genutzt — alle im Paket `ui`. `core.InvoiceFilePath(string, core.CSVRow) string` (bestehend) wird in Task 2 verwendet; `it.app.storageManager.GetMonthFolder`, `it.app.currentYear`, `it.app.currentMonth` sind bestehende Felder. Die Spaltenverschiebung von `+2`/`-2` auf `+3`/`-3` ist in Task 2 an allen fünf Stellen (Zell-Anzahl, Zell-Update, Header, OnSelected, Spaltenbreiten) konsistent durchgezogen.
