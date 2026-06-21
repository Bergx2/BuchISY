# Export-Robustheit (Phase E2) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Protect the accountant hand-off: track which bookings were exported (double-booking guard with warn+override), show an export preview/summary (incl. why receipts are skipped), and surface receipts without a (valid) booking.

**Architecture:** A new `CSVRow.Exportiert` flag (persisted in CSV + SQLite) is set after a successful export and reset whenever the invoice is updated. A pure `ClassifyForExport` splits a period's rows into exportable / already-exported / skipped-with-reason. The export dialog shows that breakdown as a confirm step, offers an "include already exported" option, and marks the exported rows afterwards. A "Belege ohne Buchung" quick-filter chip lists receipts whose booking is missing or unbalanced.

**Tech Stack:** Go 1.25, Fyne v2. Builds on D2 export (`BuildDATEVStapel`/`BuildLexwareCSV`, `runBookingExport`), the D1 `Booking`, and the table's existing quick-filter chip system.

## Global Constraints

- `CSVRow.Exportiert bool` persists in CSV (column `Exportiert`, JSON-free "true"/"false") and SQLite (`exportiert INTEGER DEFAULT 0`), NULL-safe on read (mirror the `buchung`/`trinkgeld` column handling).
- `Repository.Update` ALWAYS sets `exportiert = 0` (editing an invoice invalidates a prior export); `Repository.Insert` stores `0`; only `Repository.MarkExported` sets it to `1`.
- A receipt is "exportable" iff its `Booking.Balanced()` AND it has exactly one Haben (`Booking.PaymentEntry` ok) — the SAME rule the DATEV/Lexware exporters already use. Skipped reasons: "keine Buchung", "nicht ausgeglichen". Already-exported = exportable AND `Exportiert`.
- Default export EXCLUDES already-exported rows (warn with a count); an opt-in checkbox includes them. Never silently double-export.
- After a successful file write, mark exactly the rows that were written as exported.
- NEVER invent data. All user-facing strings via `a.bundle.T(...)` in both JSONs (valid JSON).
- `go build ./... && go test ./... && go vet ./internal/ui/` clean (ignore the pre-existing `clipboard_windows.go:206` warning). Commit per task.

---

### Task 1: Exportiert flag — storage + MarkExported

**Files:**
- Modify: `internal/core/types.go` (Meta + CSVRow + conversions)
- Modify: `internal/core/csvrepo.go` (column + load/save)
- Modify: `internal/db/schema.go` + `internal/db/repository.go` (column, migration, Insert/Update/List, MarkExported)
- Test: `internal/core/csvrepo_test.go`, `internal/db/repository_test.go`

**Interfaces:**
- Produces: `Meta.Exportiert bool` + `CSVRow.Exportiert bool` (carried by ToCSVRow/ToMeta); CSV column `Exportiert`; SQLite `exportiert` column; `Repository.MarkExported(jahr, monat, dateiname string) error` (sets `exportiert=1`); `Update` resets `exportiert=0`.

- [ ] **Step 1: Write the failing tests**

Add to `internal/db/repository_test.go`:

```go
func TestMarkExportedAndUpdateResets(t *testing.T) {
	repo := newTestRepo(t)
	if _, err := repo.Insert(core.CSVRow{Dateiname: "a.pdf", Jahr: "2026", Monat: "06"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.MarkExported("2026", "06", "a.pdf"); err != nil {
		t.Fatal(err)
	}
	rows, _ := repo.List("2026", "06")
	if len(rows) != 1 || !rows[0].Exportiert {
		t.Fatalf("expected Exportiert=true after MarkExported: %+v", rows)
	}
	// Updating the invoice must reset the exported flag.
	if err := repo.Update("2026", "06", "a.pdf", rows[0]); err != nil {
		t.Fatal(err)
	}
	rows, _ = repo.List("2026", "06")
	if rows[0].Exportiert {
		t.Error("Update must reset Exportiert to false")
	}
}
```

Add to `internal/core/csvrepo_test.go`:

```go
func TestCSVExportiertRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invoices.csv")
	repo := NewCSVRepository()
	if err := repo.Append(path, CSVRow{Dateiname: "a.pdf", Jahr: "2026", Monat: "06", Exportiert: true}); err != nil {
		t.Fatal(err)
	}
	rows, _ := repo.Load(path)
	if len(rows) != 1 || !rows[0].Exportiert {
		t.Fatalf("Exportiert not round-tripped: %+v", rows)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/core/ -run TestCSVExportiertRoundTrip; go test ./internal/db/ -run TestMarkExportedAndUpdateResets`
Expected: FAIL (unknown field Exportiert / undefined MarkExported).

- [ ] **Step 3: Implement**

In `internal/core/types.go`: add `Exportiert bool` to `Meta` and `CSVRow` (after `Buchung`); carry it in `ToCSVRow()` (`Exportiert: m.Exportiert`) and `ToMeta()` (`Exportiert: r.Exportiert`).

In `internal/core/csvrepo.go`: add `"Exportiert"` to `DefaultCSVColumns` (after `"Buchung"`), `ColumnDisplayNames` (`"Exportiert": "Exportiert"`), `ColumnTranslationKeys` (`"Exportiert": "table.col.exportiert"`); in `Load` after the row literal: `row.Exportiert = strings.EqualFold(strings.TrimSpace(valueForColumn(record, headerMap, "Exportiert")), "true")`; in `rowToRecord` valueMap: `"Exportiert": fmt.Sprintf("%t", row.Exportiert)` (confirm `fmt` imported). Add i18n key `table.col.exportiert` (de "Exportiert" / en "Exported") to both JSONs.

In `internal/db/schema.go`: add `exportiert INTEGER DEFAULT 0,` after `buchung TEXT,`. In `internal/db/repository.go`:
- `initSchema` ALTER loop: add `"ALTER TABLE invoices ADD COLUMN exportiert INTEGER DEFAULT 0"`.
- `Insert`: add `exportiert` column + `?` + arg `0` (always 0 on insert).
- `Update`: add `exportiert = 0` to the SET clause (no placeholder/arg — a literal `0`, so it always resets). Keep all other placeholders/args aligned.
- `List`: add `exportiert` to SELECT + scan into `sql.NullInt64` then `row.Exportiert = ni.Int64 != 0`.
- Add method:

```go
// MarkExported flags an invoice as having been included in a booking export.
func (r *Repository) MarkExported(jahr, monat, dateiname string) error {
	_, err := r.db.Exec(`UPDATE invoices SET exportiert = 1 WHERE jahr = ? AND monat = ? AND dateiname = ?`, jahr, monat, dateiname)
	if err != nil {
		return fmt.Errorf("failed to mark exported: %w", err)
	}
	return nil
}
```

Count check: Insert and List get one more column/arg; Update adds a literal `exportiert = 0` (no new arg). Verify Insert column-count == placeholder-count == arg-count, and List SELECT-count == scan-count.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/core/ ./internal/db/ && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/core/types.go internal/core/csvrepo.go internal/core/csvrepo_test.go internal/db/schema.go internal/db/repository.go internal/db/repository_test.go assets/i18n/de.json assets/i18n/en.json
git commit -m "Track per-invoice Exportiert flag (reset on update, MarkExported)"
```

---

### Task 2: ClassifyForExport (core)

**Files:**
- Create: `internal/core/exportclassify.go`
- Test: `internal/core/exportclassify_test.go`

**Interfaces:**
- Consumes: `CSVRow.Buchung` (`Balanced`/`PaymentEntry`), `CSVRow.Exportiert`.
- Produces: `type ExportSkip struct { Dateiname, Grund string }`;
  `type ExportClassification struct { Exportable []CSVRow; AlreadyExported []CSVRow; Skipped []ExportSkip }`;
  `ClassifyForExport(rows []CSVRow, includeExported bool) ExportClassification` — an exportable row has a balanced booking with one Haben; if it is also `Exportiert` it goes to `AlreadyExported` (and is added to `Exportable` only when `includeExported`); non-exportable rows go to `Skipped` with `Grund` "keine Buchung" (no entries) or "nicht ausgeglichen".

- [ ] **Step 1: Write the failing test**

```go
package core

import "testing"

func TestClassifyForExport(t *testing.T) {
	good := Booking{Entries: []BookingEntry{{Konto: 6640, Betrag: 10, Soll: true}, {Konto: 1800, Betrag: 10, Soll: false}}}
	rows := []CSVRow{
		{Dateiname: "neu.pdf", Buchung: good},
		{Dateiname: "alt.pdf", Buchung: good, Exportiert: true},
		{Dateiname: "leer.pdf"},
		{Dateiname: "schief.pdf", Buchung: Booking{Entries: []BookingEntry{{Konto: 6640, Betrag: 10, Soll: true}}}},
	}
	c := ClassifyForExport(rows, false)
	if len(c.Exportable) != 1 || c.Exportable[0].Dateiname != "neu.pdf" {
		t.Errorf("exportable = %+v", c.Exportable)
	}
	if len(c.AlreadyExported) != 1 {
		t.Errorf("alreadyExported = %+v", c.AlreadyExported)
	}
	if len(c.Skipped) != 2 {
		t.Fatalf("skipped = %+v", c.Skipped)
	}
	// includeExported puts the already-exported row back into Exportable.
	if len(ClassifyForExport(rows, true).Exportable) != 2 {
		t.Error("includeExported should yield 2 exportable")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run TestClassifyForExport`
Expected: FAIL (undefined ClassifyForExport).

- [ ] **Step 3: Implement**

```go
package core

// ExportSkip records why a receipt was left out of an export.
type ExportSkip struct {
	Dateiname string
	Grund     string
}

// ExportClassification splits a period's rows by export eligibility.
type ExportClassification struct {
	Exportable      []CSVRow
	AlreadyExported []CSVRow
	Skipped         []ExportSkip
}

// ClassifyForExport partitions rows into exportable, already-exported, and
// skipped (with a reason). An exportable row has a balanced booking with one
// Haben. Already-exported exportable rows are added to Exportable only when
// includeExported is true.
func ClassifyForExport(rows []CSVRow, includeExported bool) ExportClassification {
	var c ExportClassification
	for _, r := range rows {
		_, ok := r.Buchung.PaymentEntry()
		if !r.Buchung.Balanced() || !ok {
			grund := "nicht ausgeglichen"
			if len(r.Buchung.Entries) == 0 {
				grund = "keine Buchung"
			}
			c.Skipped = append(c.Skipped, ExportSkip{Dateiname: r.Dateiname, Grund: grund})
			continue
		}
		if r.Exportiert {
			c.AlreadyExported = append(c.AlreadyExported, r)
			if includeExported {
				c.Exportable = append(c.Exportable, r)
			}
			continue
		}
		c.Exportable = append(c.Exportable, r)
	}
	return c
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run TestClassifyForExport && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/core/exportclassify.go internal/core/exportclassify_test.go
git commit -m "Add ClassifyForExport: exportable / already-exported / skipped"
```

---

### Task 3: Export dialog — preview + include-exported + mark

**Files:**
- Modify: `internal/ui/bookingexport.go`
- Test: none (UI; covered by build + manual). 

**Interfaces:**
- Consumes: `core.ClassifyForExport`, `a.dbRepo.MarkExported`, the existing `runBookingExport` build/write flow.
- Produces: `runBookingExport` first classifies the rows, shows a confirm dialog with the breakdown + an "auch bereits exportierte" checkbox, exports only the chosen set, and marks the written rows exported.

- [ ] **Step 1: Insert the preview/confirm before writing**

In `internal/ui/bookingexport.go` `runBookingExport`, after `rows := a.collectInvoiceRows(...)` and before building the files, classify and confirm:

```go
	includeExported := false
	cls := core.ClassifyForExport(rows, includeExported)
	msg := a.bundle.T("export.preview", len(cls.Exportable), len(cls.AlreadyExported), len(cls.Skipped))
	includeCheck := widget.NewCheck(a.bundle.T("export.includeExported"), nil)
	confirm := container.NewVBox(widget.NewLabel(msg), includeCheck)
	dialog.ShowCustomConfirm(a.bundle.T("export.bookings"), a.bundle.T("export.do"), a.bundle.T("btn.cancel"), confirm, func(ok bool) {
		if !ok {
			return
		}
		cls = core.ClassifyForExport(rows, includeCheck.Checked)
		a.writeBookingExport(cls.Exportable, fromY, fromM, toY, toM, period)
	}, a.window)
}
```

Move the existing build/folder-pick/write code into a new `func (a *App) writeBookingExport(exportable []core.CSVRow, fromY, fromM, toY, toM int, period string)` — it builds the DATEV/Lexware files from `exportable` (already filtered, so the exporters' own skip is a no-op), picks the folder, writes both files, and on success marks each exported row:

```go
		for _, r := range exportable {
			if err := a.dbRepo.MarkExported(r.Jahr, r.Monat, r.Dateiname); err != nil {
				a.logger.Warn("MarkExported failed for %s: %v", r.Dateiname, err)
			}
		}
		a.loadInvoices() // reload from DB to reflect the new Exportiert state
```

(`a.loadInvoices()` is the established reload — it re-reads from the DB and calls `a.invoiceTable.SetData(...)`; used in app.go:575/881 and tableedit.go:421/582.)

- [ ] **Step 2: i18n + summary**

Add i18n keys: `export.preview` (de "%d buchbar, %d bereits exportiert, %d übersprungen." / en "%d bookable, %d already exported, %d skipped."), `export.includeExported` (de "Auch bereits exportierte einbeziehen" / en "Include already exported"). Keep the existing `export.done` summary after writing.

- [ ] **Step 3: Build + vet + test**

Run: `go build ./... && go vet ./internal/ui/ && go test ./...`
Expected: clean. Validate both JSONs.

- [ ] **Step 4: Commit**

```bash
git add internal/ui/bookingexport.go assets/i18n/de.json assets/i18n/en.json
git commit -m "Export dialog: preview, include-exported option, mark exported after write"
```

---

### Task 4: "Belege ohne Buchung" quick-filter

**Files:**
- Modify: `internal/ui/table.go`
- Test: none (UI; covered by build + manual). 

**Interfaces:**
- Consumes: the existing quick-filter chip mechanism (`activeChip`, the chip handler that filters `it.filtered`).
- Produces: a chip "Ohne Buchung" that filters the table to rows whose `Buchung` is not balanced (missing or unbalanced).

- [ ] **Step 1: Add the chip + filter branch**

In `internal/ui/table.go`, the quick-filter chips are a literal-labelled list (`chips := []chipDef{...}` ~line 443, entries like `{"teilzahlung", "Teilzahlung"}` — HARDCODED German labels, NOT i18n) and the per-chip filter is a `switch`/`case` on `activeChip` inside `applyFilter` (~line 475, `case "anhang":` / `case "teilzahlung":`). Two changes, matching the existing literal style exactly:
1. Add `{"obuchung", "Ohne Buchung"}` to the `chips` list (literal label, like its neighbours — do NOT introduce an i18n key, to stay consistent with the existing hardcoded chips).
2. In `applyFilter`'s chip `switch`, add `case "obuchung":` that keeps a row only when `!row.Buchung.Balanced()` (mirror the predicate style of the `"teilzahlung"` case).

No i18n keys are added in this task (the chip strip is intentionally literal-German in the current codebase).

- [ ] **Step 2: Build + vet + test**

Run: `go build ./... && go vet ./internal/ui/ && go test ./...`
Expected: clean. Validate both JSONs.

- [ ] **Step 3: Commit**

```bash
git add internal/ui/table.go assets/i18n/de.json assets/i18n/en.json
git commit -m "Add 'Ohne Buchung' quick-filter chip"
```

---

## Self-Review

- **Spec coverage:** double-booking guard (Task 1 flag + Task 3 mark/warn/override), export preview with skip reasons (Tasks 2/3), receipts-without-booking surfaced (Task 4). Covered. Also addresses the E1-final-review note (skipped receipts — incl. unbalanced manual — are now shown by reason in the preview).
- **Placeholder scan:** storage task spells out every column/placeholder/arg change with an explicit count check; UI tasks reference concrete anchors (`runBookingExport`, the chip mechanism) with "find the real reload/chip method" verification steps.
- **Type consistency:** `CSVRow.Exportiert bool`, `MarkExported(jahr,monat,dateiname)`, `ClassifyForExport(rows,includeExported)ExportClassification{Exportable,AlreadyExported,Skipped}`, `ExportSkip{Dateiname,Grund}` — consistent across tasks.
- **Data integrity:** Update always resets the flag (edited invoices re-export); only MarkExported sets it; export uses the SAME exportable rule as the DATEV/Lexware writers; default excludes already-exported (no silent double-export); marking happens only after a successful write.
- **Out of scope:** PDF/UStVA/controlling-table are E3/E4; new categories are E5.
