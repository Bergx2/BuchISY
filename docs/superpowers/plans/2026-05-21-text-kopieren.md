# Rechtsklick „Kopieren" Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Anzeige-Texte und Tabellenzellen bekommen ein Rechtsklick-Kontextmenü „Kopieren", das den Text in die Zwischenablage legt.

**Architecture:** Ein neues kleines Widget `copyableLabel` (bettet `widget.Label` ein und implementiert `fyne.SecondaryTappable`) ersetzt die wertetragenden Anzeige-Labels im Fenster und in den Einstellungen. Das bestehende Rechtsklick-Menü der Rechnungstabelle wird um „Zelle kopieren" / „Zeile kopieren" erweitert. Kopiert wird über `fyne.CurrentApp().Clipboard()`.

**Tech Stack:** Go 1.25, Fyne v2.6.3.

**Hinweis zu Commits:** Git-Commits sind in diesem Plan bewusst ausgelassen — Auslieferung per Build + Kopie der `.exe`. Jede Aufgabe endet mit `go build`/`go vet`/`go test` als Verifikation.

---

### Task 1: i18n-Schlüssel für die Kopier-Menüeinträge

**Files:**
- Modify: `assets/i18n/de.json`
- Modify: `assets/i18n/en.json`

- [ ] **Step 1: Add German keys**

In `assets/i18n/de.json` gibt es die Zeile `"table.delete": "Datei löschen",`. Füge direkt danach ein:

```json
  "menu.copy": "Kopieren",
  "table.copyCell": "Zelle kopieren",
  "table.copyRow": "Zeile kopieren",
```

- [ ] **Step 2: Add English keys**

In `assets/i18n/en.json` die Zeile mit `"table.delete"` suchen und direkt danach einfügen:

```json
  "menu.copy": "Copy",
  "table.copyCell": "Copy cell",
  "table.copyRow": "Copy row",
```

- [ ] **Step 3: Verify JSON validity**

Run: `go build ./...`
Expected: PASS. (Die i18n-Dateien werden zur Laufzeit geladen; kaputtes JSON fällt erst dann auf — daher mindestens optisch prüfen, dass kein abschließendes Komma am Ende des Objekts steht und beide neuen Blöcke innerhalb der geschweiften Klammern liegen.)

---

### Task 2: `joinRowValues`-Hilfsfunktion (TDD)

**Files:**
- Modify: `internal/ui/table.go`
- Test: `internal/ui/table_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/ui/table_test.go`:

```go
package ui

import "testing"

func TestJoinRowValues(t *testing.T) {
	if got := joinRowValues([]string{"AWS", "37.64", "EUR"}); got != "AWS\t37.64\tEUR" {
		t.Errorf("joinRowValues = %q, want tab-joined", got)
	}
	if got := joinRowValues(nil); got != "" {
		t.Errorf("joinRowValues(nil) = %q, want empty", got)
	}
	if got := joinRowValues([]string{"only"}); got != "only" {
		t.Errorf("joinRowValues single = %q", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ui/ -run TestJoinRowValues -v`
Expected: FAIL — `undefined: joinRowValues`.

- [ ] **Step 3: Add the helper**

At the end of `internal/ui/table.go`, append:

```go
// joinRowValues joins cell values with a tab, for clipboard copy of a row.
func joinRowValues(values []string) string {
	return strings.Join(values, "\t")
}
```

(`strings` is already imported in `table.go`.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/ui/ -run TestJoinRowValues -v`
Expected: PASS.

---

### Task 3: `copyableLabel`-Widget

**Files:**
- Create: `internal/ui/copyablelabel.go`

- [ ] **Step 1: Write the widget**

Create `internal/ui/copyablelabel.go`:

```go
package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"

	"github.com/bergx2/buchisy/internal/i18n"
)

// copyableLabel is a label with a right-click "Kopieren" context menu that
// copies the label's text to the clipboard. It embeds widget.Label, so
// Text, Wrapping and Alignment stay settable by callers.
type copyableLabel struct {
	widget.Label
	bundle *i18n.Bundle
}

// newCopyableLabel creates a copyable label showing the given text.
func newCopyableLabel(bundle *i18n.Bundle, text string) *copyableLabel {
	l := &copyableLabel{bundle: bundle}
	l.Text = text
	l.ExtendBaseWidget(l)
	return l
}

// TappedSecondary shows the "Kopieren" context menu on right-click.
func (l *copyableLabel) TappedSecondary(e *fyne.PointEvent) {
	canvas := fyne.CurrentApp().Driver().CanvasForObject(l)
	if canvas == nil {
		return
	}
	menu := fyne.NewMenu("",
		fyne.NewMenuItem(l.bundle.T("menu.copy"), func() {
			fyne.CurrentApp().Clipboard().SetContent(l.Text)
		}),
	)
	widget.ShowPopUpMenuAtPosition(menu, canvas, e.AbsolutePosition)
}
```

- [ ] **Step 2: Build and vet**

Run: `go build ./internal/ui/ && go vet ./internal/ui/...`
Expected: PASS. (`newCopyableLabel` is not yet called anywhere — that is fine, it is exercised by Tasks 5. The Go compiler does not flag unused package-level functions.)

---

### Task 4: Rechtsklick „Zelle/Zeile kopieren" in der Tabelle

**Files:**
- Modify: `internal/ui/table.go`

- [ ] **Step 1: Add the `lastSelectedCol` field**

In `internal/ui/table.go`, the `InvoiceTable` struct has the line
`lastSelectedRow int  // Track last selected row for context menu`. Add a field directly after it:

```go
	lastSelectedRow int  // Track last selected row for context menu
	lastSelectedCol int  // Track last selected data-column index (-1 = none)
```

- [ ] **Step 2: Track the selected column in `OnSelected`**

In `internal/ui/table.go`, replace the `OnSelected` handler:

```go
	// Track selected row for right-click context menu
	it.table.OnSelected = func(id widget.TableCellID) {
		if id.Row >= 0 && id.Row < len(it.filtered) {
			it.lastSelectedRow = id.Row
		}
	}
```

with:

```go
	// Track selected cell for the right-click context menu.
	it.table.OnSelected = func(id widget.TableCellID) {
		if id.Row >= 0 && id.Row < len(it.filtered) {
			it.lastSelectedRow = id.Row
			colIdx := id.Col - 2 // columns 0,1 are the edit/delete actions
			if colIdx >= 0 && colIdx < len(it.columnOrder) {
				it.lastSelectedCol = colIdx
			} else {
				it.lastSelectedCol = -1
			}
		}
	}
```

- [ ] **Step 3: Initialize `lastSelectedCol`**

In `internal/ui/table.go`, the `NewInvoiceTable` constructor builds the
`InvoiceTable` literal with `lastSelectedRow: -1,`. Add the new field:

```go
		lastSelectedRow: -1,
		lastSelectedCol: -1,
```

- [ ] **Step 4: Extend the right-click menu**

In `internal/ui/table.go`, replace the body of `rightClickTable.TappedSecondary` —
the current menu construction:

```go
	row := r.table.filtered[r.table.lastSelectedRow]

	// Create context menu
	menu := fyne.NewMenu("",
		fyne.NewMenuItem(r.table.bundle.T("table.delete"), func() {
			if r.table.app != nil {
				r.table.app.showDeleteConfirmation(row)
			}
		}),
	)
```

with:

```go
	it := r.table
	row := it.filtered[it.lastSelectedRow]

	// Create context menu
	menu := fyne.NewMenu("",
		fyne.NewMenuItem(it.bundle.T("table.delete"), func() {
			if it.app != nil {
				it.app.showDeleteConfirmation(row)
			}
		}),
		fyne.NewMenuItem(it.bundle.T("table.copyCell"), func() {
			if it.lastSelectedCol >= 0 && it.lastSelectedCol < len(it.columnOrder) {
				value := it.getCellValue(it.lastSelectedRow, it.columnOrder[it.lastSelectedCol])
				fyne.CurrentApp().Clipboard().SetContent(value)
			}
		}),
		fyne.NewMenuItem(it.bundle.T("table.copyRow"), func() {
			values := make([]string, len(it.columnOrder))
			for i, colID := range it.columnOrder {
				values[i] = it.getCellValue(it.lastSelectedRow, colID)
			}
			fyne.CurrentApp().Clipboard().SetContent(joinRowValues(values))
		}),
	)
```

(The early-return guard `if r.table.lastSelectedRow < 0 || r.table.lastSelectedRow >= len(r.table.filtered)` above this block stays unchanged.)

- [ ] **Step 5: Build, vet, test**

Run: `go build ./... && go vet ./internal/ui/... && go test ./internal/ui/ -run TestJoinRowValues`
Expected: PASS — build clean, vet clean, the Task 2 test passes.

---

### Task 5: Kopierbare Anzeige-Labels im Fenster und in den Einstellungen

**Files:**
- Modify: `internal/ui/invoicemodal.go`
- Modify: `internal/ui/settings.go`

- [ ] **Step 1: Filename preview becomes copyable**

In `internal/ui/invoicemodal.go`, find:

```go
	// Filename preview
	filenamePreview := widget.NewLabel("")
	filenamePreview.Wrapping = fyne.TextWrapBreak
```

Replace with:

```go
	// Filename preview
	filenamePreview := newCopyableLabel(a.bundle, "")
	filenamePreview.Wrapping = fyne.TextWrapBreak
```

(`filenamePreview.SetText(...)` calls elsewhere keep working — `copyableLabel`
embeds `widget.Label`.)

- [ ] **Step 2: Attachment label becomes copyable**

In `internal/ui/invoicemodal.go`, find:

```go
		attLabel := widget.NewLabel(fmt.Sprintf(
			"Anhänge (%d): %s", len(attachments), strings.Join(names, ", "),
		))
		attLabel.Wrapping = fyne.TextWrapWord
```

Replace with:

```go
		attLabel := newCopyableLabel(a.bundle, fmt.Sprintf(
			"Anhänge (%d): %s", len(attachments), strings.Join(names, ", "),
		))
		attLabel.Wrapping = fyne.TextWrapWord
```

- [ ] **Step 3: Settings hint labels become copyable**

In `internal/ui/settings.go`, replace each of the five hint-label
declarations. Find and replace one-for-one:

`templateHelp := widget.NewLabel(a.bundle.T("settings.templateHelp"))`
→ `templateHelp := newCopyableLabel(a.bundle, a.bundle.T("settings.templateHelp"))`

`accountsNote := widget.NewLabel(a.bundle.T("settings.accountsNote"))`
→ `accountsNote := newCopyableLabel(a.bundle, a.bundle.T("settings.accountsNote"))`

`bankAccountsNote := widget.NewLabel("Verwalten Sie Ihre Bankkonten hier.")`
→ `bankAccountsNote := newCopyableLabel(a.bundle, "Verwalten Sie Ihre Bankkonten hier.")`

`debugHint := widget.NewLabel(a.bundle.T("settings.debugMode.hint"))`
→ `debugHint := newCopyableLabel(a.bundle, a.bundle.T("settings.debugMode.hint"))`

`columnHint := widget.NewLabel(a.bundle.T("settings.columns.hint"))`
→ `columnHint := newCopyableLabel(a.bundle, a.bundle.T("settings.columns.hint"))`

The `.Wrapping = fyne.TextWrapWord` line that follows each of these stays
unchanged (the field is inherited from the embedded `widget.Label`).

- [ ] **Step 4: Build, vet, test**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS — build and vet clean; `internal/core` and `internal/ui`
tests pass; other packages report `no test files`.

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

Terminate any running `BuchISY` process (PowerShell `wmic process where "ProcessId=<id>" call terminate` per running PID, as established in this session).

- [ ] **Step 4: Deploy and restart**

Copy `cmd/buchisy/BuchISY.exe` over `C:\Users\istok\Desktop\BuchISY.exe`,
then launch `C:\Users\istok\Desktop\BuchISY.exe` with working directory
`C:\Users\istok\Desktop`.

- [ ] **Step 5: Manual smoke test**

1. In the invoice table, left-click a data cell, then right-click → the menu
   shows „Löschen", „Zelle kopieren", „Zeile kopieren"; copying and pasting
   elsewhere yields the cell value resp. the tab-joined row.
2. In „Rechnungsdaten prüfen", right-click the filename preview → „Kopieren"
   copies the previewed filename.
3. In the settings sub-pages, right-click a hint text → „Kopieren" works.

---

## Self-Review

**Spec coverage:**
- Wiederverwendbares `copyableLabel` mit Rechtsklick→Kopieren → Task 3.
- Fenster: Dateiname-Vorschau + Anhänge-Liste kopierbar → Task 5 Steps 1-2.
- Einstellungen: Hinweis-Labels kopierbar → Task 5 Step 3.
- Tabelle: „Zelle kopieren" + „Zeile kopieren" im Rechtsklick-Menü → Task 4.
- `lastSelectedCol` für die selektierte Zelle → Task 4 Steps 1-3.
- `joinRowValues`-Hilfsfunktion + Unit-Test → Task 2.
- i18n-Schlüssel → Task 1.
- Clipboard via `fyne.CurrentApp().Clipboard()` → Tasks 3 & 4.

**Placeholder scan:** Keine TBD/TODO; alle Code-Schritte enthalten vollständigen Code.

**Type consistency:** `newCopyableLabel(bundle *i18n.Bundle, text string) *copyableLabel` (Task 3) wird in Task 5 mit genau dieser Signatur aufgerufen. `joinRowValues([]string) string` (Task 2) wird in Task 4 Step 4 verwendet. `lastSelectedCol int` (Task 4 Step 1) wird in Steps 2-4 gesetzt/gelesen. `getCellValue(row int, colID string)` ist eine bestehende Methode von `InvoiceTable`. i18n-Schlüssel `menu.copy`/`table.copyCell`/`table.copyRow` (Task 1) werden in Tasks 3 & 4 per `bundle.T(...)` verwendet.
