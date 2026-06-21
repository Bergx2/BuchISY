# Auto-Verbuchung mit Bestätigung (Phase D1) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** In the invoice confirm and edit dialogs, propose a booking (Buchungssatz) live from the receipt's tax lines + chosen category + accounts (Phase C's `BuildBooking`), let the user review it, and persist it on save while learning a per-company template.

**Architecture:** Each Zahlungskonto (BankAccount) gains an SKR04 account number so the booking's Haben side resolves deterministically. The booking rules base and the per-company template store are loaded at profile startup (mirroring the chart). Both dialogs gain a category selector (default from the company's learned template, else "standard") and a read-only live preview that calls `core.BuildBooking` whenever an input changes. On save the resulting `Booking` is stored in `CSVRow.Buchung` (Phase C persistence) and the company→category/account template is updated.

**Tech Stack:** Go 1.25, Fyne v2. Reuses Phase B (`a.chart`, `accountLabel`) and Phase C (`core.Booking`, `core.BuildBooking`, `core.BookingRules`, `core.BookingTemplateStore`).

## Global Constraints

- Reuse Phase C logic verbatim — do NOT re-implement booking math. The dialog computes via `core.BuildBooking(rules, kategorie, taxLines, trinkgeld, expenseAccount, paymentAccount)`.
- NEVER invent account numbers. The payment account comes from the user's per-Zahlungskonto SKR04 mapping; the expense account is the dialog's Gegenkonto (`selectedAccount`); Bewirtung's split accounts come from the rules base.
- Category for a NEW/unknown company defaults to `"standard"`; a known company uses its learned `BookingTemplate.Kategorie`. The user can always change the category in the dialog. No Claude call in D1.
- "Always confirm": the booking is shown as a preview in the dialog; saving is the confirmation. Never auto-post without the dialog.
- Backward compat: a receipt whose payment account has no SKR04 mapping, or with no tax lines, must not crash the dialog — the preview shows a clear "nicht buchbar" hint and save still works (stores an empty `Booking`).
- All user-facing strings via `a.bundle.T(...)` with keys in both `de.json` and `en.json` (valid JSON).
- `go build ./... && go test ./... && go vet ./internal/ui/` clean (ignore the pre-existing `clipboard_windows.go:206` warning). Commit per task.

---

### Task 1: Per-Zahlungskonto SKR04 account

**Files:**
- Modify: `internal/core/types.go` (BankAccount + a lookup helper)
- Test: `internal/core/settings_test.go`

**Interfaces:**
- Produces: `BankAccount.SKR04Konto int` (json `skr04_konto,omitempty`);
  `(s Settings) PaymentAccountSKR04(bankAccountName string) (int, bool)` — returns the mapped SKR04 account for a Zahlungskonto by name; if the named account has `SKR04Konto == 0`, falls back by `AccountType` (bank→1800, cash→1600) and returns `(0,false)` for creditcard/unknown with no explicit mapping.

- [ ] **Step 1: Write the failing test**

```go
func TestPaymentAccountSKR04(t *testing.T) {
	s := Settings{BankAccounts: []BankAccount{
		{Name: "Sparkasse", AccountType: AccountTypeBank, SKR04Konto: 1800},
		{Name: "Barkasse", AccountType: AccountTypeCash},        // no explicit → fallback 1600
		{Name: "Visa", AccountType: AccountTypeCreditCard},      // no mapping → (0,false)
	}}
	if k, ok := s.PaymentAccountSKR04("Sparkasse"); !ok || k != 1800 {
		t.Errorf("Sparkasse = %d,%v", k, ok)
	}
	if k, ok := s.PaymentAccountSKR04("Barkasse"); !ok || k != 1600 {
		t.Errorf("Barkasse (cash fallback) = %d,%v", k, ok)
	}
	if _, ok := s.PaymentAccountSKR04("Visa"); ok {
		t.Error("Visa without mapping should be (0,false)")
	}
	if _, ok := s.PaymentAccountSKR04("Unbekannt"); ok {
		t.Error("unknown account name should be (0,false)")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run TestPaymentAccountSKR04`
Expected: FAIL (unknown field SKR04Konto / undefined method).

- [ ] **Step 3: Implement**

In `internal/core/types.go`, add to `BankAccount` (after `SettlementAccount`):

```go
	SKR04Konto int `json:"skr04_konto,omitempty"` // SKR04 account for the Haben side when booking
```

Add the method (near the other `Settings` methods):

```go
// PaymentAccountSKR04 returns the SKR04 account that the Haben (credit) side of
// a booking should post to for a given Zahlungskonto, looked up by name. An
// explicit BankAccount.SKR04Konto wins; otherwise it falls back by account type
// (bank→1800, cash→1600). Returns (0,false) when nothing maps.
func (s Settings) PaymentAccountSKR04(bankAccountName string) (int, bool) {
	for _, ba := range s.BankAccounts {
		if ba.Name != bankAccountName {
			continue
		}
		if ba.SKR04Konto != 0 {
			return ba.SKR04Konto, true
		}
		switch ba.AccountType {
		case AccountTypeBank:
			return 1800, true
		case AccountTypeCash:
			return 1600, true
		}
		return 0, false
	}
	return 0, false
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run TestPaymentAccountSKR04 && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/core/types.go internal/core/settings_test.go
git commit -m "Add per-Zahlungskonto SKR04 account mapping"
```

---

### Task 2: Load booking rules + template store at startup

**Files:**
- Modify: `internal/ui/app.go` (App struct + startProfile)
- Test: none (wiring; covered by build). 

**Interfaces:**
- Consumes: `assets.BuchungsregelnJSON`, `core.ParseBookingRules`, `core.NewBookingTemplateStore`.
- Produces: `a.bookingRules *core.BookingRules`, `a.bookingTemplates *core.BookingTemplateStore` populated in `startProfile`.

- [ ] **Step 1: Add the fields**

In the `App` struct (after `chart *core.ChartOfAccounts` at line 49), add:

```go
	bookingRules     *core.BookingRules
	bookingTemplates *core.BookingTemplateStore
```

- [ ] **Step 2: Load them in startProfile**

In `startProfile`, immediately AFTER the chart-loading block (the lines that set `a.chartStore`/`a.chart`, ~line 230-236), add:

```go
	if rules, err := core.ParseBookingRules(assets.BuchungsregelnJSON); err != nil {
		logger.Warn("Failed to parse booking rules: %v", err)
		a.bookingRules = &core.BookingRules{}
	} else {
		a.bookingRules = rules
	}
	a.bookingTemplates = core.NewBookingTemplateStore(configDir)
	if err := a.bookingTemplates.Load(); err != nil {
		logger.Warn("Failed to load booking templates: %v", err)
	}
```

- [ ] **Step 3: Build**

Run: `go build ./...`
Expected: clean.

- [ ] **Step 4: Commit**

```bash
git add internal/ui/app.go
git commit -m "Load booking rules and templates at profile startup"
```

---

### Task 3: Booking preview component

**Files:**
- Create: `internal/ui/bookingpreview.go`
- Test: `internal/ui/bookingpreview_test.go`

**Interfaces:**
- Consumes: `core.Booking`, `core.ChartOfAccounts`, `accountLabel`.
- Produces: `formatBookingLines(b core.Booking, chart *core.ChartOfAccounts) []string` (one display string per entry, e.g. `"Soll  6640  Bewirtungskosten (abziehbar)   12,71"`, Haben for credit; amounts with German decimal comma);
  `type bookingPreview struct { container *fyne.Container }`;
  `newBookingPreview(a *App) *bookingPreview`;
  `(p *bookingPreview) set(b core.Booking, bookable bool, reason string)` — renders the lines + a balance line `"Σ Soll = Σ Haben ✓"` when bookable, or the `reason` hint when not.

- [ ] **Step 1: Write the failing test**

```go
package ui

import (
	"strings"
	"testing"

	"github.com/bergx2/buchisy/internal/core"
)

func TestFormatBookingLines(t *testing.T) {
	chart := core.NewChartOfAccounts([]core.SKRAccount{
		{Number: 6640, Name: "Bewirtungskosten (abziehbar)"},
		{Number: 1800, Name: "Bank"},
	})
	b := core.Booking{Entries: []core.BookingEntry{
		{Konto: 6640, Betrag: 12.71, Soll: true},
		{Konto: 1800, Betrag: 12.71, Soll: false},
	}}
	lines := formatBookingLines(b, chart)
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d", len(lines))
	}
	if !strings.Contains(lines[0], "Soll") || !strings.Contains(lines[0], "6640") ||
		!strings.Contains(lines[0], "Bewirtungskosten") || !strings.Contains(lines[0], "12,71") {
		t.Errorf("soll line wrong: %q", lines[0])
	}
	if !strings.Contains(lines[1], "Haben") || !strings.Contains(lines[1], "1800") {
		t.Errorf("haben line wrong: %q", lines[1])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ui/ -run TestFormatBookingLines`
Expected: FAIL (undefined formatBookingLines).

- [ ] **Step 3: Implement**

Create `internal/ui/bookingpreview.go`. The package has no shared German-amount helper, so format inline with `strings.Replace(fmt.Sprintf("%.2f", e.Betrag), ".", ",", 1)` (as the code below does). The taxLinesEditor exposes `Lines() []core.TaxLine` and `Trinkgeld() float64` (confirmed) — later tasks use those.

```go
package ui

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/bergx2/buchisy/internal/core"
)

// formatBookingLines renders each booking entry as one human-readable line.
func formatBookingLines(b core.Booking, chart *core.ChartOfAccounts) []string {
	lines := make([]string, 0, len(b.Entries))
	for _, e := range b.Entries {
		side := "Soll "
		if !e.Soll {
			side = "Haben"
		}
		name := ""
		if chart != nil {
			if acc, ok := chart.Find(e.Konto); ok {
				name = acc.Name
			}
		}
		amount := strings.Replace(fmt.Sprintf("%.2f", e.Betrag), ".", ",", 1)
		lines = append(lines, strings.TrimSpace(fmt.Sprintf("%s  %d  %s  %s", side, e.Konto, name, amount)))
	}
	return lines
}

// bookingPreview is a read-only widget that shows the proposed Buchungssatz.
type bookingPreview struct {
	app       *App
	container *fyne.Container
}

func newBookingPreview(a *App) *bookingPreview {
	return &bookingPreview{app: a, container: container.NewVBox()}
}

// set replaces the preview content. When bookable is false, reason is shown.
func (p *bookingPreview) set(b core.Booking, bookable bool, reason string) {
	p.container.RemoveAll()
	if !bookable {
		hint := widget.NewLabel(reason)
		hint.Wrapping = fyne.TextWrapWord
		p.container.Add(hint)
		p.container.Refresh()
		return
	}
	for _, line := range formatBookingLines(b, p.app.chart) {
		p.container.Add(widget.NewLabel(line))
	}
	status := p.app.bundle.T("booking.balanced")
	if !b.Balanced() {
		status = p.app.bundle.T("booking.unbalanced")
	}
	p.container.Add(widget.NewLabel(status))
	p.container.Refresh()
}
```

Add i18n keys to both JSONs: `booking.balanced` (de "Σ Soll = Σ Haben ✓" / en "Σ debit = Σ credit ✓"), `booking.unbalanced` (de "Nicht ausgeglichen!" / en "Not balanced!").

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/ui/ -run TestFormatBookingLines && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/bookingpreview.go internal/ui/bookingpreview_test.go assets/i18n/de.json assets/i18n/en.json
git commit -m "Add booking preview component"
```

---

### Task 4: Settings UI — SKR04 account per Zahlungskonto

**Files:**
- Modify: `internal/ui/settings.go` (the Zahlungskonten/bank-accounts editor rows)
- Test: none (UI; covered by build + manual). 

**Interfaces:**
- Consumes: `BankAccount.SKR04Konto`, `a.showAccountSearch`, `accountLabel`, `a.chart`.
- Produces: each bank-account row in settings gets an SKR04 control bound to `BankAccount.SKR04Konto`.

- [ ] **Step 1: Locate the bank-account row builder**

In `internal/ui/settings.go`, find where each `BankAccount` is rendered as an editable row (search for `AccountType` select / `IBAN` entry within the bank-accounts section). Each row already edits Name/IBAN/Type into a `BankAccount` value held in a slice the settings dialog saves.

- [ ] **Step 2: Add an SKR04 control to the row**

For each row, add a button labeled via i18n key `settings.payment.skr04` ("SKR04-Konto") that shows the current mapping and opens the picker:

```go
	skr04Display := widget.NewLabel(paymentSKR04Label(a, ba.SKR04Konto))
	skr04Btn := widget.NewButton(a.bundle.T("settings.payment.skr04"), func() {
		a.showAccountSearch(ba.SKR04Konto, func(n int) {
			ba.SKR04Konto = n            // update the row's BankAccount (capture by pointer/index as the other fields do)
			accounts[i] = ba             // mirror how Name/IBAN writes back into the slice
			skr04Display.SetText(paymentSKR04Label(a, n))
		})
	})
```

Place `skr04Display` + `skr04Btn` in the row layout next to the existing fields. Match exactly how the row already writes Name/IBAN/AccountType back into the accounts slice (same capture pattern — pointer or `accounts[i] = ba`); if the row uses an index closure, reuse it.

Add the helper at the bottom of settings.go:

```go
// paymentSKR04Label renders the SKR04 mapping for a payment account row.
func paymentSKR04Label(a *App, konto int) string {
	if konto == 0 {
		return a.bundle.T("settings.payment.skr04.none")
	}
	if acc, ok := a.chart.Find(konto); ok {
		return accountLabel(acc)
	}
	return fmt.Sprintf("%d", konto)
}
```

Add i18n keys: `settings.payment.skr04` (de "SKR04-Konto…" / en "SKR04 account…"), `settings.payment.skr04.none` (de "— (Standard nach Typ)" / en "— (default by type)").

- [ ] **Step 3: Build + vet**

Run: `go build ./... && go vet ./internal/ui/`
Expected: clean (only the pre-existing clipboard warning). Validate both JSONs parse.

- [ ] **Step 4: Commit**

```bash
git add internal/ui/settings.go assets/i18n/de.json assets/i18n/en.json
git commit -m "Settings: map an SKR04 account to each Zahlungskonto"
```

---

### Task 5: Booking in the confirmation modal

**Files:**
- Modify: `internal/ui/invoicemodal.go` (`showConfirmationModal`, `saveInvoice`)
- Test: none (UI integration; covered by build + manual). 

**Interfaces:**
- Consumes: `a.bookingRules`, `a.bookingTemplates`, `a.settings.PaymentAccountSKR04`, `core.BuildBooking`, `newBookingPreview`, the dialog's `selectedAccount`, `bankAccountSelect`, `ed` (taxLinesEditor).
- Produces: the modal shows a category selector + live booking preview; `saveInvoice` persists the computed `Booking` and learns the template.

- [ ] **Step 1: Build the category selector + preview, wire live updates**

After `ed` is created (~line 361) and `selectedAccount`/`bankAccountSelect` exist, add:

```go
	// Booking category: learned template for this company, else "standard".
	category := "standard"
	if tmpl, ok := a.bookingTemplates.Get(meta.Auftraggeber); ok {
		category = tmpl.Kategorie
	}
	catOptions, catKeyByLabel := a.bookingCategoryOptions() // helper below
	categorySelect := widget.NewSelect(catOptions, nil)
	categorySelect.SetSelected(a.bookingCategoryLabel(category))

	preview := newBookingPreview(a)
	recomputeBooking := func() {
		b, bookable, reason := a.computeInvoiceBooking(
			catKeyByLabel[categorySelect.Selected],
			ed.Lines(), ed.Trinkgeld(), selectedAccount, bankAccountSelect.Selected)
		preview.set(b, bookable, reason)
	}
	categorySelect.OnChanged = func(string) { recomputeBooking() }
```

Call `recomputeBooking()` once after construction, and ALSO from the existing `OnChanged`/onPick handlers that already fire when the tax lines, Gegenkonto (`selectedAccount` pick at ~line 206), or `bankAccountSelect` (~line 217) change — append `recomputeBooking()` to each. The taxLinesEditor already takes an `onChange` callback (`updateFilenamePreview`); wrap it so it also calls `recomputeBooking()`.

Place `categorySelect` and `preview.container` in the modal layout near the Gegenkonto/tax area (a labeled section "Buchungsvorschlag").

- [ ] **Step 2: Add the helpers**

In `invoicemodal.go` (or a small `internal/ui/bookinghelpers.go`):

```go
// bookingCategoryOptions returns display labels for the rule categories plus a
// map from label back to the category key.
func (a *App) bookingCategoryOptions() ([]string, map[string]string) {
	labels := make([]string, 0, len(a.bookingRules.Regeln))
	byLabel := map[string]string{}
	for _, r := range a.bookingRules.Regeln {
		label := r.Name
		if label == "" {
			label = r.Kategorie
		}
		labels = append(labels, label)
		byLabel[label] = r.Kategorie
	}
	return labels, byLabel
}

// bookingCategoryLabel maps a category key to its display label.
func (a *App) bookingCategoryLabel(kategorie string) string {
	for _, r := range a.bookingRules.Regeln {
		if r.Kategorie == kategorie {
			if r.Name != "" {
				return r.Name
			}
			return r.Kategorie
		}
	}
	return kategorie
}

// computeInvoiceBooking resolves the payment account and builds the booking.
// Returns (booking, bookable, reasonIfNotBookable).
func (a *App) computeInvoiceBooking(kategorie string, lines []core.TaxLine, trinkgeld float64, expenseAccount int, bankAccountName string) (core.Booking, bool, string) {
	if len(lines) == 0 {
		return core.Booking{}, false, a.bundle.T("booking.no.lines")
	}
	payment, ok := a.settings.PaymentAccountSKR04(bankAccountName)
	if !ok {
		return core.Booking{}, false, a.bundle.T("booking.no.payment.account")
	}
	b, err := core.BuildBooking(a.bookingRules, kategorie, lines, trinkgeld, expenseAccount, payment)
	if err != nil {
		return core.Booking{}, false, err.Error()
	}
	return b, true, ""
}
```

`ed.Lines()` / `ed.Trinkgeld()` — confirm the taxLinesEditor exposes the current lines and trinkgeld; if the accessors have different names, use those. Add i18n keys `booking.no.lines` (de "Keine MwSt-Zeilen — nicht buchbar" / en "No tax lines — not bookable") and `booking.no.payment.account` (de "Zahlungskonto ohne SKR04-Konto (in Einstellungen zuordnen)" / en "Payment account has no SKR04 account (map it in settings)").

- [ ] **Step 3: Persist the booking + learn the template on save**

At the save handler (~line 538) compute the final booking and pass it through. Add a `Buchung core.Booking` parameter to `saveInvoice` (set `row.Buchung = booking` before insert). After a successful save, learn the template:

```go
	booking, bookable, _ := a.computeInvoiceBooking(catKeyByLabel[categorySelect.Selected], ed.Lines(), ed.Trinkgeld(), selectedAccount, bankAccountSelect.Selected)
	// ... pass `booking` into saveInvoice (stored only when bookable; empty Booking otherwise) ...
	if bookable {
		_ = a.bookingTemplates.Set(meta.Auftraggeber, core.BookingTemplate{Kategorie: catKeyByLabel[categorySelect.Selected], ExpenseKonto: selectedAccount})
	}
```

In `saveInvoice`, add the parameter and set `Buchung: booking` in the `core.CSVRow{...}` literal it builds (around line 662 where `Bankkonto` is set).

- [ ] **Step 4: Build + vet + manual smoke**

Run: `go build ./... && go vet ./internal/ui/ && go test ./...`
Expected: clean. Validate both JSONs.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/invoicemodal.go internal/ui/bookinghelpers.go assets/i18n/de.json assets/i18n/en.json
git commit -m "Propose, preview and persist the booking in the confirmation modal"
```

---

### Task 6: Booking in the edit dialog

**Files:**
- Modify: `internal/ui/tableedit.go` (`showEditDialog`, `updateInvoice`)
- Test: none (UI integration; covered by build + manual). 

**Interfaces:**
- Consumes: the same helpers from Task 5 (`bookingCategoryOptions`, `bookingCategoryLabel`, `computeInvoiceBooking`, `newBookingPreview`).
- Produces: the edit dialog shows the category selector + preview and persists the booking + learns the template on update; an existing `row.Buchung` pre-selects the category.

- [ ] **Step 1: Pre-select category from the existing booking, else template, else standard**

In `showEditDialog`, after `ed`/`selectedAccount`/`bankAccountSelect` exist, mirror Task 5's construction. For the initial category use, in order: if `row.Buchung` has an `Info`-encoded category or a recognizable structure use it; simplest deterministic rule — if a template exists for `meta.Auftraggeber`, use it; else `"standard"`. (Do NOT try to reverse-engineer the category from stored entries — use the template/standard default.)

```go
	category := "standard"
	if tmpl, ok := a.bookingTemplates.Get(meta.Auftraggeber); ok {
		category = tmpl.Kategorie
	}
	catOptions, catKeyByLabel := a.bookingCategoryOptions()
	categorySelect := widget.NewSelect(catOptions, nil)
	categorySelect.SetSelected(a.bookingCategoryLabel(category))
	preview := newBookingPreview(a)
	recomputeBooking := func() {
		b, bookable, reason := a.computeInvoiceBooking(
			catKeyByLabel[categorySelect.Selected],
			ed.Lines(), ed.Trinkgeld(), selectedAccount, bankAccountSelect.Selected)
		preview.set(b, bookable, reason)
	}
	categorySelect.OnChanged = func(string) { recomputeBooking() }
```

Wire `recomputeBooking()` into the same change points (tax editor onChange, account pick at ~line 139, bank select) and call once after construction. Place `categorySelect` + `preview.container` in the layout.

- [ ] **Step 2: Persist on update + learn template**

At the update handler (~line 455) compute the booking and pass it to `updateInvoice` (add a `Buchung core.Booking` parameter; set `Buchung: booking` in the `core.CSVRow{...}` it builds near line 563). After a successful update, `a.bookingTemplates.Set(meta.Auftraggeber, core.BookingTemplate{Kategorie: catKeyByLabel[categorySelect.Selected], ExpenseKonto: selectedAccount})` when bookable.

- [ ] **Step 3: Build + vet + test**

Run: `go build ./... && go vet ./internal/ui/ && go test ./...`
Expected: clean.

- [ ] **Step 4: Commit**

```bash
git add internal/ui/tableedit.go assets/i18n/de.json assets/i18n/en.json
git commit -m "Propose, preview and persist the booking in the edit dialog"
```

---

## Self-Review

- **Spec coverage (Phase D / Baustein D — interactive booking part):** booking proposal generated per receipt from tax lines + category + chart + rules (Tasks 3/5/6 via `computeInvoiceBooking`→`BuildBooking`); hybrid memory arm = company template default + learning (Tasks 2/5/6); always-confirm via the preview + save (Tasks 5/6); deterministic payment account (Tasks 1/4). DEFERRED to later D phases: Claude category suggestion (D1 uses standard default per the user's choice), DATEV/Lexware export (D2), controlling (D3).
- **Placeholder scan:** UI tasks reference concrete existing anchors (`selectedAccount`, `bankAccountSelect`, `ed`, save handlers at named lines) and concrete helper code; the only "confirm the accessor name" notes (`ed.Lines()`/`ed.Trinkgeld()`, `formatGermanAmount`) are explicit verification steps, not vague directives.
- **Type consistency:** `BankAccount.SKR04Konto int`, `Settings.PaymentAccountSKR04(string)(int,bool)`, `formatBookingLines(Booking,*ChartOfAccounts)[]string`, `bookingPreview.set(Booking,bool,string)`, `bookingCategoryOptions()([]string,map[string]string)`, `computeInvoiceBooking(string,[]TaxLine,float64,int,string)(Booking,bool,string)` — consistent across tasks 1/3/5/6. `saveInvoice`/`updateInvoice` both gain a trailing `Buchung core.Booking` parameter.
- **Account/data integrity:** payment account from user mapping or type fallback (bank→1800/cash→1600), never invented; expense = dialog Gegenkonto; Bewirtung split from rules; empty/unbookable → stored empty `Booking`, dialog still saves.
- **Open dependency for D2 (not this plan):** DATEV Berater-/Mandantennummer + Wirtschaftsjahr and the Lexware import format — collect from the user before planning D2.
