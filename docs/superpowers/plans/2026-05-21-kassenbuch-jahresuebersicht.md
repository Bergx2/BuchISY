# Kassenbuch-Jahresübersicht Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Eine rein lesende Jahresübersicht, die je Barkassen-Konto alle zwölf Monate eines Jahres mit Anfangsbestand, Einnahmen, Ausgaben und Endbestand zeigt.

**Architecture:** Eine reine Kern-Funktion `ComputeYearOverview` rollt den Saldo Januar→Dezember (TDD-getestet). Die bestehende Übertrag-Rückwärtslogik wird aus einer Closure in die wiederverwendbare Methode `cashCarryIn` extrahiert. Eine neue Vollseiten-Ansicht `showCashYearView` lädt die zwölf Monate und zeigt die Tabelle; ein Button in der Kassenbuch-Ansicht öffnet sie.

**Tech Stack:** Go 1.25, Fyne v2.6.3, Standard-`testing`.

**Hinweis zu Commits:** Git-Commits sind in diesem Plan bewusst ausgelassen — Auslieferung per Build + Kopie der `.exe`. Jede Aufgabe endet mit `go build`/`go vet`/`go test` als Verifikation.

---

### Task 1: Jahres-Berechnung `ComputeYearOverview` (TDD)

**Files:**
- Create: `internal/core/jahresuebersicht.go`
- Test: `internal/core/jahresuebersicht_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/core/jahresuebersicht_test.go`:

```go
package core

import (
	"testing"
	"time"
)

func TestComputeYearOverview(t *testing.T) {
	months := make([]MonthInput, 12)
	// January: stored book, opening 100, one deposit of 50.
	months[0] = MonthInput{
		HasStoredBook: true,
		Book: CashBook{Konto: "Barkasse", Anfangsbestand: 100, Einlagen: []CashDeposit{
			{Datum: "10.01.2026", Beschreibung: "Einlage", Betrag: 50},
		}},
	}
	// February: no stored book, one cash invoice of 30.
	months[1] = MonthInput{
		Invoices: []CSVRow{{Firmenname: "X", Bruttobetrag: 30, Bezahldatum: "05.02.2026"}},
	}
	// March..December: empty.

	got := ComputeYearOverview(999, months) // carriedIn ignored: Jan has a stored book.

	if len(got) != 12 {
		t.Fatalf("got %d summaries, want 12", len(got))
	}
	if got[0].Month != time.January {
		t.Errorf("month[0] = %v, want January", got[0].Month)
	}
	if got[0].Anfangsbestand != 100 {
		t.Errorf("Jan opening = %v, want 100 (stored book overrides carriedIn)", got[0].Anfangsbestand)
	}
	if got[0].Einnahmen != 50 || got[0].Ausgaben != 0 || got[0].Endbestand != 150 {
		t.Errorf("Jan = %+v, want Einnahmen 50 / Ausgaben 0 / Endbestand 150", got[0])
	}
	if got[1].Anfangsbestand != 150 {
		t.Errorf("Feb opening = %v, want 150 (carried from Jan)", got[1].Anfangsbestand)
	}
	if got[1].Ausgaben != 30 || got[1].Endbestand != 120 {
		t.Errorf("Feb = %+v, want Ausgaben 30 / Endbestand 120", got[1])
	}
	if got[2].Anfangsbestand != 120 || got[2].Endbestand != 120 {
		t.Errorf("Mar = %+v, want 120/120 (empty month carries forward)", got[2])
	}
}

func TestComputeYearOverviewCarriedIn(t *testing.T) {
	months := make([]MonthInput, 12) // all empty, no stored books
	got := ComputeYearOverview(3497.35, months)
	if got[0].Anfangsbestand != 3497.35 || got[0].Endbestand != 3497.35 {
		t.Errorf("Jan = %+v, want carriedIn 3497.35 unchanged", got[0])
	}
	if got[11].Endbestand != 3497.35 {
		t.Errorf("Dec Endbestand = %v, want 3497.35", got[11].Endbestand)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run TestComputeYearOverview -v`
Expected: FAIL — `undefined: MonthInput`, `MonthSummary`, `ComputeYearOverview`.

- [ ] **Step 3: Write the implementation**

Create `internal/core/jahresuebersicht.go`:

```go
package core

import "time"

// MonthInput is the per-month data fed to ComputeYearOverview.
type MonthInput struct {
	HasStoredBook bool     // whether a cash book is stored for this month
	Book          CashBook // valid only when HasStoredBook
	Invoices      []CSVRow // cash invoices booked to the account this month
}

// MonthSummary is one month's row of a year overview.
type MonthSummary struct {
	Month          time.Month
	Anfangsbestand float64
	Einnahmen      float64
	Ausgaben       float64
	Endbestand     float64
}

// ComputeYearOverview rolls the cash balance through a year's months.
// carriedIn is the opening balance entering the first month, used when that
// month has no stored cash book. months are in calendar order, January
// first; each summary carries the matching time.Month (index 0 -> January).
// A month with a stored book uses that book's own opening balance; a month
// without one opens with the previous month's closing balance.
func ComputeYearOverview(carriedIn float64, months []MonthInput) []MonthSummary {
	summaries := make([]MonthSummary, len(months))
	running := carriedIn
	for i, mi := range months {
		anfang := running
		var book CashBook
		if mi.HasStoredBook {
			book = mi.Book
			anfang = book.Anfangsbestand
		} else {
			book = CashBook{Anfangsbestand: anfang}
		}
		entries, end := ComputeCashReport(book, mi.Invoices)
		var einnahmen, ausgaben float64
		for _, e := range entries {
			einnahmen += e.Einnahme
			ausgaben += e.Ausgabe
		}
		summaries[i] = MonthSummary{
			Month:          time.Month(i + 1),
			Anfangsbestand: anfang,
			Einnahmen:      einnahmen,
			Ausgaben:       ausgaben,
			Endbestand:     end,
		}
		running = end
	}
	return summaries
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run TestComputeYearOverview -v`
Expected: PASS — both `TestComputeYearOverview` and `TestComputeYearOverviewCarriedIn`.

- [ ] **Step 5: Build and vet**

Run: `go build ./internal/core/ && go vet ./internal/core/... && go test ./internal/core/...`
Expected: PASS.

---

### Task 2: Übertrag-Logik in die Methode `cashCarryIn` extrahieren

**Files:**
- Modify: `internal/ui/kassenbuchview.go`

- [ ] **Step 1: Add the `cashCarryIn` method**

In `internal/ui/kassenbuchview.go`, the function `cashInvoicesFor` currently ends like this:

```go
// cashInvoicesFor returns the current month's invoices booked to the named
// cash account.
func (a *App) cashInvoicesFor(account string) []core.CSVRow {
	return a.cashInvoicesForMonth(account, a.currentYear, a.currentMonth)
}
```

Immediately after that function, add this new method:

```go
// cashCarryIn returns the opening balance carried into (year, month) for a
// cash account: it walks backwards to the most recent month that has a
// stored cash book (the anchor), then rolls the balance forward — counting
// each month's cash invoices — up to the month before (year, month). ok is
// false when no stored cash book exists in the lookback window.
func (a *App) cashCarryIn(account string, year int, month time.Month) (float64, bool) {
	const maxLookback = 60 // months
	type ym struct {
		y int
		m time.Month
	}
	// chain: the previous month first, walking back to the anchor (inclusive).
	var chain []ym
	var anchorBook core.CashBook
	found := false
	y, m := year, month
	for i := 0; i < maxLookback && !found; i++ {
		m--
		if m < time.January {
			m, y = time.December, y-1
		}
		chain = append(chain, ym{y, m})
		mb, _ := core.LoadCashBooks(
			filepath.Join(a.storageManager.GetMonthFolder(y, m), "kassenbuch.json"))
		for _, b := range mb {
			if b.Konto == account {
				anchorBook = b
				found = true
				break
			}
		}
	}
	if !found {
		return 0, false
	}
	// Roll forward: the anchor's closing balance, then each later month
	// (no stored book → empty book seeded with the running balance).
	anchor := chain[len(chain)-1]
	_, balance := core.ComputeCashReport(anchorBook, a.cashInvoicesForMonth(account, anchor.y, anchor.m))
	for i := len(chain) - 2; i >= 0; i-- {
		mo := chain[i]
		_, balance = core.ComputeCashReport(
			core.CashBook{Konto: account, Anfangsbestand: balance},
			a.cashInvoicesForMonth(account, mo.y, mo.m))
	}
	return balance, true
}
```

- [ ] **Step 2: Replace the `carryOver` closure with a call to the method**

In `internal/ui/kassenbuchview.go`, inside `showCashBookView`, the `carryOver` closure and the `bookFor` closure currently read exactly:

```go
	// carryOver pre-fills a new month's opening balance. It walks backwards
	// from the previous month to the most recent month that has a stored
	// cash book for the account (the anchor), then rolls the balance forward
	// through every month in between — counting each month's cash invoices —
	// up to the previous month. This keeps the opening balance correct even
	// when the months between the anchor and now were never opened.
	carryOver := func(account string) (float64, bool) {
		const maxLookback = 60 // months
		type ym struct {
			y int
			m time.Month
		}
		// chain: previous month first, walking back to the anchor (inclusive).
		var chain []ym
		var anchorBook core.CashBook
		found := false
		y, m := a.currentYear, a.currentMonth
		for i := 0; i < maxLookback && !found; i++ {
			m--
			if m < time.January {
				m, y = time.December, y-1
			}
			chain = append(chain, ym{y, m})
			mb, _ := core.LoadCashBooks(
				filepath.Join(a.storageManager.GetMonthFolder(y, m), "kassenbuch.json"))
			for _, b := range mb {
				if b.Konto == account {
					anchorBook = b
					found = true
					break
				}
			}
		}
		if !found {
			return 0, false
		}
		// Roll forward: the anchor's closing balance, then each later month
		// (no stored book → empty book seeded with the running balance).
		anchor := chain[len(chain)-1]
		_, balance := core.ComputeCashReport(anchorBook, a.cashInvoicesForMonth(account, anchor.y, anchor.m))
		for i := len(chain) - 2; i >= 0; i-- {
			mo := chain[i]
			_, balance = core.ComputeCashReport(
				core.CashBook{Konto: account, Anfangsbestand: balance},
				a.cashInvoicesForMonth(account, mo.y, mo.m))
		}
		return balance, true
	}

	// bookFor returns a pointer to the working CashBook for an account,
	// creating it in books if absent. A freshly created book pre-fills its
	// opening balance from the previous month's closing balance.
	bookFor := func(account string) *core.CashBook {
		for i := range books {
			if books[i].Konto == account {
				return &books[i]
			}
		}
		nb := core.CashBook{Konto: account}
		if end, ok := carryOver(account); ok {
			nb.Anfangsbestand = end
		}
		books = append(books, nb)
		return &books[len(books)-1]
	}
```

Replace that entire block (the `carryOver` closure plus the `bookFor` closure) with just:

```go
	// bookFor returns a pointer to the working CashBook for an account,
	// creating it in books if absent. A freshly created book pre-fills its
	// opening balance from the previous month's closing balance.
	bookFor := func(account string) *core.CashBook {
		for i := range books {
			if books[i].Konto == account {
				return &books[i]
			}
		}
		nb := core.CashBook{Konto: account}
		if end, ok := a.cashCarryIn(account, a.currentYear, a.currentMonth); ok {
			nb.Anfangsbestand = end
		}
		books = append(books, nb)
		return &books[len(books)-1]
	}
```

- [ ] **Step 3: Build and vet**

Run: `go build ./... && go vet ./...`
Expected: PASS — the carry-over behaviour of `showCashBookView` is unchanged; the logic now lives in the reusable `cashCarryIn` method.

---

### Task 3: Jahresübersicht-Ansicht + Button

**Files:**
- Modify: `internal/ui/kassenbuchview.go`

- [ ] **Step 1: Add the "Jahresübersicht" button to the cash-book view header**

In `internal/ui/kassenbuchview.go`, inside `showCashBookView`, the end of the `pdfBtn` definition is immediately followed by the `header` construction:

```go
	})

	header := container.NewBorder(nil, nil,
		container.NewPadded(titleLabel),
		container.NewPadded(container.NewHBox(backBtn, saveBtn, pdfBtn)),
		container.NewPadded(accountSelect),
	)
```

Replace that with (adds `yearViewBtn` and puts it in the header's button row):

```go
	})

	yearViewBtn := widget.NewButton("Jahresübersicht", func() {
		a.showCashYearView(accountSelect.Selected, a.currentYear)
	})

	header := container.NewBorder(nil, nil,
		container.NewPadded(titleLabel),
		container.NewPadded(container.NewHBox(backBtn, saveBtn, pdfBtn, yearViewBtn)),
		container.NewPadded(accountSelect),
	)
```

- [ ] **Step 2: Add the `showCashYearView` function**

In `internal/ui/kassenbuchview.go`, append this function at the end of the file:

```go
// showCashYearView shows a read-only twelve-month overview for one cash
// account: per month the opening balance, deposits, cash expenses and
// closing balance. Clicking a month opens that month's cash book.
func (a *App) showCashYearView(account string, year int) {
	accounts := a.cashAccounts()
	if len(accounts) == 0 {
		a.showMainView()
		return
	}
	// Fall back to the first account if the requested one no longer exists.
	sel := accounts[0]
	for _, n := range accounts {
		if n == account {
			sel = account
			break
		}
	}

	curYear := year

	titleLabel := widget.NewLabelWithStyle(
		fmt.Sprintf("Jahresübersicht — %d", curYear),
		fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	backBtn := widget.NewButton(a.bundle.T("btn.cancel"), func() { a.showMainView() })

	accountSelect := widget.NewSelect(accounts, nil)
	accountSelect.SetSelected(sel)
	yearSelect := widget.NewSelect(generateYearOptions(), nil)
	yearSelect.SetSelected(fmt.Sprintf("%d", curYear))

	tableArea := container.NewVBox()

	rebuild := func() {
		curAccount := accountSelect.Selected
		titleLabel.SetText(fmt.Sprintf("Jahresübersicht — %d", curYear))

		// Collect the twelve months' input data.
		months := make([]core.MonthInput, 12)
		for i := 0; i < 12; i++ {
			m := time.Month(i + 1)
			folder := a.storageManager.GetMonthFolder(curYear, m)
			storedBooks, _ := core.LoadCashBooks(filepath.Join(folder, "kassenbuch.json"))
			mi := core.MonthInput{Invoices: a.cashInvoicesForMonth(curAccount, curYear, m)}
			for _, b := range storedBooks {
				if b.Konto == curAccount {
					mi.HasStoredBook = true
					mi.Book = b
					break
				}
			}
			months[i] = mi
		}
		carriedIn, _ := a.cashCarryIn(curAccount, curYear, time.January)
		summaries := core.ComputeYearOverview(carriedIn, months)

		tableArea.Objects = tableArea.Objects[:0]
		tableArea.Add(container.NewGridWithColumns(5,
			widget.NewLabelWithStyle("Monat", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			widget.NewLabelWithStyle("Anfangsbestand", fyne.TextAlignTrailing, fyne.TextStyle{Bold: true}),
			widget.NewLabelWithStyle("Einnahmen", fyne.TextAlignTrailing, fyne.TextStyle{Bold: true}),
			widget.NewLabelWithStyle("Ausgaben", fyne.TextAlignTrailing, fyne.TextStyle{Bold: true}),
			widget.NewLabelWithStyle("Endbestand", fyne.TextAlignTrailing, fyne.TextStyle{Bold: true}),
		))
		for _, s := range summaries {
			m := s.Month
			monthBtn := widget.NewButton(a.bundle.T(fmt.Sprintf("month.%02d", int(m))), func() {
				a.yearSelect.SetSelected(fmt.Sprintf("%d", curYear))
				a.monthSelect.SetSelected(
					fmt.Sprintf("%02d - %-12s", int(m), a.bundle.T(fmt.Sprintf("month.%02d", int(m)))))
				a.showCashBookView()
			})
			monthBtn.Importance = widget.LowImportance
			tableArea.Add(container.NewGridWithColumns(5,
				monthBtn,
				widget.NewLabelWithStyle(formatDecimal(s.Anfangsbestand, a.settings.DecimalSeparator),
					fyne.TextAlignTrailing, fyne.TextStyle{}),
				widget.NewLabelWithStyle(formatDecimal(s.Einnahmen, a.settings.DecimalSeparator),
					fyne.TextAlignTrailing, fyne.TextStyle{}),
				widget.NewLabelWithStyle(formatDecimal(s.Ausgaben, a.settings.DecimalSeparator),
					fyne.TextAlignTrailing, fyne.TextStyle{}),
				widget.NewLabelWithStyle(formatDecimal(s.Endbestand, a.settings.DecimalSeparator),
					fyne.TextAlignTrailing, fyne.TextStyle{}),
			))
		}
		tableArea.Refresh()
	}

	accountSelect.OnChanged = func(string) { rebuild() }
	yearSelect.OnChanged = func(s string) {
		var y int
		fmt.Sscanf(s, "%d", &y)
		if y != 0 {
			curYear = y
		}
		rebuild()
	}
	rebuild()

	header := container.NewBorder(nil, nil,
		container.NewPadded(titleLabel),
		container.NewPadded(backBtn),
		container.NewPadded(container.NewHBox(accountSelect, yearSelect)),
	)
	a.window.SetContent(container.NewBorder(
		header, nil, nil, nil, container.NewVScroll(tableArea)))
}
```

- [ ] **Step 3: Build, vet, test**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS — build and vet clean; `internal/core` tests pass; other packages report `no test files`.

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

Terminate any running `BuchISY` process (PowerShell `wmic process where "ProcessId=<id>" call terminate` per running PID, as established in this session).

- [ ] **Step 4: Deploy and restart**

Copy `cmd/buchisy/BuchISY.exe` over `C:\Users\istok\Desktop\BuchISY.exe`, then launch
`C:\Users\istok\Desktop\BuchISY.exe` with working directory `C:\Users\istok\Desktop`.

- [ ] **Step 5: Manual smoke test**

1. Oben Jahr/Monat wählen → „Kassenbuch" → in der Kassenbuch-Ansicht den Button „Jahresübersicht" klicken.
2. Die Tabelle zeigt alle zwölf Monate (Januar–Dezember) mit Anfangsbestand, Einnahmen, Ausgaben, Endbestand.
3. Im Beispiel: Anfangsbestand Januar 3497,35 € erfasst → jeder Folgemonat ohne Buchungen zeigt 3497,35 → 3497,35.
4. Konto- und Jahr-Auswahlfeld wechseln aktualisiert die Tabelle.
5. Klick auf eine Monatszeile (Monatsname) öffnet das Kassenbuch dieses Monats.

---

## Self-Review

**Spec coverage:**
- `MonthInput`/`MonthSummary` + `ComputeYearOverview` (Saldo-Rollung, gespeicherter Anfangsbestand überschreibt Übertrag, carriedIn) → Task 1.
- Übertrag-Helfer `cashCarryIn` als Methode extrahiert, `showCashBookView` unverändert im Verhalten → Task 2.
- „Jahresübersicht"-Button in der Kassenbuch-Ansicht → Task 3 Step 1.
- `showCashYearView`: Kopf mit Konto-/Jahr-Auswahl + Zurück, 12-Zeilen-Tabelle, anklickbare Monatszeilen → Task 3 Step 2.
- Daten je Monat laden (`kassenbuch.json` + `cashInvoicesForMonth`), `carriedIn` über `cashCarryIn(account, year, time.January)` → Task 3 Step 2 (`rebuild`).
- Beträge im Dezimalformat der Einstellungen → `formatDecimal(..., a.settings.DecimalSeparator)` in Task 3.
- Unit-Tests für `ComputeYearOverview` (gespeicherter Monat, leerer Monat, carriedIn) → Task 1.

**Placeholder scan:** Keine TBD/TODO; alle Code-Schritte enthalten vollständigen Code.

**Type consistency:** `MonthInput`/`MonthSummary`/`ComputeYearOverview(float64, []MonthInput) []MonthSummary` (Task 1) werden in Task 3 mit denselben Namen/Signaturen verwendet. `cashCarryIn(account string, year int, month time.Month) (float64, bool)` (Task 2) wird in Task 2 (`bookFor`) und Task 3 (`rebuild`) identisch aufgerufen. `showCashYearView(account string, year int)` (Task 3 Step 2) wird vom Button in Task 3 Step 1 mit dieser Signatur aufgerufen. Bestehende Symbole `formatDecimal`, `cashAccounts`, `cashInvoicesForMonth`, `generateYearOptions`, `a.yearSelect`, `a.monthSelect`, `a.storageManager.GetMonthFolder`, `core.LoadCashBooks`, `core.ComputeCashReport` werden unverändert genutzt.
