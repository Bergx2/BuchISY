# USt-Voranmeldung & Controlling-Tabelle (Phase E3) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a USt-Voranmeldung helper (abziehbare Vorsteuer per rate + total for a period) and turn the Controlling view into a properly column-aligned table.

**Architecture:** A pure `ComputeUStVA` sums booking entries posted to the profile's Vorsteuer accounts (from `BookingRules.VorsteuerKonten`), grouped by VAT rate. A UStVA dialog (month/year toggle) shows the per-rate Vorsteuer + total. The Controlling dialog's list is replaced with a `widget.Table` (Konto · Bezeichnung · Summe) using the table-widget pattern already in `table.go`.

**Tech Stack:** Go 1.25, Fyne v2. Reuses D1 bookings, the per-profile `a.bookingRules` (E6), `a.collectInvoiceRows`, D3 `AggregateBookingsByAccount`, and the controlling dialog.

## Global Constraints

- UStVA is computed from the profile's own Vorsteuer accounts (`a.bookingRules.VorsteuerKonten`, e.g. Bergx2 1406/1401 or Boomstraat 1576/1571) — never hard-coded accounts.
- Vorsteuer = sum of booking Soll (debit) entries posted to a Vorsteuer account, grouped by the rate that maps to that account. Output VAT (Umsatzsteuer) is out of scope (the booking engine books expense receipts; revenue/output-tax handling is a later phase) — the dialog is labelled "abziehbare Vorsteuer".
- Money `float64`, round2, German decimal comma in the UI.
- Reuse the `export.month`/`export.year` i18n labels for the period toggles (already exist).
- All new user-facing strings via `a.bundle.T(...)` in both JSONs (valid JSON).
- `go build ./... && go test ./... && go vet ./internal/ui/` clean (ignore the pre-existing `clipboard_windows.go:206` warning). Commit per task.

---

### Task 1: ComputeUStVA (core)

**Files:**
- Create: `internal/core/ustva.go`
- Test: `internal/core/ustva_test.go`

**Interfaces:**
- Consumes: `CSVRow.Buchung` (`DebitEntries`), `BookingRules.VorsteuerKonten`.
- Produces: `type UStVAZeile struct { Satz int; Konto int; Vorsteuer float64 }`;
  `type UStVA struct { Zeilen []UStVAZeile; VorsteuerGesamt float64 }`;
  `ComputeUStVA(rows []CSVRow, rules *BookingRules) UStVA` — sums booking Soll entries posted to a Vorsteuer account, grouped by the rate that maps to that account; `Zeilen` sorted ascending by `Satz`, plus the grand total. Rules with no `VorsteuerKonten` yield an empty result.

- [ ] **Step 1: Write the failing test**

```go
package core

import "testing"

func TestComputeUStVA(t *testing.T) {
	rules, _ := ParseBookingRules([]byte(`{"vorsteuer_konten":{"19":1406,"7":1401},"regeln":[]}`))
	rows := []CSVRow{
		{Buchung: Booking{Entries: []BookingEntry{{Konto: 6640, Betrag: 100, Soll: true}, {Konto: 1406, Betrag: 19, Soll: true}, {Konto: 1800, Betrag: 119, Soll: false}}}},
		{Buchung: Booking{Entries: []BookingEntry{{Konto: 6815, Betrag: 50, Soll: true}, {Konto: 1401, Betrag: 3.50, Soll: true}, {Konto: 1800, Betrag: 53.50, Soll: false}}}},
		{Buchung: Booking{Entries: []BookingEntry{{Konto: 1406, Betrag: 1, Soll: true}, {Konto: 1800, Betrag: 1, Soll: false}}}},
	}
	u := ComputeUStVA(rows, rules)
	if !almost(u.VorsteuerGesamt, 23.50) {
		t.Errorf("total = %v, want 23.50", u.VorsteuerGesamt)
	}
	if len(u.Zeilen) != 2 {
		t.Fatalf("want 2 rate lines, got %d: %+v", len(u.Zeilen), u.Zeilen)
	}
	// sorted ascending by Satz → 7% first
	if u.Zeilen[0].Satz != 7 || !almost(u.Zeilen[0].Vorsteuer, 3.50) {
		t.Errorf("7%% line = %+v", u.Zeilen[0])
	}
	if u.Zeilen[1].Satz != 19 || !almost(u.Zeilen[1].Vorsteuer, 20.00) || u.Zeilen[1].Konto != 1406 {
		t.Errorf("19%% line = %+v (want 20.00 on 1406)", u.Zeilen[1])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run TestComputeUStVA`
Expected: FAIL (undefined ComputeUStVA). `almost` already exists.

- [ ] **Step 3: Implement**

Create `internal/core/ustva.go`:

```go
package core

import (
	"sort"
	"strconv"
)

// UStVAZeile is the deductible input VAT (Vorsteuer) for one VAT rate.
type UStVAZeile struct {
	Satz      int     // VAT percent (19, 7, …)
	Konto     int     // the Vorsteuer account for that rate
	Vorsteuer float64 // summed input VAT booked to that account
}

// UStVA is the deductible-input-VAT summary for a period.
type UStVA struct {
	Zeilen          []UStVAZeile
	VorsteuerGesamt float64
}

// ComputeUStVA sums the booking Soll entries posted to the profile's Vorsteuer
// accounts (rules.VorsteuerKonten), grouped by VAT rate.
func ComputeUStVA(rows []CSVRow, rules *BookingRules) UStVA {
	// Reverse map: account → rate.
	rateByKonto := map[int]int{}
	for satz, konto := range rules.VorsteuerKonten {
		if s, err := strconv.Atoi(satz); err == nil {
			rateByKonto[konto] = s
		}
	}
	sumByRate := map[int]float64{}
	kontoByRate := map[int]int{}
	for _, r := range rows {
		for _, e := range r.Buchung.DebitEntries() {
			if satz, ok := rateByKonto[e.Konto]; ok {
				sumByRate[satz] += e.Betrag
				kontoByRate[satz] = e.Konto
			}
		}
	}
	var u UStVA
	for satz, summe := range sumByRate {
		summe = round2(summe)
		u.Zeilen = append(u.Zeilen, UStVAZeile{Satz: satz, Konto: kontoByRate[satz], Vorsteuer: summe})
		u.VorsteuerGesamt += summe
	}
	sort.Slice(u.Zeilen, func(i, j int) bool { return u.Zeilen[i].Satz < u.Zeilen[j].Satz })
	u.VorsteuerGesamt = round2(u.VorsteuerGesamt)
	return u
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run TestComputeUStVA && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/core/ustva.go internal/core/ustva_test.go
git commit -m "Add ComputeUStVA: deductible input VAT per rate"
```

---

### Task 2: UStVA dialog

**Files:**
- Create: `internal/ui/ustvaview.go`
- Modify: `internal/ui/app.go` (menu item)
- Test: none (UI; covered by build + manual). 

**Interfaces:**
- Consumes: `a.collectInvoiceRows`, `core.ComputeUStVA`, `a.bookingRules`, `a.currentYear`, `a.currentMonth`.
- Produces: `func (a *App) showUStVADialog()` opened from a menu item; month/year toggle; per-rate Vorsteuer + total.

- [ ] **Step 1: Build the dialog (model on controllingview.go)**

Create `internal/ui/ustvaview.go`. Mirror `internal/ui/controllingview.go`'s structure (a `widget.NewList`, a `widget.NewRadioGroup` month/year toggle reusing `export.month`/`export.year`, a `reload` closure, a total label):

```go
package ui

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/bergx2/buchisy/internal/core"
)

// showUStVADialog shows the deductible input VAT (Vorsteuer) per rate for the
// current month or whole year.
func (a *App) showUStVADialog() {
	var u core.UStVA
	yearMode := false

	list := widget.NewList(
		func() int { return len(u.Zeilen) },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			z := u.Zeilen[i]
			amount := strings.Replace(fmt.Sprintf("%.2f", z.Vorsteuer), ".", ",", 1)
			o.(*widget.Label).SetText(fmt.Sprintf("%s %d %%   (%d)   %s", a.bundle.T("ustva.vorsteuer"), z.Satz, z.Konto, amount))
		},
	)
	totalLabel := widget.NewLabel("")

	reload := func() {
		fromY, fromM, toY, toM := a.currentYear, int(a.currentMonth), a.currentYear, int(a.currentMonth)
		if yearMode {
			fromM, toM = 1, 12
		}
		rows := a.collectInvoiceRows(fromY, fromM, toY, toM)
		u = core.ComputeUStVA(rows, a.bookingRules)
		amount := strings.Replace(fmt.Sprintf("%.2f", u.VorsteuerGesamt), ".", ",", 1)
		totalLabel.SetText(a.bundle.T("ustva.total", amount))
		list.Refresh()
	}

	toggle := widget.NewRadioGroup([]string{a.bundle.T("export.month"), a.bundle.T("export.year")}, func(sel string) {
		yearMode = sel == a.bundle.T("export.year")
		reload()
	})
	toggle.Horizontal = true
	toggle.SetSelected(a.bundle.T("export.month"))
	reload()

	header := widget.NewLabelWithStyle(a.bundle.T("ustva.heading"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	content := container.NewBorder(container.NewVBox(header, toggle), totalLabel, nil, nil, list)
	d := dialog.NewCustom(a.bundle.T("ustva.title"), a.bundle.T("common.close"), content, a.window)
	d.Resize(fyne.NewSize(460, 380))
	d.Show()
}
```

Add i18n keys (both JSONs, valid): `ustva.title` (de "USt-Voranmeldung" / en "VAT return"), `ustva.heading` (de "Abziehbare Vorsteuer" / en "Deductible input VAT"), `ustva.vorsteuer` (de "Vorsteuer" / en "Input VAT"), `ustva.total` (de "Summe Vorsteuer: %s €" / en "Total input VAT: %s €").

- [ ] **Step 2: Menu item**

In `internal/ui/app.go`, next to the `fyne.NewMenuItem("Controlling", …)` item, add `fyne.NewMenuItem("USt-Voranmeldung", func() { a.showUStVADialog() })` (literal German, matching neighbours).

- [ ] **Step 3: Build + vet + test**

Run: `go build ./... && go vet ./internal/ui/ && go test ./...`
Expected: clean. Validate both JSONs.

- [ ] **Step 4: Commit**

```bash
git add internal/ui/ustvaview.go internal/ui/app.go assets/i18n/de.json assets/i18n/en.json
git commit -m "Add USt-Voranmeldung dialog (deductible input VAT per rate)"
```

---

### Task 3: Controlling as a table

**Files:**
- Modify: `internal/ui/controllingview.go`
- Test: none (UI; covered by build + manual). 

**Interfaces:**
- Consumes: the existing `showControllingDialog` state (`sums []core.AccountSum`, `reload`), `widget.Table`.
- Produces: the controlling dialog renders `sums` in a 3-column `widget.Table` (Konto · Bezeichnung · Summe) instead of a single-label list.

- [ ] **Step 1: Replace the list with a table**

In `internal/ui/controllingview.go`, replace the `widget.NewList(...)` (and its `list.Refresh()` in `reload`) with a `widget.NewTable`. The table reads the SAME `sums` slice. Pattern:

```go
	table := widget.NewTable(
		func() (int, int) { return len(sums), 3 },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(id widget.TableCellID, o fyne.CanvasObject) {
			lbl := o.(*widget.Label)
			s := sums[id.Row]
			switch id.Col {
			case 0:
				lbl.SetText(fmt.Sprintf("%d", s.Konto))
			case 1:
				lbl.SetText(s.Name)
			default:
				lbl.Alignment = fyne.TextAlignTrailing
				lbl.SetText(strings.Replace(fmt.Sprintf("%.2f", s.Summe), ".", ",", 1))
			}
		},
	)
	table.SetColumnWidth(0, 70)
	table.SetColumnWidth(1, 300)
	table.SetColumnWidth(2, 110)
```

In `reload`, replace `list.Refresh()` with `table.Refresh()`. In the `container.NewBorder(...)`, use `table` as the center object instead of `list`. (Optionally set `table.ShowHeaderRow = true` + a `CreateHeader`/`UpdateHeader` showing "Konto"/"Bezeichnung"/"Summe" — mirror `it.table.ShowHeaderRow`/`CreateHeader` in `table.go` if you add headers; headers are optional, the column widths already align the data.)

- [ ] **Step 2: Build + vet + test**

Run: `go build ./... && go vet ./internal/ui/ && go test ./...`
Expected: clean.

- [ ] **Step 3: Commit**

```bash
git add internal/ui/controllingview.go
git commit -m "Render Controlling as an aligned 3-column table"
```

---

## Self-Review

- **Spec coverage:** UStVA prep (abziehbare Vorsteuer per rate + total) via the profile's own Vorsteuer accounts (Tasks 1/2); Controlling as an aligned table (Task 3). The E3-chosen "USt-VA + Controlling-Tabelle" is covered.
- **Placeholder scan:** Task 1 fully coded; UI tasks mirror concrete existing files (`controllingview.go`, the `table.go` `widget.NewTable` pattern) with full snippets.
- **Type consistency:** `UStVAZeile{Satz,Konto,Vorsteuer}`, `UStVA{Zeilen,VorsteuerGesamt}`, `ComputeUStVA(rows,rules)`, `showUStVADialog()` — consistent.
- **Data integrity:** Vorsteuer accounts come from the live profile rules; sums are booking-derived, rounded; output VAT explicitly out of scope and the dialog says "abziehbare Vorsteuer".
- **Out of scope:** PDF reports (E4); backup + new categories (E5); output-VAT/Umsatzsteuer (the booking engine handles expense receipts only).
