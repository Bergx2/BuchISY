# UI/UX Navigation Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the hidden ⋮ overflow menu and look-alike toggle buttons with a Lexware-style workflow sidebar + native menu bar, promote the booking period, make data copyable (Ctrl+C), and apply a light theme pass — so a newcomer can find every feature.

**Architecture:** A new persistent left sidebar (`sidebar.go`) and native main menu (`mainmenu.go`) drive navigation by calling the **existing** `a.show…()` functions unchanged (Phase 1 — no view rewrites). A new **outer shell** in `buildUI()` wraps BOTH the Belege content and the Konten content with `Border(periodHeader, statusBar, sidebar, nil, content)` plus `window.SetMainMenu(...)`. Copy and theme are independent, low-risk passes.

**Tech Stack:** Go 1.25+, Fyne v2.6.3, go-i18n (flat JSON in `assets/i18n/{de,en}.json`), `fyne bundle` for embedding the Inter font.

## Global Constraints

- All user-facing strings via `a.bundle.T("key")`; add keys to BOTH `assets/i18n/de.json` and `assets/i18n/en.json` (flat JSON, no nesting). The key `menu.copy` already exists.
- **Clipboard is `fyne.CurrentApp().Clipboard()`** — NOT `a.window.Clipboard()` (the latter does not exist on the Fyne v2.6.3 `fyne.Window` interface). See `copyablelabel.go:40`.
- **Copy shortcut is `&fyne.ShortcutCopy{}`** registered via `canvas.AddShortcut` — NOT `desktop.CustomShortcut{KeyName: fyne.KeyC, Modifier: fyne.KeyModifierControl}`. Fyne maps Ctrl/Cmd+C to the built-in `fyne.ShortcutCopy`, so a custom-shortcut handler never fires for normal copy. Focused Entry/Table widgets receive the shortcut first, which is the desired behavior.
- Never break period locking, Mandanten isolation, or the audit trail — sidebar/menu items call existing functions verbatim.
- Sidebar/menu entries must map 1:1 to the 25 actions in the old ⋮ menu (`app.go:919–952`) — no feature may become unreachable.
- German is primary; default window size stays 1500×875.
- Run `go build -o build/buchisy ./cmd/buchisy` (or `make dev`) and `go test ./...` green before each commit.

### Verified source facts (do not re-assume — these were checked against the repo)

- `InvoiceTable` fields: `filtered []core.CSVRow` (NOT `rows`), `columnOrder []string`, `selectedRow int` (-1 = none), `app *App`, `window fyne.Window`. (`table.go:126–151`)
- `(it *InvoiceTable) getCellValue(row int, colID string) string` (`table.go:964`); `joinRowValues(values []string) string` (`table.go:1116`).
- `(it *InvoiceTable) RegisterKeyHandler(cv fyne.Canvas)` wires ↑/↓/Enter/Del via `cv.SetOnTypedKey` and no-ops when an Entry is focused. (`table.go:1138`)
- `hoverLabel` constructor is `newHoverLabel(onHover func(string, fyne.Position), onExit func()) *hoverLabel` (`table.go:40`). It embeds `widget.Label`, supports `.SetText`, and already has a right-click "Kopieren" menu (`table.go:102`).
- `copyableLabel`: `newCopyableLabel(bundle *i18n.Bundle, text string) *copyableLabel` (`copyablelabel.go:19`).
- Period selectors `a.yearSelect` / `a.monthSelect` are currently CREATED inside `buildTopBar` (`app.go:871, 890`) — they must be extracted before any shell rewrite reuses them.
- `buildUI` returns EARLY for `a.viewMode == "konten"` (`app.go:663–666`) returning `a.buildKontenUI()` — the shell must wrap BOTH branches.
- Lock state is `a.currentMonthLocked bool`; status bar already renders `period.locked.indicator` (`app.go:800`).
- Legend is `a.invoiceTable.LegendButton()` (`app.go:720`) — there is no `a.showLegend()`.
- File-import picker is `a.showFilesPicker(func(paths []string){ a.enqueueSubmissions(paths) })` (`app.go:923`).
- `theme.SettingsIcon()`, `theme.NavigateBackIcon()`, `theme.NavigateNextIcon()` are the icons used today.

---

### Task 1: Ctrl+C copies the selected invoice-table row

**Files:**
- Modify: `internal/ui/table.go` (add `CopySelectedRow()`; register `&fyne.ShortcutCopy{}` inside `RegisterKeyHandler`)
- Test: `internal/ui/table_test.go`

**Interfaces:**
- Consumes: `it.getCellValue(row, colID)`, `joinRowValues([]string)`, `it.filtered`, `it.columnOrder`, `it.selectedRow`.
- Produces: `func (it *InvoiceTable) CopySelectedRow() string` — tab-joined values of the selected row, or "" if none.

- [ ] **Step 1: Write the failing test**

```go
// internal/ui/table_test.go (add)
func TestCopySelectedRow_EmptyWhenNoSelection(t *testing.T) {
	it := &InvoiceTable{selectedRow: -1}
	if got := it.CopySelectedRow(); got != "" {
		t.Fatalf("expected empty string with no selection, got %q", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ui/ -run TestCopySelectedRow_EmptyWhenNoSelection -v`
Expected: FAIL — `it.CopySelectedRow undefined`.

- [ ] **Step 3: Implement `CopySelectedRow`** (real field names)

In `internal/ui/table.go`:

```go
// CopySelectedRow returns the currently selected row's values joined by tab,
// or "" if nothing is selected. Backs the Ctrl+C (ShortcutCopy) handler.
func (it *InvoiceTable) CopySelectedRow() string {
	if it.selectedRow < 0 || it.selectedRow >= len(it.filtered) {
		return ""
	}
	vals := make([]string, 0, len(it.columnOrder))
	for _, colID := range it.columnOrder {
		vals = append(vals, it.getCellValue(it.selectedRow, colID))
	}
	return joinRowValues(vals)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/ui/ -run TestCopySelectedRow_EmptyWhenNoSelection -v`
Expected: PASS.

- [ ] **Step 5: Register the copy shortcut on the table's canvas**

In `RegisterKeyHandler` (`table.go:1138`), after the `SetOnTypedKey(...)` block, add:

```go
	// Ctrl/Cmd+C copies the selected row. Fyne routes copy to the focused
	// widget first (Entry gets Entry-copy); the table is not Shortcutable,
	// so it falls through to this canvas handler.
	// IMPORTANT: canvas.AddShortcut persists on the window canvas across
	// SetContent, so it would also fire in Konten mode (no invoice table).
	// Gate on Belege mode + a live table to avoid copying stale rows.
	cv.AddShortcut(&fyne.ShortcutCopy{}, func(fyne.Shortcut) {
		if it.app == nil || it.app.viewMode != "" || it.app.invoiceTable != it {
			return
		}
		if s := it.CopySelectedRow(); s != "" {
			fyne.CurrentApp().Clipboard().SetContent(s)
		}
	})
```

- [ ] **Step 6: Build & manual check**

Run: `go build -o build/buchisy ./cmd/buchisy`
Expected: builds. Manually: select a table row (click), press Ctrl+C, paste → row values appear (tab-separated).

- [ ] **Step 7: Commit**

```bash
git add internal/ui/table.go internal/ui/table_test.go
git commit -m "feat(ui): Ctrl+C copies the selected invoice-table row"
```

---

### Task 2: Make remaining report/recon cells copyable (incl. SuSa)

**Files:**
- Modify: `internal/ui/susaview.go` (SuSa table cells → copyable)
- Modify: `internal/ui/oposview.go` (table cells → copyable)
- Modify: `internal/ui/controllingview.go` (table cells + totals → copyable)
- Modify: `internal/ui/auditview.go` (table cells → copyable)
- Modify: `internal/ui/belegabgleichview.go` (plain value `widget.NewLabel` → `newCopyableLabel`)
- Modify: `internal/ui/erloesabgleichview.go` (plain value `widget.NewLabel` → `newCopyableLabel`)

**Interfaces:**
- Consumes: `newCopyableLabel(a.bundle, text)`, `newHoverLabel(nil, nil)` + `.SetText`.
- Produces: behavioral change only.

- [ ] **Step 1: Convert `widget.NewTable` cell templates (SuSa, OPOS, Controlling, Audit)**

For each table whose cell template is `func() fyne.CanvasObject { return widget.NewLabel("") }`, swap to a copyable `hoverLabel`. Use the REAL constructor `newHoverLabel(nil, nil)` (no tooltip needed) and reset recycled state in the update callback:

```go
// template (was widget.NewLabel(""))
func() fyne.CanvasObject { return newHoverLabel(nil, nil) },
// update (was o.(*widget.Label).SetText(val))
func(id widget.TableCellID, o fyne.CanvasObject) {
	hl := o.(*hoverLabel)
	// CRITICAL: table cells are RECYCLED. Reset every style field the old
	// widget.NewLabel update used, or bold/alignment leaks across rows.
	// SuSa (susaview.go:47) and OPOS (oposview.go:45) reset TextStyle AND
	// Alignment on every update — preserve that exactly.
	hl.tooltip = ""
	hl.TextStyle = fyne.TextStyle{ /* Bold: true ONLY where the original did */ }
	hl.Alignment = fyne.TextAlignLeading // or the per-column alignment the original used
	hl.SetText(/* existing value expression unchanged */)
},
```

> `hoverLabel` already provides right-click "Kopieren" (`table.go:102`). Match the original cell's TextStyle/Alignment per row/column exactly — do not assume a single style for the whole table.

- [ ] **Step 2: Convert standalone value labels in the abgleich views + Controlling totals**

In `belegabgleichview.go`, `erloesabgleichview.go`, and the Controlling total labels, replace each **value-displaying** `widget.NewLabel(x)` with `newCopyableLabel(a.bundle, x)`. Do NOT convert section headers, buttons, or icons. Example:

```go
// before
amountLbl := widget.NewLabel(formatEUR(line.Amount))
// after
amountLbl := newCopyableLabel(a.bundle, formatEUR(line.Amount))
```

- [ ] **Step 3: Build**

Run: `go build -o build/buchisy ./cmd/buchisy`
Expected: builds clean.

- [ ] **Step 4: Manual check**

Open SuSa, OPOS, Controlling, Audit, Belegabgleich, Erlös-Abgleich → right-click an amount/cell → "Kopieren" present and copies the value.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/susaview.go internal/ui/oposview.go internal/ui/controllingview.go internal/ui/auditview.go internal/ui/belegabgleichview.go internal/ui/erloesabgleichview.go
git commit -m "feat(ui): make report and reconciliation cells copyable (incl. SuSa)"
```

---

### Task 3: Theme pass — Inter font

**Files:**
- Create: `internal/ui/fonts.go` (generated by `fyne bundle`)
- Add: `assets/fonts/Inter-Regular.ttf`, `Inter-Bold.ttf` (OFL, from rsms.me/inter)
- Modify: `internal/ui/theme.go` (add `Font()` method to `buchisyTheme`)

**Interfaces:**
- Consumes: bundled resources `resourceInterRegularTtf`, `resourceInterBoldTtf` (confirm exact symbol names in generated file).
- Produces: `func (t *buchisyTheme) Font(style fyne.TextStyle) fyne.Resource`.

- [ ] **Step 1: Add the font files** to `assets/fonts/`.

- [ ] **Step 2: Bundle them**

```bash
~/go/bin/fyne bundle -package ui -prefix resource -o internal/ui/fonts.go assets/fonts/Inter-Regular.ttf
~/go/bin/fyne bundle -package ui -prefix resource -append -o internal/ui/fonts.go assets/fonts/Inter-Bold.ttf
```
Confirm symbol names in `internal/ui/fonts.go`.

- [ ] **Step 3: Add `Font()` to the theme** (`internal/ui/theme.go`)

```go
func (t *buchisyTheme) Font(style fyne.TextStyle) fyne.Resource {
	if style.Monospace {
		return t.Theme.Font(style)
	}
	if style.Bold {
		return resourceInterBoldTtf
	}
	return resourceInterRegularTtf
}
```

- [ ] **Step 4: Build & run**

Run: `go build -o build/buchisy ./cmd/buchisy && ./build/buchisy`
Expected: launches; text renders in Inter.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/fonts.go internal/ui/theme.go assets/fonts/
git commit -m "feat(ui): bundle and apply Inter as the UI font"
```

---

### Task 4: Theme pass — color & size tokens

**Files:**
- Modify: `internal/ui/theme.go` (extend existing `Color()` and `Size()` overrides)

- [ ] **Step 1: Extend `Color()`** — add cases before the fallback (keep the existing primary/accent branch at the top):

```go
	switch name {
	case theme.ColorNameBackground:
		if variant == theme.VariantDark {
			return color.NRGBA{R: 24, G: 26, B: 30, A: 255}
		}
		return color.NRGBA{R: 250, G: 250, B: 252, A: 255}
	case theme.ColorNameSeparator:
		if variant == theme.VariantDark {
			return color.NRGBA{R: 255, G: 255, B: 255, A: 24}
		}
		return color.NRGBA{R: 0, G: 0, B: 0, A: 22}
	}
```

- [ ] **Step 2: Extend `Size()`** — add cases to the existing `switch` (keep scrollbar cases + the `* t.scale` multiply):

```go
	case theme.SizeNamePadding:
		base = 5
	case theme.SizeNameInnerPadding:
		base = 7
	case theme.SizeNameSeparatorThickness:
		base = 1
```

- [ ] **Step 3: Build & verify both variants**

Run: `go build -o build/buchisy ./cmd/buchisy && ./build/buchisy`
Expected: launches; toggle dark/light → palette consistent, spacing deliberate, accent buttons unchanged.

- [ ] **Step 4: Commit**

```bash
git add internal/ui/theme.go
git commit -m "feat(ui): fuller color and spacing tokens in the theme"
```

---

### Task 5: i18n keys for navigation labels

**Files:**
- Modify: `assets/i18n/de.json`, `assets/i18n/en.json`

**Interfaces:**
- Produces: keys consumed by Tasks 7–10 (see lists below). `menu.copy` already exists — do NOT re-add.

- [ ] **Step 1: Add German keys to `assets/i18n/de.json`**

```json
  "nav.group.erfassen": "Erfassen",
  "nav.group.buchen": "Buchen",
  "nav.group.auswerten": "Auswerten",
  "nav.group.finanzamt": "Finanzamt",
  "nav.group.abschluss": "Abschluss / GoBD",
  "nav.belege": "Belege",
  "nav.kassenbuch": "Kassenbuch",
  "nav.konten": "Konten (Bank)",
  "nav.belegabgleich": "Belegabgleich",
  "nav.erloesabgleich": "Erlös-Abgleich",
  "nav.anlagen": "Anlagen",
  "nav.susa": "SuSa",
  "nav.guv": "GuV",
  "nav.opos": "Offene Posten",
  "nav.controlling": "Controlling",
  "nav.yearoverview": "Übersicht (Jahr)",
  "nav.ustva": "USt-Voranmeldung",
  "nav.zm": "Zusammenfassende Meldung",
  "nav.lock": "Zeitraum sperren",
  "nav.unlock": "Zeitraum entsperren",
  "nav.audit": "Änderungsprotokoll",
  "nav.verfahrensdoku": "Verfahrensdokumentation",
  "nav.gobdexport": "DATEV/GoBD-Export",
  "menu.file": "Datei",
  "menu.edit": "Bearbeiten",
  "menu.export": "Export",
  "menu.view": "Ansicht",
  "menu.help": "Hilfe",
  "menu.quit": "Beenden",
  "menu.about": "Über BuchISY",
  "menu.import": "Mehrere Belege importieren …",
  "menu.backup": "Backup erstellen",
  "menu.renumber": "Belegnummern neu vergeben",
  "menu.autorules": "Auto-Regeln …",
  "menu.csvexport": "CSV-Export",
  "menu.bookingexport": "Buchungen exportieren",
  "menu.beleglistepdf": "Belegliste (PDF)",
  "menu.salesjournalpdf": "Rechnungsausgangsbuch (PDF)",
  "menu.zoomin": "Vergrößern",
  "menu.zoomout": "Verkleinern",
  "menu.zoomreset": "Zoom zurücksetzen",
  "menu.prevmonth": "Voriger Monat",
  "menu.nextmonth": "Nächster Monat",
  "menu.legend": "Legende (Kennzahlen)",
  "copy.hint": "Rechtsklick oder Strg+C zum Kopieren",
```

- [ ] **Step 2: Add matching English keys to `assets/i18n/en.json`**

```json
  "nav.group.erfassen": "Capture",
  "nav.group.buchen": "Booking",
  "nav.group.auswerten": "Reports",
  "nav.group.finanzamt": "Tax office",
  "nav.group.abschluss": "Closing / GoBD",
  "nav.belege": "Receipts",
  "nav.kassenbuch": "Cash book",
  "nav.konten": "Accounts (bank)",
  "nav.belegabgleich": "Receipt matching",
  "nav.erloesabgleich": "Revenue matching",
  "nav.anlagen": "Fixed assets",
  "nav.susa": "Trial balance",
  "nav.guv": "P&L",
  "nav.opos": "Open items",
  "nav.controlling": "Controlling",
  "nav.yearoverview": "Year overview",
  "nav.ustva": "VAT return",
  "nav.zm": "EC sales list",
  "nav.lock": "Lock period",
  "nav.unlock": "Unlock period",
  "nav.audit": "Audit log",
  "nav.verfahrensdoku": "Process documentation",
  "nav.gobdexport": "DATEV/GoBD export",
  "menu.file": "File",
  "menu.edit": "Edit",
  "menu.export": "Export",
  "menu.view": "View",
  "menu.help": "Help",
  "menu.quit": "Quit",
  "menu.about": "About BuchISY",
  "menu.import": "Import multiple receipts …",
  "menu.backup": "Create backup",
  "menu.renumber": "Renumber document numbers",
  "menu.autorules": "Auto rules …",
  "menu.csvexport": "CSV export",
  "menu.bookingexport": "Export bookings",
  "menu.beleglistepdf": "Receipt list (PDF)",
  "menu.salesjournalpdf": "Sales journal (PDF)",
  "menu.zoomin": "Zoom in",
  "menu.zoomout": "Zoom out",
  "menu.zoomreset": "Reset zoom",
  "menu.prevmonth": "Previous month",
  "menu.nextmonth": "Next month",
  "menu.legend": "Legend (key figures)",
  "copy.hint": "Right-click or Ctrl+C to copy",
```

- [ ] **Step 3: Build (embeds i18n)**

Run: `go build -o build/buchisy ./cmd/buchisy`
Expected: builds; JSON valid.

- [ ] **Step 4: Commit**

```bash
git add assets/i18n/de.json assets/i18n/en.json
git commit -m "i18n: add navigation, menu, and copy-hint strings"
```

---

### Task 6: Extract period selectors + small helpers (shell prerequisite)

**Why:** `yearSelect`/`monthSelect` are created inside `buildTopBar` (`app.go:871, 890`), and the menu/header need a legend handler and a lock indicator that don't exist yet. Extract these FIRST so later tasks reuse real, initialized fields — fixing the "stale/nil selector" and "missing helper" blockers.

**Files:**
- Modify: `internal/ui/app.go`

**Interfaces:**
- Produces:
  - `func (a *App) buildPeriodSelectors() (year fyne.CanvasObject, monthControls fyne.CanvasObject)` — constructs `a.yearSelect`/`a.monthSelect` (moved verbatim from `buildTopBar`) plus prev/next month buttons; returns the year wrap and the `[◀ month ▶]` group.
  - `func (a *App) lockIndicator() fyne.CanvasObject` — returns a small label showing `period.locked.indicator` when `a.currentMonthLocked`, else an empty object.
  - `func (a *App) showLegend()` — calls the existing legend popup via `a.invoiceTable.LegendButton()` semantics (extract the legend popup body into a reusable method, or call `.LegendButton().OnTapped()` if non-nil).
  - `func (a *App) showAbout()` — `dialog.ShowInformation(a.bundle.T("menu.about"), a.bundle.T("app.about"), a.window)`.
  - `func (a *App) importMultiple()` — wraps `a.showFilesPicker(func(paths []string){ a.enqueueSubmissions(paths) })`.

- [ ] **Step 1: Extract `buildPeriodSelectors`**

Move the `a.yearSelect` (`app.go:868–880, 956`) and `a.monthSelect` (`app.go:882–908`) construction, plus `prevMonthBtn`/`nextMonthBtn` (`app.go:989–996`), out of `buildTopBar` into:

```go
// buildPeriodSelectors constructs the year + month controls (creating
// a.yearSelect / a.monthSelect) and the prev/next month buttons.
func (a *App) buildPeriodSelectors() (fyne.CanvasObject, fyne.CanvasObject) {
	// ... (verbatim year/month newHighlightedSelect construction from buildTopBar) ...
	yearWrap := container.New(fixedWidthLayout{width: 90}, a.yearSelect)
	prev := widget.NewButtonWithIcon("", theme.NavigateBackIcon(), func() { a.stepMonth(-1) })
	prev.Importance = widget.LowImportance
	next := widget.NewButtonWithIcon("", theme.NavigateNextIcon(), func() { a.stepMonth(1) })
	next.Importance = widget.LowImportance
	monthControls := container.NewHBox(prev, container.NewStack(a.monthSelect), next)
	return yearWrap, monthControls
}
```

- [ ] **Step 2: Add `lockIndicator`, `showLegend`, `showAbout`, `importMultiple`**

```go
func (a *App) lockIndicator() fyne.CanvasObject {
	if !a.currentMonthLocked {
		return container.NewWithoutLayout()
	}
	lbl := widget.NewLabelWithStyle("🔒 "+a.bundle.T("period.locked.indicator"),
		fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	return lbl
}

func (a *App) showAbout() {
	dialog.ShowInformation(a.bundle.T("menu.about"), a.bundle.T("app.about"), a.window)
}

func (a *App) importMultiple() {
	a.showFilesPicker(func(paths []string) { a.enqueueSubmissions(paths) })
}
```

For `showLegend`: read `a.invoiceTable.LegendButton()` (`table.go:511`). Expose the popup body as `(it *InvoiceTable) ShowLegend()` and have `a.showLegend()` call it. **Nil-guard required** — the menu is global, so `showLegend` can fire in Konten mode where `a.invoiceTable` may be nil:

```go
func (a *App) showLegend() {
	if a.invoiceTable == nil {
		return
	}
	a.invoiceTable.ShowLegend()
}
```
Keep the existing "?" button working (it can call the same `ShowLegend()`).

- [ ] **Step 3: Keep `buildTopBar` compiling**

Have `buildTopBar` call `a.buildPeriodSelectors()` for the year/month controls instead of constructing them inline (so the app still builds and behaves identically at this step — the shell rewrite in Task 9 will drop `buildTopBar`).

- [ ] **Step 4: Build & verify no behavior change**

Run: `go build -o build/buchisy ./cmd/buchisy && ./build/buchisy`
Expected: identical UI to before; month/year selectors + arrows still work; "?" legend still works.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/app.go internal/ui/table.go
git commit -m "refactor(ui): extract period selectors and legend/lock/import helpers"
```

---

### Task 7: i18n-wired native main menu (actions)

**Files:**
- Create: `internal/ui/mainmenu.go`
- Modify: `internal/ui/app.go` (install in `startProfile`/`showMainView`)

**Interfaces:**
- Consumes: existing action funcs (`openTargetFolder`, `importMultiple` [Task 6], `showBackup`, `renumberBelegnummern`, `showAutoRulesDialog`, `showCSVExportDialog`, `showBookingExportDialog`, `showBelegListePDF`, `showSalesJournalPDF`, `showExportPackage`, `showVerfahrensdokuPDF`, `adjustUIScale`, `setUIScale`, `stepMonth`, `showLegend`, `showAbout` [Task 6], `a.app.Quit`).
- Produces: `func (a *App) buildMainMenu() *fyne.MainMenu`.

- [ ] **Step 1: Create `mainmenu.go`**

```go
package ui

import "fyne.io/fyne/v2"

// buildMainMenu builds the native menu bar holding one-shot ACTIONS
// (navigation lives in the sidebar). Every item calls an existing handler.
func (a *App) buildMainMenu() *fyne.MainMenu {
	t := a.bundle.T

	file := fyne.NewMenu(t("menu.file"),
		fyne.NewMenuItem(t("menu.import"), a.importMultiple),
		fyne.NewMenuItem(t("menu.openTarget"), a.openTargetFolder),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem(t("menu.backup"), a.showBackup),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem(t("menu.quit"), a.app.Quit),
	)
	edit := fyne.NewMenu(t("menu.edit"),
		fyne.NewMenuItem(t("menu.renumber"), a.renumberBelegnummern),
		fyne.NewMenuItem(t("menu.autorules"), a.showAutoRulesDialog),
	)
	export := fyne.NewMenu(t("menu.export"),
		fyne.NewMenuItem(t("menu.csvexport"), a.showCSVExportDialog),
		fyne.NewMenuItem(t("menu.bookingexport"), a.showBookingExportDialog),
		fyne.NewMenuItem(t("menu.beleglistepdf"), a.showBelegListePDF),
		fyne.NewMenuItem(t("menu.salesjournalpdf"), a.showSalesJournalPDF),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem(t("nav.gobdexport"), a.showExportPackage),
		fyne.NewMenuItem(t("nav.verfahrensdoku"), a.showVerfahrensdokuPDF),
	)
	view := fyne.NewMenu(t("menu.view"),
		fyne.NewMenuItem(t("menu.zoomin"), func() { a.adjustUIScale(UIScaleStep) }),
		fyne.NewMenuItem(t("menu.zoomout"), func() { a.adjustUIScale(-UIScaleStep) }),
		fyne.NewMenuItem(t("menu.zoomreset"), func() { a.setUIScale(1.0) }),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem(t("menu.prevmonth"), func() { a.stepMonth(-1) }),
		fyne.NewMenuItem(t("menu.nextmonth"), func() { a.stepMonth(1) }),
	)
	help := fyne.NewMenu(t("menu.help"),
		fyne.NewMenuItem(t("menu.legend"), a.showLegend),
		fyne.NewMenuItem(t("menu.about"), a.showAbout),
	)
	return fyne.NewMainMenu(file, edit, export, view, help)
}
```

- [ ] **Step 2: Install the menu** once per profile — in `startProfile`, after `a.window.SetContent(a.buildUI())` (`app.go:319`):

```go
	a.window.SetMainMenu(a.buildMainMenu())
```
(It persists across `SetContent`; no need to re-set on every rebuild.)

- [ ] **Step 3: Build & verify**

Run: `go build -o build/buchisy ./cmd/buchisy && ./build/buchisy`
Expected: native menu bar (Datei/Bearbeiten/Export/Ansicht/Hilfe); each item triggers the same dialog/action as the old ⋮; Beenden quits; Über shows info.

- [ ] **Step 4: Commit**

```bash
git add internal/ui/mainmenu.go internal/ui/app.go
git commit -m "feat(ui): native menu bar for one-shot actions"
```

---

### Task 8: Workflow sidebar component (screens)

**Files:**
- Create: `internal/ui/sidebar.go`
- Modify: `internal/ui/kontenview.go` / `app.go` (extract `switchToBelege` / `openKontenPicker` from `viewToggleButtons`)

**Interfaces:**
- Consumes: screen funcs (`showSuSa`, `showGuV`, `showOpenItems`, `showControllingDialog`, `showYearOverviewDialog`, `showUStVADialog`, `showZMDialog`, `showCashBookView`, `showBelegabgleich`, `showErloesAbgleich`, `showAnlagen`, `showAuditLog`, `lockCurrentMonth`, `showExportPackage`, `showVerfahrensdokuPDF`), plus mode switches.
- Produces: `func (a *App) buildSidebar() fyne.CanvasObject`; `func (a *App) switchToBelege()`; `func (a *App) openKontenPicker()`.

- [ ] **Step 1: Extract mode-switch helpers from `viewToggleButtons`**

```go
// in kontenview.go or app.go
func (a *App) switchToBelege() {
	a.viewMode = ""
	a.window.SetContent(a.buildUI())
}
// openKontenPicker: lift the account-picker popup body out of viewToggleButtons'
// Konten button OnTapped so the sidebar can trigger the same flow.
```
Keep `viewToggleButtons` for the in-Konten account chip row (`kontenview.go:586`) if still used there.

Also add the lock/unlock toggle (the old ⋮ had BOTH `lockCurrentMonth` at app.go:939 AND `unlockCurrentMonth` at app.go:940 — both must stay reachable):

```go
// lockToggleNavItem returns the Festschreibung entry, showing "Zeitraum
// entsperren" when the current month is already locked, else "Zeitraum sperren".
func (a *App) lockToggleNavItem() navItem {
	if a.currentMonthLocked {
		return navItem{"nav.unlock", a.unlockCurrentMonth}
	}
	return navItem{"nav.lock", a.lockCurrentMonth}
}
```

- [ ] **Step 2: Create `sidebar.go`** (fixed width via `fixedWidthLayout`, which already exists in the repo)

```go
package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

type navItem struct {
	key    string
	action func()
}

// buildSidebar returns the persistent workflow navigation column (fixed width).
func (a *App) buildSidebar() fyne.CanvasObject {
	groups := []struct {
		titleKey string
		items    []navItem
	}{
		{"nav.group.erfassen", []navItem{
			{"nav.belege", a.switchToBelege},
			{"nav.kassenbuch", a.showCashBookView},
		}},
		{"nav.group.buchen", []navItem{
			{"nav.konten", a.openKontenPicker},
			{"nav.belegabgleich", a.showBelegabgleich},
			{"nav.erloesabgleich", a.showErloesAbgleich},
			{"nav.anlagen", a.showAnlagen},
		}},
		{"nav.group.auswerten", []navItem{
			{"nav.susa", a.showSuSa},
			{"nav.guv", a.showGuV},
			{"nav.opos", a.showOpenItems},
			{"nav.controlling", a.showControllingDialog},
			{"nav.yearoverview", a.showYearOverviewDialog},
		}},
		{"nav.group.finanzamt", []navItem{
			{"nav.ustva", a.showUStVADialog},
			{"nav.zm", a.showZMDialog},
		}},
		{"nav.group.abschluss", []navItem{
			a.lockToggleNavItem(), // lock OR unlock depending on a.currentMonthLocked
			{"nav.audit", a.showAuditLog},
			{"nav.verfahrensdoku", a.showVerfahrensdokuPDF},
			{"nav.gobdexport", a.showExportPackage},
		}},
	}

	col := container.NewVBox()
	for _, g := range groups {
		col.Add(widget.NewLabelWithStyle(a.bundle.T(g.titleKey),
			fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
		for _, it := range g.items {
			item := it // capture
			btn := widget.NewButton(a.bundle.T(item.key), item.action)
			btn.Alignment = widget.ButtonAlignLeading
			btn.Importance = widget.LowImportance
			col.Add(btn)
		}
		col.Add(widget.NewSeparator())
	}

	// Fixed ~200px width so the Border left region doesn't size to button
	// min-width. fixedWidthLayout already exists in the repo (used at app.go:956).
	return container.New(fixedWidthLayout{width: 200}, container.NewVScroll(col))
}
```

- [ ] **Step 3: Build (component not yet placed)**

Run: `go build -o build/buchisy ./cmd/buchisy`
Expected: compiles (if Go complains about unused `buildSidebar`, that resolves in Task 9; you may temporarily reference it).

- [ ] **Step 4: Commit**

```bash
git add internal/ui/sidebar.go internal/ui/kontenview.go internal/ui/app.go
git commit -m "feat(ui): fixed-width workflow sidebar component (screens)"
```

---

### Task 9: Wire outer shell (both modes) + remove ⋮ menu & toggles

**Files:**
- Modify: `internal/ui/app.go` (`buildUI` outer shell, period header, remove overflow + toggles, drop/shrink `buildTopBar`)

**Interfaces:**
- Consumes: `a.buildSidebar()`, `a.buildPeriodSelectors()`, `a.lockIndicator()`, `a.buildStatusBar()`, `a.buildKontenUI()`.
- Produces: new shell.

- [ ] **Step 1: Build the prominent period header**

```go
// buildPeriodHeader returns the always-visible Buchungszeitraum strip + settings gear.
func (a *App) buildPeriodHeader() fyne.CanvasObject {
	yearWrap, monthControls := a.buildPeriodSelectors()
	period := container.NewHBox(yearWrap, monthControls, a.lockIndicator())
	settings := widget.NewButtonWithIcon("", theme.SettingsIcon(), func() { a.showSettingsView() })
	settings.Importance = widget.LowImportance
	return container.NewBorder(nil, nil, period, settings, nil)
}
```

- [ ] **Step 2: Restructure `buildUI` to wrap BOTH modes in one shell**

Replace the early-return konten branch and the Belege border (`app.go:663–666, 780`) so the shell is built ONCE around whichever content the mode produces:

```go
func (a *App) buildUI() fyne.CanvasObject {
	a.applyAccentForMode()

	var content fyne.CanvasObject
	if a.viewMode == "konten" {
		content = a.buildKontenContent() // konten body WITHOUT its own outer shell
	} else {
		content = a.buildBelegeContent() // belege body (table + filter row + hint banners + upload card)
	}

	a.mainContent = container.NewBorder(
		a.buildPeriodHeader(), // top
		a.buildStatusBar(),    // bottom
		a.buildSidebar(),      // left (fixed width)
		nil,                   // right
		content,               // center
	)
	return a.mainContent
}
```

> Refactor note: split today's `buildUI` Belege body (table creation, drop handler, filter row, hint banners, upload card — `app.go:667–778`) into `buildBelegeContent()`, and split `buildKontenUI()` into `buildKontenContent()` that returns ONLY the statement/account body.
>
> CRITICAL (Codex): `buildKontenUI()` today builds its OWN chrome that the new shell now provides — you MUST strip these from `buildKontenContent()` or they double up:
> - its own Belege/Konten toggles (`kontenview.go:586`)
> - its own top bar (`kontenview.go:701`)
> - its own settings button (`kontenview.go:638`)
> - its own status bar (`kontenview.go:726`)
> The account picker/chip row for Zahlungskonten STAYS in `buildKontenContent()` (it's content, not chrome). The upload card and Belege filter row belong in `buildBelegeContent`, NOT the global shell.

- [ ] **Step 3: Remove the ⋮ overflow menu and toggle buttons**

Delete the `overflowBtn` block (`app.go:917–952`) and remove `belegeBtn, kontenBtn := a.viewToggleButtons()` + `leftGroup` from the old `buildTopBar` (`app.go:1009–1010`). `buildTopBar` is no longer called by `buildUI`; delete it (its period selectors now live in `buildPeriodHeader`, its upload card in `buildBelegeContent`).

- [ ] **Step 4: Build & full manual regression**

Run: `go build -o build/buchisy ./cmd/buchisy && ./build/buchisy`
Expected:
- Sidebar shows all 5 groups; clicking each opens the same screen/action as the old ⋮ item.
- **Sidebar + period header stay visible in BOTH Belege and Konten modes.**
- Belege and Konten are in different groups (Erfassen vs Buchen), clearly separated.
- Period header prominent at top; month arrows + year + lock indicator work.
- Menu bar actions all work. ⋮ button and the two old toggle buttons are GONE.
- Verify each of the 25 former ⋮ items against `git show HEAD~N:internal/ui/app.go` — none unreachable.

- [ ] **Step 5: Run tests**

Run: `go test ./...`
Expected: pass.

- [ ] **Step 6: Commit**

```bash
git add internal/ui/app.go
git commit -m "feat(ui): outer shell wraps both modes; remove overflow menu and toggles"
```

---

### Task 10: Copy-hint affordance + final acceptance

**Files:**
- Modify: `internal/ui/app.go` (`buildStatusBar` — append a muted hint)

- [ ] **Step 1: Add the hint to the status bar**

In `buildStatusBar` (`app.go:786`), add a trailing muted label and place it on the right of the bar's Border:

```go
	hint := widget.NewLabelWithStyle(a.bundle.T("copy.hint"),
		fyne.TextAlignTrailing, fyne.TextStyle{Italic: true})
	hint.Importance = widget.LowImportance
	bar := container.NewStack(bg, container.NewBorder(nil, nil,
		container.NewPadded(lbl), container.NewPadded(hint)))
	return container.NewBorder(widget.NewSeparator(), nil, nil, nil, bar)
```

- [ ] **Step 2: Build & verify**

Run: `go build -o build/buchisy ./cmd/buchisy && ./build/buchisy`
Expected: status bar shows "Rechtsklick oder Strg+C zum Kopieren"; switching language flips it.

- [ ] **Step 3: Full acceptance pass (spec §Test/Verifikation)**

- [ ] Each of the 25 former ⋮ items reachable in ≤1 click (sidebar or menu).
- [ ] Belege vs Konten clearly separated; shell persists across both modes.
- [ ] Ctrl+C copies row; SuSa/OPOS/Controlling/Audit/Belegabgleich/Erlös cells copyable.
- [ ] App launches with Inter; dark/light consistent; zoom (Ctrl ±) works.
- [ ] `go test ./...` green; `go build` green; app runs on macOS.

- [ ] **Step 4: Commit**

```bash
git add internal/ui/app.go
git commit -m "feat(ui): visible copy hint in the status bar"
```

---

## Self-Review (post-codex-revision)

**Codex P1 blockers — all fixed:**
- P1 Ctrl+C/clipboard → Task 1 uses `&fyne.ShortcutCopy{}` + `fyne.CurrentApp().Clipboard()`. ✅
- P1 `it.rows` → Task 1 uses `it.filtered`. ✅
- P1 `newHoverLabel("")` → Task 2 uses `newHoverLabel(nil, nil)` + state reset. ✅
- P1 missing `showLegend`/`lockIndicator` → Task 6 creates real helpers. ✅
- P1 period-selector ordering → Task 6 extracts `buildPeriodSelectors` BEFORE shell. ✅
- P1 shell ignores Konten → Task 9 wraps BOTH modes in one outer Border. ✅

**Codex P2s — fixed:**
- Sidebar width → Task 8 uses `fixedWidthLayout{width: 200}`. ✅
- SuSa copy omitted → added to Task 2. ✅
- Menu mapping under-specified → `importMultiple`/`showAbout`/`showLegend` created in Task 6 before Task 7 uses them. ✅

**Spec coverage:** sidebar (8,9); menu bar (7); prominent period (9); copy Ctrl+C (1) + cells (2) + hint (10); theme font (3) + tokens (4); remove ⋮+toggles (9); i18n DE+EN (5); no feature unreachable (9 Step 4). ✅

**Type consistency:** `CopySelectedRow() string`; `buildSidebar()/buildPeriodHeader()/buildMainMenu()` return types consistent; `buildPeriodSelectors()` two-value return matches its use in `buildPeriodHeader`; `switchToBelege`/`openKontenPicker` named consistently across 8–9.

**Ordering:** 1–5 independent/low-risk (copy, theme, i18n). 6 is the prerequisite extraction. 7→8→9 build the shell incrementally; each leaves the app compiling. 10 finishes.

## Self-Review (post-codex round 2)

Codex re-reviewed the revised plan and confirmed the 6 original P1s fixed. It found 4 more; all now addressed:
- **P1 lost `unlockCurrentMonth()`** (old ⋮ had both lock+unlock) → Task 8 adds `lockToggleNavItem()` showing lock OR unlock by `currentMonthLocked`; `nav.unlock` i18n key added (Task 5). ✅
- **P1 Ctrl+C leaks outside Belege** (canvas shortcut persists across SetContent) → Task 1 handler now gates `it.app.viewMode == "" && it.app.invoiceTable == it`. ✅
- **P2 recycled-cell style leak** (SuSa/OPOS reset TextStyle+Alignment each update) → Task 2 update func now resets `TextStyle` + `Alignment`, matching the original per-column. ✅
- **P2 Konten chrome double-up** → Task 9 refactor note now lists the exact chrome to strip (`kontenview.go:586/701/638/726`) and what stays (account picker). ✅

Codex confirmed valid: `&fyne.ShortcutCopy{}`, `filtered/columnOrder/selectedRow`, `hoverLabel` embedding `widget.Label`, `LegendButton()`, `showFilesPicker(func([]string))`, `dialog` already imported, `fixedWidthLayout{width float32}` (app.go:1272). Add a nil-guard in `showLegend()` since the menu is global.
