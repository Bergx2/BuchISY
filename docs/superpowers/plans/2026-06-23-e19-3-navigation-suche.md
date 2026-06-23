# E19.3 — Navigation & Suche — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Steps use `- [ ]`.

**Goal:** Find invoices across all months, step months quickly, see a per-year overview, and drive the table from the keyboard.

**Architecture:** Live month filtering already exists (`InvoiceTable.filterEntry.OnChanged → applyFilter`). We add: a global DB search on Enter (`OnSubmitted`) with a jump-to-result overlay; ◀▶ month-step buttons + Ctrl+←/→; a per-year overview dialog from a new core KPI helper; and ↑/↓/Enter/Del keyboard handling on the table.

**Tech Stack:** Go 1.25, Fyne. `internal/core`, `internal/db`, `internal/ui`. Branch `feat/e19-3-navigation-suche`.

## Global Constraints

- `go build ./...`, `go test ./...`. Commit per task. Co-author trailer: `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`.
- i18n via `a.bundle.T(...)`, keys in BOTH `assets/i18n/{de,en}.json`.
- `a.collectInvoiceRows(fromY,fromM,toY,toM)` returns rows for a span. `a.currentYear int`, `a.currentMonth time.Month`; `a.yearSelect`/`a.monthSelect` are `*highlightedSelect` with `SetSelected`; `a.onMonthChanged()` reloads. The table is `a.invoiceTable`; `loadInvoices()` reloads it.

---

### Task 1: `Repository.SearchInvoices` (global)

**Files:** `internal/db/repository.go`, `internal/db/repository_test.go`.

**Interface:** `(*Repository).SearchInvoices(query string) ([]core.CSVRow, error)` — matches across all months.

- [ ] **Step 1:** READ `List` (`repository.go:300+`). Extract its row-scan loop (everything after `rows, err := r.db.Query(...)` that builds `[]core.CSVRow`) into an unexported helper `func scanInvoiceRows(rows *sql.Rows) ([]core.CSVRow, error)` and make `List` call it. Run full db tests → still green (pure refactor).
- [ ] **Step 2: Test** (append to repository_test.go, mirror the temp-DB setup): insert invoices in two different months (e.g. "Müller GmbH" rechnungsnummer "R-77" in 2026/01, and a 2025/12 row), then assert `SearchInvoices("müller")` returns the Müller row, `SearchInvoices("R-77")` returns it, and `SearchInvoices("zzzz")` returns none. Run → fail.
- [ ] **Step 3:** Implement `SearchInvoices`: same SELECT column list as `List`, `WHERE LOWER(auftraggeber) LIKE ? OR LOWER(verwendungszweck) LIKE ? OR LOWER(rechnungsnummer) LIKE ? OR LOWER(belegnummer) LIKE ?` with one `%lowered%` arg each, `ORDER BY rechnungsdatum DESC LIMIT 200`; scan via `scanInvoiceRows`. Trim/lower the query in Go; empty query → return `nil, nil`.
- [ ] **Step 4:** run → pass + full db. Commit `E19.3: Repository.SearchInvoices global search (DRY scanInvoiceRows)`.

---

### Task 2: Global search overlay + jump-to-result

**Files:** `internal/ui/table.go` (filterEntry wiring) and/or `internal/ui/app.go`, `assets/i18n/{de,en}.json`.

**Context:** `it.filterEntry.OnChanged` already live-filters the current month. `OnSubmitted` is free.

- [ ] **Step 1:** Set `it.filterEntry.OnSubmitted = func(q string){ it.app.showGlobalSearch(q) }` (add the entrypoint on App). Guard empty `q`.
- [ ] **Step 2:** Implement `func (a *App) showGlobalSearch(query string)`: call `a.dbRepo.SearchInvoices(query)`; show results in a `widget.NewModalPopUp`/`PopUp` as a `widget.List` (rows: `Belegnr · Datum · Auftraggeber · Brutto`), a result count header, and a Close button. On a row tap: hide the popup, switch the app to the result's month/year (`a.currentYear`/`a.currentMonth` = parsed from the row's Jahr/Monat, update `yearSelect`/`monthSelect` via `SetSelected`, call `a.onMonthChanged()`), then select/scroll the table to that invoice (match by `Dateiname`; add a small `InvoiceTable.SelectByDateiname(string)` helper that finds the filtered index and calls `it.table.ScrollTo`/`Select`). On empty results show a "keine Treffer" line.
- [ ] **Step 3:** i18n keys `search.results` ("%d Treffer"), `search.none` ("Keine Treffer") in both JSONs. `go build ./... && go test ./...`. Commit `E19.3: global search overlay with jump-to-result`.

---

### Task 3: ◀▶ month navigation + Ctrl+←/→

**Files:** `internal/ui/app.go` (top bar near the month/year selects + `registerZoomShortcuts`-style shortcut registration).

- [ ] **Step 1:** Add `func (a *App) stepMonth(delta int)`: compute new month = `int(a.currentMonth)+delta`; roll over year (month 0 → Dec prev year; 13 → Jan next year); clamp year to the available range used by `yearSelect`; set `a.currentMonth`/`a.currentYear`, update `monthSelect`/`yearSelect` via `SetSelected` (use the same label formats they were built with), and call `a.onMonthChanged()`. Avoid double-reload (set selects without retriggering, or guard).
- [ ] **Step 2:** Add ◀ and ▶ `widget.NewButtonWithIcon` (`theme.NavigateBackIcon()`/`NavigateNextIcon()`) next to the month select in the top bar calling `stepMonth(-1)`/`stepMonth(+1)`.
- [ ] **Step 3:** Register canvas shortcuts Ctrl+Left → `stepMonth(-1)`, Ctrl+Right → `stepMonth(+1)` (mirror `registerZoomShortcuts` with `desktop.CustomShortcut{KeyName: fyne.KeyLeft/Right, Modifier: fyne.KeyModifierControl}`).
- [ ] **Step 4:** `go build ./...`. Commit `E19.3: ◀▶ month navigation + Ctrl+←/→`.

---

### Task 4: Per-year overview (KPI helper + dialog)

**Files:** `internal/core/overview.go` (new) + `internal/core/overview_test.go` (new), `internal/ui/app.go` (a menu item + dialog).

**Interface:** `core.OverviewKPIs(rows []CSVRow) OverviewKPI` with fields `Count int`, `Netto/USt/Brutto float64`, `Zahllast float64`, `OpenReconcile int` (rows that need but lack a BuchungRef — bank/creditcard, BuchungRef==""), `Warnings int` (rows with `len(InvoiceWarnings(row))>0`).

- [ ] **Step 1: Test** (`overview_test.go`): two rows (one booked+linked, one with a warning + no BuchungRef on a bank account) → assert Count=2, summed Netto/Brutto, OpenReconcile=1, Warnings≥1.
- [ ] **Step 2:** run → fail.
- [ ] **Step 3:** Implement `OverviewKPIs`: sum BetragNetto/SteuersatzBetrag/Bruttobetrag; `Zahllast` = USt on outgoing − Vorsteuer-ish is out of scope here, so set `Zahllast` = sum SteuersatzBetrag on `Ausgangsrechnung` rows minus sum on expense rows (document it as a rough indicator) OR omit Zahllast if ambiguous — keep it simple: `Zahllast` = Σ SteuersatzBetrag(Ausgangsrechnung) − Σ SteuersatzBetrag(!Ausgangsrechnung). `OpenReconcile`/`Warnings` as defined. (Reconciliation-eligibility: treat any non-empty Bankkonto with `BuchungRef==""` as open; cash handled by E18 separately — keep the rule simple and documented.)
- [ ] **Step 4:** run → pass + full core. Add a menu item "Übersicht (Jahr)" that, for each month 1–12 of `a.currentYear`, runs `OverviewKPIs(a.collectInvoiceRows(y,m,y,m))` and shows a table dialog (month · count · Brutto · #offen · #Warnungen) with a total row; a row tap jumps to that month (set selects + `onMonthChanged`, close dialog). i18n for headers. Commit `E19.3: per-year overview KPIs + dialog`.

---

### Task 5: Table keyboard navigation

**Files:** `internal/ui/table.go`, `internal/ui/app.go` (focus wiring).

**Context:** `it.table.OnSelected` exists (table.go:445). Fyne `widget.Table` supports `Select(TableCellID)` and is focusable.

- [ ] **Step 1:** Add keyboard handling so the table responds to ↑/↓ (move selection by row), Enter (open edit dialog for the selected row), Del (delete selected row with the existing confirm). Implement via the canvas `SetOnTypedKey` on the main window WHEN the table has focus, OR via `it.table.OnSelected` tracking `it.selectedRow` plus shortcuts — choose the approach that fits Fyne's table focus model after reading how selection works here. Keep a `selectedRow int` on the table; ↑/↓ clamp to `[0, RowCount()-1]` and `it.table.Select`/`ScrollTo`; Enter → `it.app.showEditDialog(it.filtered[selectedRow], nil)`; Del → the existing delete path with confirm.
- [ ] **Step 2:** Ensure these keys don't fire while a text entry (search/filter) is focused (guard on focused widget if needed). `go build ./... && go test ./...`. Commit `E19.3: table keyboard navigation (↑/↓/Enter/Del)`.

## Self-Review

Spec coverage: #3 → Task 1+2 (live filter already existed). #10 → Task 3 (nav) + Task 4 (overview; reuses no cash-specific code — new general KPI helper). #11 → Task 5. DRY `scanInvoiceRows`. Core helpers unit-tested.
