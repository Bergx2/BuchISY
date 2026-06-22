# Barkassen-Abgleich / Deckungsprüfung (Phase E11) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend the reconciliation to cash accounts: a cash-paid receipt is "covered" (🟢) when the running cash balance stays ≥ 0 at its entry, and flagged (⚠️) when paying it drives the cash balance negative (a missing Einlage or a wrong date/amount).

**Architecture:** Unlike a bank statement, the cash book is generated FROM the cash invoices (`ComputeCashReport` = opening balance + deposits − cash invoices, chronological running `Saldo`). So there is no external line to match — the meaningful check is coverage: a new pure function `CashCoverage` runs the report and returns which invoices are paid while the balance is negative. The invoice table shows "✓ Bar"/"⚠ Bar" for cash rows (replacing the "—"), and the Belegabgleich dialog gains a Barkasse section summarising each cash account's closing balance + any uncovered points.

**Tech Stack:** Go 1.25, Fyne v2. Reuses `core.ComputeCashReport`/`CashBook`/`CashEntry`, `a.cashAccounts()`, `a.isCashAccount()`, `a.cashInvoicesForMonth()`, `core.LoadCashBooks`, `a.storageManager.GetMonthFolder`, the E10 table status cell + Belegabgleich dialog.

## Global Constraints

- The cash book is generated, not external: never "auto-link" a cash invoice (no BuchungRef). Cash status is purely the coverage check.
- Coverage rule: an invoice is **uncovered** when the running `Saldo` at its cash-report entry is `< -0.005` (negative beyond a rounding tolerance); otherwise covered.
- Reuse `core.ComputeCashReport` verbatim — do NOT reimplement the running-balance logic. `CashEntry.Beleg` is the invoice `Dateiname` (the map key), `CashEntry.Saldo` is the running balance.
- Cash loading mirrors `internal/ui/kassenbuchview.go` (lines ~88-104): `core.LoadCashBooks(filepath.Join(a.storageManager.GetMonthFolder(y, m), "kassenbuch.json"))` + `a.cashInvoicesForMonth(account, y, m)`. Read that file to copy the exact calls/signatures.
- All user-facing strings via `a.bundle.T(...)` in both JSONs (valid JSON). `go build ./... && go test ./... && go vet ./internal/ui/` clean (ignore the pre-existing `clipboard_windows.go:206` warning). Commit per task.

---

### Task 1: Cash coverage function (core)

**Files:**
- Modify: `internal/core/kassenbuch.go` (add `CashCoverage`)
- Test: `internal/core/kassenbuch_test.go`

**Interfaces:**
- Consumes: `ComputeCashReport(book CashBook, invoices []CSVRow) ([]CashEntry, float64)`.
- Produces: `CashCoverage(book CashBook, invoices []CSVRow) (uncovered map[string]bool, closing float64)` — `uncovered[Dateiname]=true` for every invoice entry whose running `Saldo < -0.005`; `closing` is the final balance.

- [ ] **Step 1: Write the failing test**

```go
func TestCashCoverage(t *testing.T) {
	book := CashBook{Konto: "Kasse", Anfangsbestand: 200}
	invoices := []CSVRow{
		{Dateiname: "mueller.pdf", Auftraggeber: "Müller", Bezahldatum: "12.06.2026", Bruttobetrag: 50},
		{Dateiname: "baecker.pdf", Auftraggeber: "Bäcker", Bezahldatum: "14.06.2026", Bruttobetrag: 180},
	}
	uncovered, closing := CashCoverage(book, invoices)
	// 200 - 50 = 150 (covered); 150 - 180 = -30 (Bäcker uncovered).
	if uncovered["mueller.pdf"] {
		t.Errorf("mueller should be covered")
	}
	if !uncovered["baecker.pdf"] {
		t.Errorf("baecker should be uncovered (balance -30)")
	}
	if closing > -29.99 || closing < -30.01 {
		t.Errorf("closing = %v, want -30", closing)
	}
	// A deposit that keeps the balance positive → all covered.
	book2 := CashBook{Konto: "Kasse", Anfangsbestand: 200, Einlagen: []CashDeposit{{Datum: "13.06.2026", Beschreibung: "Einlage", Betrag: 100}}}
	unc2, _ := CashCoverage(book2, invoices)
	if len(unc2) != 0 {
		t.Errorf("with deposit all covered, got %v", unc2)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run TestCashCoverage`
Expected: FAIL (undefined CashCoverage).

- [ ] **Step 3: Implement**

In `internal/core/kassenbuch.go`, after `ComputeCashReport`:

```go
// CashCoverage runs the cash report and reports which cash-paid invoices are
// booked while the running cash balance is negative (i.e. not covered by
// available cash), plus the closing balance. The map key is the invoice
// Dateiname. invoices must already be filtered to this cash account.
func CashCoverage(book CashBook, invoices []CSVRow) (uncovered map[string]bool, closing float64) {
	entries, closing := ComputeCashReport(book, invoices)
	uncovered = map[string]bool{}
	for _, e := range entries {
		if e.Beleg != "" && e.Saldo < -0.005 {
			uncovered[e.Beleg] = true
		}
	}
	return uncovered, closing
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run TestCashCoverage && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/core/kassenbuch.go internal/core/kassenbuch_test.go
git commit -m "Add CashCoverage: flag cash invoices paid while the till is negative"
```

---

### Task 2: Cash coverage status in the invoice table

**Files:**
- Modify: `internal/ui/app.go` (compute `a.cashUncovered` in `loadInvoices`; add the field)
- Modify: `internal/ui/table.go` (cash branch of the BuchungRef status cell)
- Test: none (UI; covered by build). 

**Interfaces:**
- Consumes: `core.CashCoverage`, `a.cashAccounts()`, `a.cashInvoicesForMonth`, `core.LoadCashBooks`, `a.storageManager.GetMonthFolder`.
- Produces: `App.cashUncovered map[string]bool` (Dateiname → not covered), recomputed each `loadInvoices`; the table's cash status cell reads it.

- [ ] **Step 1: Add the field + recompute in loadInvoices**

Add to the `App` struct (where other transient table state lives): `cashUncovered map[string]bool`.

In `loadInvoices` (`internal/ui/app.go:905`), determine the displayed year/month the same way the rest of the function does (it already computes `jahr`/`monat` strings and calls `a.dbRepo.List`). After the rows are loaded, (re)build the coverage map for the current month:

```go
	a.cashUncovered = map[string]bool{}
	for _, acct := range a.cashAccounts() {
		books, _ := core.LoadCashBooks(filepath.Join(a.storageManager.GetMonthFolder(a.currentYear, a.currentMonth), "kassenbuch.json"))
		var book core.CashBook
		for _, b := range books {
			if b.Konto == acct {
				book = b
				break
			}
		}
		unc, _ := core.CashCoverage(book, a.cashInvoicesForMonth(acct, a.currentYear, a.currentMonth))
		for k := range unc {
			a.cashUncovered[k] = true
		}
	}
```

READ `internal/ui/kassenbuchview.go` (lines ~88-118) first to copy the EXACT signatures of `GetMonthFolder`, `cashInvoicesForMonth`, and the `LoadCashBooks` path — `a.currentMonth` may be an `int` or `time.Month`; use whatever those functions actually take (kassenbuchview passes `a.currentMonth` to `GetMonthFolder` directly). If `loadInvoices` runs in a year/all-months scope, loop the months it displays instead of just the current one; if it's month-scoped, the current month is correct. Ensure `filepath` is imported in app.go (it likely is).

- [ ] **Step 2: Show the status in the table**

In `internal/ui/table.go`, the cash branch of the `"BuchungRef"` cell (currently `if it.app != nil && it.app.isCashAccount(row.Bankkonto) { return "—" }` ~line 908):

```go
		if it.app != nil && it.app.isCashAccount(row.Bankkonto) {
			if it.app.cashUncovered[row.Dateiname] {
				return "⚠ " + a.tr("status.cashUncovered") // till went negative
			}
			return "✓ " + a.tr("status.cashCovered") // covered by available cash
		}
```

Use the same translator the surrounding cell code uses (if the cell builds strings via `a.bundle.T(...)` or a helper like `a.tr(...)`, match it; otherwise use literal "Bar gedeckt"/"Bar −" but prefer i18n). Add keys `status.cashCovered` (de "Bar gedeckt"/en "Cash covered") and `status.cashUncovered` (de "Bar nicht gedeckt"/en "Cash uncovered") to both JSONs. `it.app.cashUncovered` may be nil before the first `loadInvoices` — a nil-map read returns false, which is safe (covered), so no guard needed.

- [ ] **Step 3: Build + vet + test**

Run: `go build ./... && go vet ./internal/ui/ && go test ./...`
Expected: clean. Validate both JSONs.

- [ ] **Step 4: Commit**

```bash
git add internal/ui/app.go internal/ui/table.go assets/i18n/de.json assets/i18n/en.json
git commit -m "Show cash coverage status (✓/⚠) for cash invoices in the table"
```

---

### Task 3: Barkasse section in the Belegabgleich dialog

**Files:**
- Modify: `internal/ui/belegabgleichview.go`
- Test: none (UI; covered by build). 

**Interfaces:**
- Consumes: `core.CashCoverage`, `a.cashAccounts()`, `a.cashInvoicesForMonth`, `core.LoadCashBooks`, `a.storageManager.GetMonthFolder`, `core.CashBook`.
- Produces: a Barkasse summary appended to the reconciliation dialog content.

- [ ] **Step 1: Build the Barkasse summary**

In `showBelegabgleich` (`internal/ui/belegabgleichview.go`), after the bank suggestion rows are built and before/after the dialog content is assembled, build a cash summary block and add it to the dialog's VBox:

```go
	cashBox := container.NewVBox()
	for _, acct := range a.cashAccounts() {
		books, _ := core.LoadCashBooks(filepath.Join(a.storageManager.GetMonthFolder(a.currentYear, a.currentMonth), "kassenbuch.json"))
		var book core.CashBook
		for _, b := range books {
			if b.Konto == acct {
				book = b
				break
			}
		}
		unc, closing := core.CashCoverage(book, a.cashInvoicesForMonth(acct, a.currentYear, a.currentMonth))
		line := fmt.Sprintf("%s: %s %s", acct, a.bundle.T("reconcile.cashBalance"), formatEUR(closing))
		if len(unc) > 0 {
			line += "  " + fmt.Sprintf(a.bundle.T("reconcile.cashUncovered"), len(unc))
		} else {
			line += "  " + a.bundle.T("reconcile.cashOk")
		}
		cashBox.Add(widget.NewLabel(line))
	}
	if len(a.cashAccounts()) > 0 {
		// add a heading + cashBox to the dialog content VBox
	}
```

Use the same money-formatting helper the file/app already uses (search for how `InvoiceEURAmount`/amounts are displayed in this file — reuse that `strings.Replace(fmt.Sprintf("%.2f", x), ".", ",", 1)` style; name it `formatEUR` locally or inline it). Place `cashBox` under a heading label `a.bundle.T("reconcile.cashHeading")` in the dialog's scrollable content, after the bank suggestions.

i18n keys (both JSONs, valid): `reconcile.cashHeading` (de "Barkasse"/en "Cash"), `reconcile.cashBalance` (de "Kassenbestand"/en "Cash balance"), `reconcile.cashOk` (de "✓ gedeckt"/en "✓ covered"), `reconcile.cashUncovered` (de "⚠ %d Beleg(e) nicht gedeckt"/en "⚠ %d receipt(s) uncovered").

- [ ] **Step 2: Build + vet + test**

Run: `go build ./... && go vet ./internal/ui/ && go test ./...`
Expected: clean. Validate both JSONs. Smoke: with a cash account whose balance goes negative, the dialog shows the ⚠ count; with a covered one, "✓ gedeckt".

- [ ] **Step 3: Commit**

```bash
git add internal/ui/belegabgleichview.go assets/i18n/de.json assets/i18n/en.json
git commit -m "Add Barkasse coverage summary to the Belegabgleich dialog"
```

---

## Self-Review

- **Spec coverage:** coverage rule (Saldo < 0 → uncovered) in `CashCoverage` (Task 1); per-cash-invoice ✓/⚠ status in the table (Task 2); Barkasse summary with closing balance + uncovered count in the dialog (Task 3). Matches the chosen Deckungsprüfung.
- **Placeholder scan:** Task 1 fully coded + tested; Tasks 2/3 reference the concrete kassenbuchview loading pattern + the E10 table cell / dialog, with full snippets; the implementer copies exact signatures from kassenbuchview.go.
- **Type consistency:** `CashCoverage(book, invoices) (map[string]bool, float64)`; `App.cashUncovered map[string]bool`; reuses `ComputeCashReport`/`CashEntry.Beleg`/`.Saldo`. Consistent.
- **Data integrity:** reuses the proven `ComputeCashReport` running balance (no reimplementation); cash invoices never get a BuchungRef (correct — generated, not matched); nil `cashUncovered` reads as covered (safe default); tolerance −0.005 avoids rounding false-positives.
- **Out of scope:** auto-creating Einlagen to fix a negative till; matching cash invoices to anything external; cross-month cash carry-over beyond what ComputeCashReport already does.
