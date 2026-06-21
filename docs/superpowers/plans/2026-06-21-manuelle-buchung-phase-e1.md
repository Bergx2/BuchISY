# Manuelle Buchungs-Anpassung (Phase E1) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let the user hand-edit the proposed Buchungssatz (Soll/Haben lines) before saving, mark it "manuell", and have that manual booking preserved instead of regenerated on later edits.

**Architecture:** A new `Booking.Manuell` flag distinguishes a hand-edited booking from an auto-built one. A dialog `showBookingEditor` presents the booking as editable rows (account picker · amount · Soll/Haben · remove) with a live balance check and returns a `Booking{Manuell:true}`. Both invoice dialogs gain a "Manuell anpassen…" button: while a manual booking is set, the live auto-recompute is suppressed and the preview shows the manual booking; a "Automatisch" button clears it. On save the manual booking (if any) is stored verbatim; the edit dialog seeds itself from a stored manual booking.

**Tech Stack:** Go 1.25, Fyne v2. Reuses `core.Booking`/`BookingEntry`, Phase B `a.chart`/`showAccountSearch`/`accountLabel`, the D1 `bookingPreview`, and the `taxLinesEditor` row pattern.

## Global Constraints

- `Booking.Manuell bool` (json `manuell,omitempty`) round-trips through the existing `MarshalBooking`/`ParseBooking` (JSON) — no new persistence columns.
- A manual booking is stored VERBATIM (entries as the user left them); never overwritten by the auto-builder while `Manuell` is set.
- NEVER invent accounts/amounts — manual rows come from the user; the account picker uses `a.chart`.
- The editor shows a live balance indicator (Σ Soll vs Σ Haben). Saving a manual booking is allowed even if unbalanced, but the imbalance is clearly shown (the user is responsible). Auto bookings remain always-balanced.
- When a manual booking is active, the company→template learning is skipped (a hand-tuned booking is not a reusable template).
- All user-facing strings via `a.bundle.T(...)` in both JSONs (valid JSON).
- `go build ./... && go test ./... && go vet ./internal/ui/` clean (ignore the pre-existing `clipboard_windows.go:206` warning). Commit per task.

---

### Task 1: Booking.Manuell flag

**Files:**
- Modify: `internal/core/buchung.go` (struct field)
- Test: `internal/core/buchung_test.go`

**Interfaces:**
- Produces: `Booking.Manuell bool` (json `manuell,omitempty`); round-trips via `MarshalBooking`/`ParseBooking`.

- [ ] **Step 1: Write the failing test**

```go
func TestBookingManuellRoundTrip(t *testing.T) {
	b := Booking{Manuell: true, Entries: []BookingEntry{{Konto: 6640, Betrag: 10, Soll: true}, {Konto: 1800, Betrag: 10, Soll: false}}}
	got := ParseBooking(MarshalBooking(b))
	if !got.Manuell {
		t.Error("Manuell flag did not round-trip")
	}
	if len(got.Entries) != 2 {
		t.Errorf("entries lost: %+v", got)
	}
	// an auto booking (Manuell=false) stays false
	if ParseBooking(MarshalBooking(Booking{Entries: b.Entries})).Manuell {
		t.Error("non-manual booking should stay false")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run TestBookingManuellRoundTrip`
Expected: FAIL (unknown field Manuell).

- [ ] **Step 3: Implement**

In `internal/core/buchung.go`, add the field to `Booking` (after `Info`):

```go
	Manuell bool `json:"manuell,omitempty"` // true = hand-edited, not auto-generated
```

(`IsEmpty()` should still treat a booking with no entries + no info as empty; leave it unchanged — a `Manuell` flag without entries is not a meaningful booking. Verify `IsEmpty` does not need to consider `Manuell`: an empty manual booking carries nothing to store, so the existing `len(Entries)==0 && Info==""` is correct.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run TestBookingManuellRoundTrip && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/core/buchung.go internal/core/buchung_test.go
git commit -m "Add Booking.Manuell flag for hand-edited bookings"
```

---

### Task 2: Booking editor dialog

**Files:**
- Create: `internal/ui/bookingeditor.go`
- Test: `internal/ui/bookingeditor_test.go`

**Interfaces:**
- Consumes: `core.Booking`/`BookingEntry`, `a.chart`, `a.showAccountSearch`, `accountLabel`, `a.bundle`.
- Produces: `bookingFromRows(rows []bookingEditRow) core.Booking` (pure: builds a `Booking{Manuell:true}` from row values, parsing each amount); `type bookingEditRow struct { Konto int; Betrag float64; Soll bool }`;
  `func (a *App) showBookingEditor(current core.Booking, onSave func(core.Booking))` — a dialog editing `current`'s entries; OK calls `onSave` with a `Booking{Manuell:true, Entries:…}`.

- [ ] **Step 1: Write the failing test (pure builder)**

```go
package ui

import (
	"testing"

	"github.com/bergx2/buchisy/internal/core"
)

func TestBookingFromRows(t *testing.T) {
	rows := []bookingEditRow{
		{Konto: 6640, Betrag: 12.71, Soll: true},
		{Konto: 1800, Betrag: 12.71, Soll: false},
	}
	b := bookingFromRows(rows)
	if !b.Manuell {
		t.Error("manual flag not set")
	}
	if len(b.Entries) != 2 || b.Entries[0].Konto != 6640 || !b.Entries[0].Soll {
		t.Fatalf("entries wrong: %+v", b.Entries)
	}
	if !b.Balanced() {
		t.Errorf("12,71 S vs 12,71 H should balance: %+v", b)
	}
	// rows with Konto==0 are dropped (incomplete)
	if len(bookingFromRows([]bookingEditRow{{Konto: 0, Betrag: 5, Soll: true}}).Entries) != 0 {
		t.Error("zero-account row should be dropped")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ui/ -run TestBookingFromRows`
Expected: FAIL (undefined bookingFromRows).

- [ ] **Step 3: Implement**

Create `internal/ui/bookingeditor.go`. Model the per-row controls and rebuild on the `taxLinesEditor` pattern in `internal/ui/taxlineseditor.go`.

```go
package ui

import (
	"fmt"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"github.com/bergx2/buchisy/internal/core"
)

// bookingEditRow is the edited value of one booking line.
type bookingEditRow struct {
	Konto  int
	Betrag float64
	Soll   bool
}

// bookingFromRows builds a manual Booking from editor rows. Rows without an
// account (Konto==0) are dropped as incomplete.
func bookingFromRows(rows []bookingEditRow) core.Booking {
	entries := make([]core.BookingEntry, 0, len(rows))
	for _, r := range rows {
		if r.Konto == 0 {
			continue
		}
		entries = append(entries, core.BookingEntry{Konto: r.Konto, Betrag: r.Betrag, Soll: r.Soll})
	}
	return core.Booking{Manuell: true, Entries: entries}
}

// parseDecimal reads a German/English decimal string ("12,71" or "12.71").
func parseDecimal(s string) float64 {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", "."))
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

// showBookingEditor opens a dialog to hand-edit a booking's Soll/Haben lines.
// On OK it calls onSave with a Booking{Manuell:true}.
func (a *App) showBookingEditor(current core.Booking, onSave func(core.Booking)) {
	rows := make([]bookingEditRow, 0, len(current.Entries))
	for _, e := range current.Entries {
		rows = append(rows, bookingEditRow{Konto: e.Konto, Betrag: e.Betrag, Soll: e.Soll})
	}
	if len(rows) == 0 {
		rows = append(rows, bookingEditRow{Soll: true})
	}

	rowsBox := container.NewVBox()
	balanceLabel := widget.NewLabel("")
	var rebuild func()

	refreshBalance := func() {
		b := bookingFromRows(rows)
		amount := strings.Replace(fmt.Sprintf("%.2f", b.SollSum()-b.HabenSum()), ".", ",", 1)
		if b.Balanced() {
			balanceLabel.SetText(a.bundle.T("booking.balanced"))
		} else {
			balanceLabel.SetText(a.bundle.T("booking.editor.diff", amount))
		}
	}

	rebuild = func() {
		rowsBox.RemoveAll()
		for i := range rows {
			i := i
			r := &rows[i]
			kontoLabel := widget.NewLabel(a.bookingKontoLabel(r.Konto))
			pickBtn := widget.NewButton("…", func() {
				a.showAccountSearch(r.Konto, func(n int) {
					r.Konto = n
					kontoLabel.SetText(a.bookingKontoLabel(n))
					refreshBalance()
				})
			})
			betrag := widget.NewEntry()
			betrag.SetText(strings.Replace(fmt.Sprintf("%.2f", r.Betrag), ".", ",", 1))
			betrag.OnChanged = func(s string) { r.Betrag = parseDecimal(s); refreshBalance() }
			sh := widget.NewSelect([]string{a.bundle.T("booking.soll"), a.bundle.T("booking.haben")}, func(sel string) {
				r.Soll = sel == a.bundle.T("booking.soll")
				refreshBalance()
			})
			if r.Soll {
				sh.SetSelected(a.bundle.T("booking.soll"))
			} else {
				sh.SetSelected(a.bundle.T("booking.haben"))
			}
			del := widget.NewButton("✕", func() {
				rows = append(rows[:i], rows[i+1:]...)
				rebuild()
				refreshBalance()
			})
			kontoCell := container.NewBorder(nil, nil, nil, pickBtn, kontoLabel)
			row := container.NewBorder(nil, nil, nil, del,
				container.New(layout.NewGridLayoutWithColumns(3), kontoCell, betrag, sh))
			rowsBox.Add(row)
		}
		rowsBox.Refresh()
	}

	addBtn := widget.NewButton(a.bundle.T("booking.editor.add"), func() {
		rows = append(rows, bookingEditRow{Soll: true})
		rebuild()
		refreshBalance()
	})

	rebuild()
	refreshBalance()

	content := container.NewBorder(nil, container.NewVBox(addBtn, balanceLabel), nil, nil,
		container.NewVScroll(rowsBox))
	d := dialog.NewCustomConfirm(a.bundle.T("booking.editor.title"), a.bundle.T("export.do"), a.bundle.T("btn.cancel"),
		content, func(ok bool) {
			if ok {
				onSave(bookingFromRows(rows))
			}
		}, a.window)
	d.Resize(fyne.NewSize(520, 460))
	d.Show()
}

// bookingKontoLabel renders an account for the editor (number — name, or a
// placeholder when unset).
func (a *App) bookingKontoLabel(konto int) string {
	if konto == 0 {
		return a.bundle.T("booking.editor.pickaccount")
	}
	if acc, ok := a.chart.Find(konto); ok {
		return accountLabel(acc)
	}
	return fmt.Sprintf("%d", konto)
}
```

Add i18n keys to BOTH JSONs (valid): `booking.editor.title` (de "Buchung manuell bearbeiten" / en "Edit booking manually"), `booking.editor.add` (de "+ Zeile" / en "+ Line"), `booking.editor.diff` (de "Differenz: %s €" / en "Difference: %s €"), `booking.editor.pickaccount` (de "Konto wählen…" / en "Pick account…"), `booking.soll` (de "Soll" / en "Debit"), `booking.haben` (de "Haben" / en "Credit"). (`booking.balanced`, `export.do`, `btn.cancel` already exist.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/ui/ -run TestBookingFromRows && go build ./... && go vet ./internal/ui/`
Expected: PASS, clean. Validate both JSONs.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/bookingeditor.go internal/ui/bookingeditor_test.go assets/i18n/de.json assets/i18n/en.json
git commit -m "Add manual booking editor dialog"
```

---

### Task 3: Manual booking in the confirmation modal

**Files:**
- Modify: `internal/ui/invoicemodal.go`
- Test: none (UI; covered by build + manual). 

**Interfaces:**
- Consumes: `a.showBookingEditor`, the existing `bookingPrev`/`recomputeBooking`/`computeInvoiceBooking`, `selectedAccount`, `bankAccountSelect`, `ed`.
- Produces: a "Manuell anpassen…" + "Automatisch" control; on save the manual booking (if active) is stored and template learning is skipped.

- [ ] **Step 1: Add manual state + buttons, suppress recompute**

Near the booking-preview construction (`bookingPrev := newBookingPreview(a)`, ~line 392), add:

```go
	var manualBooking *core.Booking
```

Change the `recomputeBooking` closure so it shows the manual booking when set instead of recomputing:

```go
	recomputeBooking = func() {
		if manualBooking != nil {
			bookingPrev.set(*manualBooking, manualBooking.Balanced(), a.bundle.T("booking.manual.hint"))
			return
		}
		b, bookable, reason := a.computeInvoiceBooking(catKeyByLabel[categorySelect.Selected], ed.Lines(), ed.Trinkgeld(), selectedAccount, bankAccountSelect.Selected)
		bookingPrev.set(b, bookable, reason)
	}
```

(Adapt to the actual existing closure body — the key change is the `if manualBooking != nil` early branch.)

Add two buttons placed beside the preview:

```go
	editBookingBtn := widget.NewButton(a.bundle.T("booking.editor.title"), func() {
		seed := manualBooking
		if seed == nil {
			b, _, _ := a.computeInvoiceBooking(catKeyByLabel[categorySelect.Selected], ed.Lines(), ed.Trinkgeld(), selectedAccount, bankAccountSelect.Selected)
			seed = &b
		}
		a.showBookingEditor(*seed, func(edited core.Booking) {
			manualBooking = &edited
			recomputeBooking()
		})
	})
	autoBookingBtn := widget.NewButton(a.bundle.T("booking.auto"), func() {
		manualBooking = nil
		recomputeBooking()
	})
```

Place `editBookingBtn` and `autoBookingBtn` in the booking section row (next to `categorySelect`).

- [ ] **Step 2: Persist the manual booking on save**

At the save handler where the booking is computed for `saveInvoice` (~line 582), branch on the manual state:

```go
	var finalBooking core.Booking
	learn := false
	if manualBooking != nil {
		finalBooking = *manualBooking
	} else {
		b, bookable, _ := a.computeInvoiceBooking(catKeyByLabel[categorySelect.Selected], ed.Lines(), ed.Trinkgeld(), selectedAccount, bankAccountSelect.Selected)
		if bookable {
			finalBooking = b
			learn = true
		}
	}
	// ... pass finalBooking into saveInvoice ...
	if learn && companyEntry.Text != "" {
		_ = a.bookingTemplates.Set(companyEntry.Text, core.BookingTemplate{Kategorie: catKeyByLabel[categorySelect.Selected], ExpenseKonto: selectedAccount})
	}
```

(Replace the existing compute-and-learn block at the save handler — currently at invoicemodal.go:~616-621 which already uses `companyEntry.Text` as the template key — with this manual/auto-branching version; keep passing `finalBooking` to `saveInvoice` exactly as the booking was passed before. Use the SAME company key the existing code uses (`companyEntry.Text`).)

Add i18n keys: `booking.auto` (de "Automatisch" / en "Automatic"), `booking.manual.hint` (de "Manuell bearbeitete Buchung" / en "Manually edited booking").

- [ ] **Step 3: Build + vet + test + manual smoke**

Run: `go build ./... && go vet ./internal/ui/ && go test ./...`
Expected: clean. Validate both JSONs.

- [ ] **Step 4: Commit**

```bash
git add internal/ui/invoicemodal.go assets/i18n/de.json assets/i18n/en.json
git commit -m "Allow manual booking edit in the confirmation modal"
```

---

### Task 4: Manual booking in the edit dialog

**Files:**
- Modify: `internal/ui/tableedit.go`
- Test: none (UI; covered by build + manual). 

**Interfaces:**
- Consumes: the same helpers + the loaded `row.Buchung`.
- Produces: a stored manual booking (`row.Buchung.Manuell`) seeds the editor and is preserved; same buttons; same save logic via `updateInvoice`.

- [ ] **Step 1: Seed manual state from the stored booking**

In `showEditDialog`, near the booking-preview construction, initialize:

```go
	var manualBooking *core.Booking
	if row.Buchung.Manuell && len(row.Buchung.Entries) > 0 {
		b := row.Buchung
		manualBooking = &b
	}
```

Mirror Task 3's `recomputeBooking` manual branch, the two buttons (`editBookingBtn`/`autoBookingBtn`), and call `recomputeBooking()` once after construction (so a stored manual booking is shown immediately).

- [ ] **Step 2: Persist on update**

At the update handler, mirror Task 3's `finalBooking`/`learn` branch and pass `finalBooking` into `updateInvoice` (the same place the booking was passed before). Skip template learning when `manualBooking != nil`.

- [ ] **Step 3: Build + vet + test**

Run: `go build ./... && go vet ./internal/ui/ && go test ./...`
Expected: clean.

- [ ] **Step 4: Commit**

```bash
git add internal/ui/tableedit.go
git commit -m "Preserve and edit manual bookings in the edit dialog"
```

---

## Self-Review

- **Spec coverage:** hand-editable Soll/Haben before save (Task 2 editor + Tasks 3/4 buttons), "manuell" flag persisted and preserved (Task 1 + the seed in Task 4), auto-recompute suppressed while manual (Tasks 3/4 `recomputeBooking` branch), template learning skipped for manual (Tasks 3/4 `learn` guard). Covered.
- **Placeholder scan:** the editor is given in full; the modal/edit integrations reference concrete existing anchors (`bookingPrev`, `recomputeBooking`, `computeInvoiceBooking`, `catKeyByLabel`, `companyEntry`, the save handlers) with explicit "adapt to the actual closure" notes, not vague directives.
- **Type consistency:** `Booking.Manuell bool`, `bookingEditRow{Konto,Betrag,Soll}`, `bookingFromRows([]bookingEditRow)Booking`, `showBookingEditor(Booking, func(Booking))`, `bookingKontoLabel(int)string` — consistent across tasks. The `manualBooking *core.Booking` pattern is identical in both dialogs.
- **Data integrity:** manual rows verbatim; zero-account rows dropped; manual booking may be unbalanced (shown), auto stays balanced; manual never overwritten by recompute; learning skipped when manual.
- **Out of scope (later E-phases):** §13b/Reisekosten/Skonto categories (E5), export of manual bookings is automatic (they flow through `CSVRow.Buchung` like any booking).
