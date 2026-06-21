# Controlling-Auswertung (Phase D3) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A Controlling view that sums the booked amounts per SKR04 account over a chosen month or year, so the user sees e.g. "6640 Bewirtungskosten: 1.240,00" at a glance.

**Architecture:** A pure-core aggregation turns the stored bookings of a period (`[]core.CSVRow`, each with its `Booking` from D1) into a sorted list of per-account sums (the Soll/debit side — expense + Vorsteuer accounts). A dialog with a month/year toggle collects the period's rows, aggregates, and shows a table (Konto · Bezeichnung · Summe) with a grand total.

**Tech Stack:** Go 1.25, Fyne v2. Reuses D1 bookings, Phase B `a.chart` (account names), the existing `collectInvoiceRows` and dialog patterns.

## Global Constraints

- Core aggregation is pure (`internal/core`), no UI/DB deps; the dialog only collects rows + renders.
- Aggregate the Soll (debit) entries of every booking that HAS entries — these are the expense and Vorsteuer postings. The Haben (Zahlungskonto) side is NOT part of this Controlling view.
- Amounts summed as `float64`, rendered with German decimal comma. Account names come from `a.chart` (bare number when not found).
- Sorted ascending by account number; include a grand total of all summed Soll amounts.
- No invented data — accounts/amounts come verbatim from the stored bookings.
- All user-facing strings via `a.bundle.T(...)` in both JSONs (valid JSON).
- `go build ./... && go test ./... && go vet ./internal/ui/` clean (ignore the pre-existing `clipboard_windows.go:206` warning). Commit per task.

---

### Task 1: Aggregate bookings by account (core)

**Files:**
- Create: `internal/core/controlling.go`
- Test: `internal/core/controlling_test.go`

**Interfaces:**
- Consumes: `CSVRow.Buchung`, `Booking.DebitEntries()`, `ChartOfAccounts.Find`.
- Produces: `type AccountSum struct { Konto int; Name string; Summe float64 }`;
  `AggregateBookingsByAccount(rows []CSVRow, chart *ChartOfAccounts) (sums []AccountSum, total float64)` — sums each booking's Soll-entry amounts grouped by `Konto`, attaches the chart name (empty when chart is nil or not found), returns the list sorted ascending by `Konto` plus the grand total. Rows whose booking has no entries contribute nothing.

- [ ] **Step 1: Write the failing test**

```go
package core

import "testing"

func TestAggregateBookingsByAccount(t *testing.T) {
	chart := NewChartOfAccounts([]SKRAccount{
		{Number: 6640, Name: "Bewirtungskosten (abziehbar)"},
		{Number: 1406, Name: "Abziehbare Vorsteuer 19%"},
	})
	rows := []CSVRow{
		{Buchung: Booking{Entries: []BookingEntry{
			{Konto: 6640, Betrag: 12.71, Soll: true},
			{Konto: 1406, Betrag: 1.26, Soll: true},
			{Konto: 1800, Betrag: 13.97, Soll: false}, // Haben — excluded
		}}},
		{Buchung: Booking{Entries: []BookingEntry{
			{Konto: 6640, Betrag: 7.29, Soll: true},
			{Konto: 1800, Betrag: 7.29, Soll: false},
		}}},
		{Buchung: Booking{}}, // no entries — contributes nothing
	}
	sums, total := AggregateBookingsByAccount(rows, chart)
	if len(sums) != 2 {
		t.Fatalf("want 2 accounts, got %d: %+v", len(sums), sums)
	}
	// sorted ascending by Konto → 1406 first, then 6640
	if sums[0].Konto != 1406 || !almost(sums[0].Summe, 1.26) || sums[0].Name != "Abziehbare Vorsteuer 19%" {
		t.Errorf("sums[0] = %+v", sums[0])
	}
	if sums[1].Konto != 6640 || !almost(sums[1].Summe, 20.00) {
		t.Errorf("6640 sum = %+v (want 20.00)", sums[1])
	}
	if !almost(total, 21.26) {
		t.Errorf("total = %v (want 21.26)", total)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run TestAggregateBookingsByAccount`
Expected: FAIL (undefined AggregateBookingsByAccount). `almost` already exists in the package.

- [ ] **Step 3: Implement**

Create `internal/core/controlling.go`:

```go
package core

import "sort"

// AccountSum is the total Soll amount booked to one SKR04 account in a period.
type AccountSum struct {
	Konto int
	Name  string
	Summe float64
}

// AggregateBookingsByAccount sums the Soll (debit) entries of every booking in
// rows, grouped by account number, and returns the per-account sums sorted by
// account plus the grand total. Names are filled from chart (empty if chart is
// nil or the account is unknown). Bookings without entries contribute nothing.
func AggregateBookingsByAccount(rows []CSVRow, chart *ChartOfAccounts) ([]AccountSum, float64) {
	byKonto := map[int]float64{}
	for _, r := range rows {
		for _, e := range r.Buchung.DebitEntries() {
			byKonto[e.Konto] += e.Betrag
		}
	}
	sums := make([]AccountSum, 0, len(byKonto))
	var total float64
	for konto, summe := range byKonto {
		summe = round2(summe)
		name := ""
		if chart != nil {
			if acc, ok := chart.Find(konto); ok {
				name = acc.Name
			}
		}
		sums = append(sums, AccountSum{Konto: konto, Name: name, Summe: summe})
		total += summe
	}
	sort.Slice(sums, func(i, j int) bool { return sums[i].Konto < sums[j].Konto })
	return sums, round2(total)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run TestAggregateBookingsByAccount && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/core/controlling.go internal/core/controlling_test.go
git commit -m "Add per-account booking aggregation for Controlling"
```

---

### Task 2: Controlling dialog (month/year toggle + table)

**Files:**
- Create: `internal/ui/controllingview.go`
- Modify: `internal/ui/app.go` (menu item)
- Test: none (UI; covered by build + manual). 

**Interfaces:**
- Consumes: `a.collectInvoiceRows(fromY, fromM, toY, toM)`, `core.AggregateBookingsByAccount`, `a.chart`, `a.currentYear`, `a.currentMonth`.
- Produces: `func (a *App) showControllingDialog()` — a dialog with a month/year segmented toggle and a table of `Konto · Bezeichnung · Summe` + a total line; opened from a menu item.

- [ ] **Step 1: Build the dialog**

Create `internal/ui/controllingview.go`. Model the table on the existing list/table widgets in the package (see `internal/ui/table.go` / `internal/ui/kontenview.go` for the `widget.NewTable` pattern). Structure:

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

// showControllingDialog shows per-account booked sums for the current month or
// the whole current year, toggled by a segmented control.
func (a *App) showControllingDialog() {
	var sums []core.AccountSum
	var total float64
	yearMode := false

	list := widget.NewList(
		func() int { return len(sums) },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			s := sums[i]
			amount := strings.Replace(fmt.Sprintf("%.2f", s.Summe), ".", ",", 1)
			o.(*widget.Label).SetText(fmt.Sprintf("%d  %s   %s", s.Konto, s.Name, amount))
		},
	)
	totalLabel := widget.NewLabel("")

	reload := func() {
		fromY, fromM, toY, toM := a.currentYear, int(a.currentMonth), a.currentYear, int(a.currentMonth)
		if yearMode {
			fromM, toM = 1, 12
		}
		rows := a.collectInvoiceRows(fromY, fromM, toY, toM)
		sums, total = core.AggregateBookingsByAccount(rows, a.chart)
		amount := strings.Replace(fmt.Sprintf("%.2f", total), ".", ",", 1)
		totalLabel.SetText(a.bundle.T("controlling.total", amount))
		list.Refresh()
	}

	toggle := widget.NewRadioGroup([]string{a.bundle.T("export.month"), a.bundle.T("export.year")}, func(sel string) {
		yearMode = sel == a.bundle.T("export.year")
		reload()
	})
	toggle.Horizontal = true
	toggle.SetSelected(a.bundle.T("export.month"))
	reload()

	header := widget.NewLabelWithStyle(a.bundle.T("controlling.heading"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	content := container.NewBorder(container.NewVBox(header, toggle), totalLabel, nil, nil, list)
	d := dialog.NewCustom(a.bundle.T("controlling.title"), a.bundle.T("common.close"), content, a.window)
	d.Resize(fyne.NewSize(520, 480))
	d.Show()
}
```

(Reuse the existing `export.month` / `export.year` i18n keys from D2 for the toggle, and `common.close` ("Schließen") for the dialog dismiss button — both already exist.)

Add i18n keys to BOTH JSONs (valid): `controlling.title` (de "Controlling" / en "Controlling"), `controlling.heading` (de "Gebuchte Summen je Konto" / en "Booked sums per account"), `controlling.total` (de "Summe: %s €" / en "Total: %s €").

- [ ] **Step 2: Wire the menu item**

In `internal/ui/app.go`, next to the `fyne.NewMenuItem("Buchungen exportieren", …)` item (added in D2), add `fyne.NewMenuItem("Controlling", func() { a.showControllingDialog() })` (match the literal-German style of the neighbouring items).

- [ ] **Step 3: Build + vet + test**

Run: `go build ./... && go vet ./internal/ui/ && go test ./...`
Expected: clean (only the pre-existing clipboard warning). Validate both JSONs parse.

- [ ] **Step 4: Commit**

```bash
git add internal/ui/controllingview.go internal/ui/app.go assets/i18n/de.json assets/i18n/en.json
git commit -m "Add Controlling dialog with per-account sums (month/year)"
```

---

## Self-Review

- **Spec coverage (Phase D / Baustein D — Controlling part):** per-account booked sums over a month or year (Tasks 1/2), with account names from the chart and a grand total; month/year toggle (Task 2). This is the user-chosen "Konten-/Kategorie-Auswertung".
- **Placeholder scan:** Task 2 references concrete reuse points (`collectInvoiceRows`, the `export.month/year` keys, the table/list widgets in `table.go`/`kontenview.go`) and provides the full dialog code; the only verification note (exact close-button key) is an explicit check, not a vague directive.
- **Type consistency:** `AccountSum{Konto,Name,Summe}`, `AggregateBookingsByAccount([]CSVRow,*ChartOfAccounts)([]AccountSum,float64)`, `showControllingDialog()` — consistent across the two tasks.
- **Data integrity:** sums only the Soll entries of stored bookings; bookings without entries contribute nothing; amounts rounded to 2 decimals; sorted by account; names from the chart (bare number when unknown). No invented data.
- **Scope:** Controlling shows the Soll side (expense + Vorsteuer accounts). The Zahlungskonto/Haben side and any netto-vs-brutto split are intentionally out of scope (YAGNI) — each account's single sum already answers "wie viel wurde auf Konto X gebucht".
