# Kassenbuch mit Kassenbericht-PDF Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Für Barkassen-Konten ein monatliches Kassenbuch (Anfangsbestand + Einlagen + Bar-Rechnungen → fortlaufender Saldo) mit Erfassungs-Ansicht und PDF-Kassenbericht.

**Architecture:** Reine Kern-Logik in `internal/core` (Modell, JSON-Persistenz je Monatsordner, Saldo-Berechnung) — TDD-getestet. Die PDF-Erzeugung nutzt die reine Go-Bibliothek `go-pdf/fpdf`. Eine neue Vollseiten-Ansicht in `internal/ui` erfasst Anfangsbestand/Einlagen und löst die PDF-Erzeugung aus.

**Tech Stack:** Go 1.25, Fyne v2.6.3, `github.com/go-pdf/fpdf` (reines Go), Standard-`testing`.

**Hinweis zu Commits:** Git-Commits sind in diesem Plan bewusst ausgelassen — Auslieferung per Build + Kopie der `.exe`. Jede Aufgabe endet mit `go build`/`go vet`/`go test` als Verifikation.

---

### Task 1: Datenmodell + Persistenz (TDD)

**Files:**
- Create: `internal/core/kassenbuch.go`
- Test: `internal/core/kassenbuch_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/core/kassenbuch_test.go`:

```go
package core

import (
	"path/filepath"
	"testing"
)

func TestCashBooksRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kassenbuch.json")

	books := []CashBook{
		{
			Konto:          "Barkasse",
			Anfangsbestand: 200.50,
			Einlagen: []CashDeposit{
				{Datum: "03.05.2026", Beschreibung: "Bankabhebung", Betrag: 300},
			},
		},
	}
	if err := SaveCashBooks(path, books); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := LoadCashBooks(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != 1 || got[0].Konto != "Barkasse" || got[0].Anfangsbestand != 200.50 {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
	if len(got[0].Einlagen) != 1 || got[0].Einlagen[0].Betrag != 300 {
		t.Fatalf("deposits mismatch: %+v", got[0].Einlagen)
	}
}

func TestLoadCashBooksMissingFile(t *testing.T) {
	got, err := LoadCashBooks(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("missing file should yield empty slice, got %+v", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run "TestCashBooks|TestLoadCashBooks" -v`
Expected: FAIL — `undefined: CashBook`, `CashDeposit`, `SaveCashBooks`, `LoadCashBooks`.

- [ ] **Step 3: Write the model and persistence**

Create `internal/core/kassenbuch.go`:

```go
package core

import (
	"encoding/json"
	"fmt"
	"os"
)

// CashDeposit is a single cash inflow into a cash register (Bar-Einlage).
type CashDeposit struct {
	Datum        string  `json:"datum"` // DD.MM.YYYY
	Beschreibung string  `json:"beschreibung"`
	Betrag       float64 `json:"betrag"`
}

// CashBook is the monthly cash book of one cash-register account.
type CashBook struct {
	Konto          string        `json:"konto"`
	Anfangsbestand float64       `json:"anfangsbestand"`
	Einlagen       []CashDeposit `json:"einlagen"`
}

// LoadCashBooks reads the cash books stored in a month folder's
// kassenbuch.json. A missing file yields an empty slice and no error.
func LoadCashBooks(path string) ([]CashBook, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return []CashBook{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read cash book: %w", err)
	}
	var books []CashBook
	if err := json.Unmarshal(data, &books); err != nil {
		return nil, fmt.Errorf("failed to parse cash book: %w", err)
	}
	return books, nil
}

// SaveCashBooks writes the cash books to kassenbuch.json.
func SaveCashBooks(path string, books []CashBook) error {
	data, err := json.MarshalIndent(books, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cash book: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write cash book: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run "TestCashBooks|TestLoadCashBooks" -v`
Expected: PASS — both tests ok.

---

### Task 2: Saldo-Berechnung (TDD)

**Files:**
- Modify: `internal/core/kassenbuch.go`
- Test: `internal/core/kassenbuch_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/core/kassenbuch_test.go`:

```go
func TestComputeCashReport(t *testing.T) {
	book := CashBook{
		Konto:          "Barkasse",
		Anfangsbestand: 100,
		Einlagen: []CashDeposit{
			{Datum: "10.05.2026", Beschreibung: "Einlage", Betrag: 50},
		},
	}
	invoices := []CSVRow{
		{Firmenname: "Spät", Dateiname: "b.pdf", Bruttobetrag: 20, Bezahldatum: "05.05.2026"},
		{Firmenname: "Früh", Dateiname: "a.pdf", Bruttobetrag: 30, Rechnungsdatum: "01.05.2026"}, // no Bezahldatum
	}

	entries, end := ComputeCashReport(book, invoices)
	if len(entries) != 3 {
		t.Fatalf("got %d entries, want 3", len(entries))
	}
	// Chronological: 01.05 (Früh, -30), 05.05 (Spät, -20), 10.05 (Einlage, +50)
	if entries[0].Beschreibung != "Früh" || entries[0].Saldo != 70 {
		t.Errorf("entry 0 = %+v", entries[0])
	}
	if entries[1].Beschreibung != "Spät" || entries[1].Saldo != 50 {
		t.Errorf("entry 1 = %+v", entries[1])
	}
	if entries[2].Einnahme != 50 || entries[2].Saldo != 100 {
		t.Errorf("entry 2 = %+v", entries[2])
	}
	if end != 100 {
		t.Errorf("endbestand = %v, want 100", end)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run TestComputeCashReport -v`
Expected: FAIL — `undefined: ComputeCashReport`, `CashEntry`.

- [ ] **Step 3: Implement the computation**

Append to `internal/core/kassenbuch.go` (and extend the import block — see below):

```go
// CashEntry is one line of a computed cash report.
type CashEntry struct {
	Datum        string
	Beschreibung string
	Beleg        string
	Einnahme     float64
	Ausgabe      float64
	Saldo        float64
}

// ComputeCashReport combines the cash book's opening balance and deposits
// with the cash-paid invoices into a chronologically ordered running
// balance. invoices must already be filtered to this cash account.
// Invoices use Bezahldatum for ordering, falling back to Rechnungsdatum;
// entries with an unparseable date are sorted last.
func ComputeCashReport(book CashBook, invoices []CSVRow) ([]CashEntry, float64) {
	type dated struct {
		entry CashEntry
		t     time.Time
		ok    bool
	}
	items := make([]dated, 0, len(book.Einlagen)+len(invoices))

	for _, d := range book.Einlagen {
		t, ok := parseGermanDate(d.Datum)
		items = append(items, dated{
			entry: CashEntry{Datum: d.Datum, Beschreibung: d.Beschreibung, Einnahme: d.Betrag},
			t:     t, ok: ok,
		})
	}
	for _, inv := range invoices {
		dateStr := inv.Bezahldatum
		if dateStr == "" {
			dateStr = inv.Rechnungsdatum
		}
		t, ok := parseGermanDate(dateStr)
		items = append(items, dated{
			entry: CashEntry{Datum: dateStr, Beschreibung: inv.Firmenname, Beleg: inv.Dateiname, Ausgabe: inv.Bruttobetrag},
			t:     t, ok: ok,
		})
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].ok != items[j].ok {
			return items[i].ok // datable entries first
		}
		if !items[i].ok {
			return false
		}
		return items[i].t.Before(items[j].t)
	})

	saldo := book.Anfangsbestand
	entries := make([]CashEntry, len(items))
	for i, it := range items {
		saldo += it.entry.Einnahme - it.entry.Ausgabe
		e := it.entry
		e.Saldo = saldo
		entries[i] = e
	}
	return entries, saldo
}

// parseGermanDate parses a DD.MM.YYYY date.
func parseGermanDate(s string) (time.Time, bool) {
	t, err := time.Parse("02.01.2006", strings.TrimSpace(s))
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}
```

Change the import block at the top of `internal/core/kassenbuch.go` from:

```go
import (
	"encoding/json"
	"fmt"
	"os"
)
```

to:

```go
import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run TestComputeCashReport -v`
Expected: PASS.

- [ ] **Step 5: Build the package**

Run: `go build ./internal/core/ && go vet ./internal/core/...`
Expected: PASS.

---

### Task 3: Kassenbericht-PDF

**Files:**
- Modify: `go.mod`, `go.sum`
- Create: `internal/core/kassenbericht.go`

- [ ] **Step 1: Add the PDF dependency**

Run: `go get github.com/go-pdf/fpdf@latest`
Expected: `go.mod`/`go.sum` get the entry, no errors. (`fpdf` is pure Go — no CGO.)

- [ ] **Step 2: Write the PDF generator**

Create `internal/core/kassenbericht.go`:

```go
package core

import (
	"fmt"
	"strings"

	"github.com/go-pdf/fpdf"
)

// formatCashAmount formats an amount with the configured decimal separator.
func formatCashAmount(v float64, decimalSep string) string {
	s := fmt.Sprintf("%.2f", v)
	if decimalSep == "," {
		s = strings.ReplaceAll(s, ".", ",")
	}
	return s
}

// truncateRunes shortens s to at most max runes, adding "..." if cut.
func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max < 4 {
		return string(r[:max])
	}
	return string(r[:max-3]) + "..."
}

// WriteCashReportPDF writes a monthly cash report (Kassenbericht) for one
// cash account to a landscape A4 PDF at path.
func WriteCashReportPDF(path string, book CashBook, entries []CashEntry, endbestand float64, monthLabel, decimalSep string) error {
	pdf := fpdf.New("L", "mm", "A4", "")
	tr := pdf.UnicodeTranslatorFromDescriptor("") // UTF-8 -> cp1252 for core fonts
	pdf.AddPage()

	pdf.SetFont("Arial", "B", 14)
	pdf.CellFormat(0, 10, tr("Kassenbericht - "+book.Konto), "", 1, "L", false, 0, "")
	pdf.SetFont("Arial", "", 10)
	pdf.CellFormat(0, 6, tr("Monat: "+monthLabel), "", 1, "L", false, 0, "")
	pdf.CellFormat(0, 6, tr("Anfangsbestand: "+formatCashAmount(book.Anfangsbestand, decimalSep)), "", 1, "L", false, 0, "")
	pdf.Ln(3)

	widths := []float64{25, 80, 75, 30, 30, 32}
	headers := []string{"Datum", "Beschreibung", "Beleg", "Einnahme", "Ausgabe", "Saldo"}
	pdf.SetFont("Arial", "B", 9)
	for i, h := range headers {
		pdf.CellFormat(widths[i], 7, tr(h), "1", 0, "C", false, 0, "")
	}
	pdf.Ln(-1)

	pdf.SetFont("Arial", "", 9)
	for _, e := range entries {
		einnahme := ""
		if e.Einnahme != 0 {
			einnahme = formatCashAmount(e.Einnahme, decimalSep)
		}
		ausgabe := ""
		if e.Ausgabe != 0 {
			ausgabe = formatCashAmount(e.Ausgabe, decimalSep)
		}
		pdf.CellFormat(widths[0], 6, tr(e.Datum), "1", 0, "L", false, 0, "")
		pdf.CellFormat(widths[1], 6, tr(truncateRunes(e.Beschreibung, 50)), "1", 0, "L", false, 0, "")
		pdf.CellFormat(widths[2], 6, tr(truncateRunes(e.Beleg, 48)), "1", 0, "L", false, 0, "")
		pdf.CellFormat(widths[3], 6, tr(einnahme), "1", 0, "R", false, 0, "")
		pdf.CellFormat(widths[4], 6, tr(ausgabe), "1", 0, "R", false, 0, "")
		pdf.CellFormat(widths[5], 6, tr(formatCashAmount(e.Saldo, decimalSep)), "1", 0, "R", false, 0, "")
		pdf.Ln(-1)
	}

	pdf.Ln(3)
	pdf.SetFont("Arial", "B", 11)
	pdf.CellFormat(0, 7, tr("Endbestand: "+formatCashAmount(endbestand, decimalSep)), "", 1, "L", false, 0, "")

	return pdf.OutputFileAndClose(path)
}
```

- [ ] **Step 3: Build to verify fpdf compiles**

Run: `go build ./...`
Expected: PASS. (`WriteCashReportPDF` is not yet called — fine; it is wired up in Task 4. No unit test: the PDF output is verified manually in Task 5.)

- [ ] **Step 4: Vet and core tests**

Run: `go vet ./internal/core/... && go test ./internal/core/...`
Expected: PASS.

---

### Task 4: Kassenbuch-Ansicht + Top-Bar-Button

**Files:**
- Create: `internal/ui/kassenbuchview.go`
- Modify: `internal/ui/app.go`

- [ ] **Step 1: Add the "Kassenbuch" button to the top bar**

In `internal/ui/app.go`, `buildTopBar` currently ends:

```go
	// Settings button
	settingsBtn := widget.NewButton(a.bundle.T("menu.settings"), func() {
		a.showSettingsView()
	})

	return container.NewHBox(
		widget.NewLabel(a.bundle.T("app.title")),
		widget.NewLabel(" | "),
		a.yearSelect,
		widget.NewLabel("-"),
		monthContainer,
		widget.NewSeparator(),
		openFolderBtn,
		settingsBtn,
	)
}
```

Replace that with:

```go
	// Settings button
	settingsBtn := widget.NewButton(a.bundle.T("menu.settings"), func() {
		a.showSettingsView()
	})

	// Kassenbuch button
	cashBookBtn := widget.NewButton("Kassenbuch", func() {
		a.showCashBookView()
	})

	return container.NewHBox(
		widget.NewLabel(a.bundle.T("app.title")),
		widget.NewLabel(" | "),
		a.yearSelect,
		widget.NewLabel("-"),
		monthContainer,
		widget.NewSeparator(),
		openFolderBtn,
		cashBookBtn,
		settingsBtn,
	)
}
```

- [ ] **Step 2: Create the cash-book view**

Create `internal/ui/kassenbuchview.go`:

```go
package ui

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/bergx2/buchisy/internal/core"
)

// parseAmount parses a decimal number that may use a comma separator.
func parseAmount(s string) float64 {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", ".")
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}

// cashAccounts returns the names of all bank accounts of type cash.
func (a *App) cashAccounts() []string {
	var names []string
	for _, ba := range a.settings.BankAccounts {
		if ba.AccountType == core.AccountTypeCash {
			names = append(names, ba.Name)
		}
	}
	return names
}

// cashInvoicesFor returns the current month's invoices booked to the named
// cash account.
func (a *App) cashInvoicesFor(account string) []core.CSVRow {
	csvPath := a.storageManager.GetCSVPath(a.currentYear, a.currentMonth)
	rows, err := a.csvRepo.Load(csvPath)
	if err != nil {
		return nil
	}
	var out []core.CSVRow
	for _, r := range rows {
		if r.Bankkonto == account {
			out = append(out, r)
		}
	}
	return out
}

// showCashBookView replaces the window content with the cash-book view
// for the currently selected month.
func (a *App) showCashBookView() {
	monthFolder := a.storageManager.GetMonthFolder(a.currentYear, a.currentMonth)
	jsonPath := filepath.Join(monthFolder, "kassenbuch.json")
	monthLabel := fmt.Sprintf("%04d-%02d", a.currentYear, a.currentMonth)

	books, err := core.LoadCashBooks(jsonPath)
	if err != nil {
		a.logger.Warn("Failed to load cash book: %v", err)
		books = nil
	}

	// bookFor returns a pointer to the working CashBook for an account,
	// creating it in books if absent.
	bookFor := func(account string) *core.CashBook {
		for i := range books {
			if books[i].Konto == account {
				return &books[i]
			}
		}
		books = append(books, core.CashBook{Konto: account})
		return &books[len(books)-1]
	}

	accounts := a.cashAccounts()

	titleLabel := widget.NewLabelWithStyle(
		"Kassenbuch — "+monthLabel, fyne.TextAlignLeading, fyne.TextStyle{Bold: true},
	)
	backBtn := widget.NewButton(a.bundle.T("btn.cancel"), func() { a.showMainView() })

	body := container.NewVBox()

	if len(accounts) == 0 {
		body.Add(widget.NewLabel(
			"Kein Konto vom Typ \"Barkasse\" vorhanden. Lege in den Einstellungen unter Konten ein Zahlungskonto mit Typ \"Barkasse\" an.",
		))
		header := container.NewBorder(nil, nil, container.NewPadded(titleLabel),
			container.NewPadded(backBtn))
		a.window.SetContent(container.NewBorder(header, nil, nil, nil, container.NewVScroll(body)))
		return
	}

	// Account selector
	accountSelect := widget.NewSelect(accounts, nil)
	accountSelect.SetSelected(accounts[0])

	// Per-account editing area, rebuilt when the account changes.
	editArea := container.NewVBox()

	var rebuild func()
	rebuild = func() {
		account := accountSelect.Selected
		book := bookFor(account)
		editArea.Objects = editArea.Objects[:0]

		startEntry := widget.NewEntry()
		startEntry.SetText(fmt.Sprintf("%.2f", book.Anfangsbestand))
		startEntry.OnChanged = func(s string) { book.Anfangsbestand = parseAmount(s) }

		depositList := container.NewVBox()
		var refreshDeposits func()
		refreshDeposits = func() {
			depositList.Objects = depositList.Objects[:0]
			for i := range book.Einlagen {
				idx := i
				dateE := widget.NewEntry()
				dateE.SetPlaceHolder("TT.MM.JJJJ")
				dateE.SetText(book.Einlagen[idx].Datum)
				dateE.OnChanged = func(s string) { book.Einlagen[idx].Datum = s }

				descE := widget.NewEntry()
				descE.SetPlaceHolder("Beschreibung")
				descE.SetText(book.Einlagen[idx].Beschreibung)
				descE.OnChanged = func(s string) { book.Einlagen[idx].Beschreibung = s }

				amtE := widget.NewEntry()
				amtE.SetPlaceHolder("Betrag")
				amtE.SetText(fmt.Sprintf("%.2f", book.Einlagen[idx].Betrag))
				amtE.OnChanged = func(s string) { book.Einlagen[idx].Betrag = parseAmount(s) }

				removeBtn := widget.NewButton("Entfernen", func() {
					book.Einlagen = append(book.Einlagen[:idx], book.Einlagen[idx+1:]...)
					refreshDeposits()
				})
				removeBtn.Importance = widget.LowImportance

				row := container.NewBorder(nil, nil, nil, removeBtn,
					container.NewGridWithColumns(3, dateE, descE, amtE))
				depositList.Add(row)
			}
			depositList.Refresh()
		}
		refreshDeposits()

		addDepositBtn := widget.NewButton("+ Einlage", func() {
			book.Einlagen = append(book.Einlagen, core.CashDeposit{})
			refreshDeposits()
		})

		// Read-only cash invoices + computed end balance
		invoices := a.cashInvoicesFor(account)
		entries, endbestand := core.ComputeCashReport(*book, invoices)

		outflowList := container.NewVBox()
		for _, e := range entries {
			if e.Ausgabe == 0 {
				continue
			}
			outflowList.Add(widget.NewLabel(fmt.Sprintf(
				"  %s  —  %s  —  %.2f", e.Datum, e.Beschreibung, e.Ausgabe)))
		}
		if len(outflowList.Objects) == 0 {
			outflowList.Add(widget.NewLabel("  (keine Bar-Ausgaben in diesem Monat)"))
		}

		editArea.Add(widget.NewForm(
			widget.NewFormItem("Anfangsbestand", startEntry),
		))
		editArea.Add(widget.NewLabelWithStyle("Einlagen", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
		editArea.Add(depositList)
		editArea.Add(addDepositBtn)
		editArea.Add(widget.NewSeparator())
		editArea.Add(widget.NewLabelWithStyle("Bar-Ausgaben (aus Rechnungen)", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
		editArea.Add(outflowList)
		editArea.Add(widget.NewSeparator())
		editArea.Add(widget.NewLabelWithStyle(
			fmt.Sprintf("Endbestand: %.2f", endbestand), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
		editArea.Refresh()
	}
	accountSelect.OnChanged = func(string) { rebuild() }
	rebuild()

	saveBtn := widget.NewButton(a.bundle.T("btn.save"), func() {
		if err := core.SaveCashBooks(jsonPath, books); err != nil {
			dialog.ShowInformation(a.bundle.T("error.processing.title"), err.Error(), a.window)
			return
		}
		rebuild() // refresh computed balance
	})
	saveBtn.Importance = widget.HighImportance

	pdfBtn := widget.NewButton("Kassenbericht PDF", func() {
		if err := core.SaveCashBooks(jsonPath, books); err != nil {
			dialog.ShowInformation(a.bundle.T("error.processing.title"), err.Error(), a.window)
			return
		}
		var made []string
		for _, acc := range accounts {
			book := bookFor(acc)
			invoices := a.cashInvoicesFor(acc)
			entries, endbestand := core.ComputeCashReport(*book, invoices)
			outPath := filepath.Join(monthFolder,
				"Kassenbericht_"+core.SanitizeFilename(acc)+"_"+monthLabel+".pdf")
			if err := core.WriteCashReportPDF(outPath, *book, entries, endbestand, monthLabel, a.settings.DecimalSeparator); err != nil {
				dialog.ShowInformation(a.bundle.T("error.processing.title"), err.Error(), a.window)
				return
			}
			made = append(made, filepath.Base(outPath))
		}
		dialog.ShowInformation("Kassenbericht",
			"Erstellt:\n"+strings.Join(made, "\n"), a.window)
	})

	header := container.NewBorder(nil, nil,
		container.NewPadded(titleLabel),
		container.NewPadded(container.NewHBox(backBtn, saveBtn, pdfBtn)),
		container.NewPadded(accountSelect),
	)

	a.window.SetContent(container.NewBorder(
		header, nil, nil, nil, container.NewVScroll(editArea)))
}
```

- [ ] **Step 3: Build, vet, test**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS — build and vet clean; `internal/core` tests pass; other packages report `no test files`.

---

### Task 5: Build, Paketierung, Auslieferung

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

Prerequisite: in Einstellungen → Konten → Zahlungskonten ein Konto mit Typ
„Barkasse" anlegen.
1. Oben „Kassenbuch" klicken → Ansicht für den gewählten Monat öffnet.
2. Anfangsbestand eingeben, „+ Einlage" → eine Einlage mit Datum/Beschreibung/Betrag erfassen.
3. „Speichern" → der Endbestand aktualisiert sich; `kassenbuch.json` liegt im Monatsordner.
4. Ansicht schließen und neu öffnen → die Werte sind erhalten.
5. „Kassenbericht PDF" → im Monatsordner liegt `Kassenbericht_<Konto>_<YYYY-MM>.pdf`
   mit korrektem fortlaufendem Saldo und Endbestand.

---

## Self-Review

**Spec coverage:**
- `CashDeposit` / `CashBook` Modell + `kassenbuch.json`-Persistenz → Task 1.
- `ComputeCashReport` + `CashEntry`, chronologische Sortierung, Saldo, Bezahldatum-Fallback → Task 2.
- PDF via `go-pdf/fpdf`, je Konto eine PDF im Monatsordner, Dezimaltrennzeichen, Sanitize-Dateiname → Task 3 (`WriteCashReportPDF`) + Task 4 (Aufruf, Pfadbildung mit `core.SanitizeFilename`).
- Kassenbuch-Ansicht: Top-Bar-Button, Anfangsbestand, Einlagen-Liste, read-only Bar-Ausgaben, Endbestand, Speichern → Task 4.
- Kein Barkassen-Konto → Hinweis → Task 4 Step 2.
- Unit-Tests `ComputeCashReport` + Persistenz-Round-Trip + fehlende Datei → Tasks 1 & 2.

**Placeholder scan:** Keine TBD/TODO; alle Code-Schritte enthalten vollständigen Code.

**Type consistency:** `CashBook`/`CashDeposit`/`CashEntry` (Tasks 1-2) werden in Tasks 3-4 mit denselben Feldnamen verwendet. `LoadCashBooks`/`SaveCashBooks([]CashBook)`/`ComputeCashReport(CashBook, []CSVRow) ([]CashEntry, float64)`/`WriteCashReportPDF(string, CashBook, []CashEntry, float64, string, string) error` — die Signaturen stimmen zwischen Definition und Aufrufstellen überein. `core.SanitizeFilename` und `core.AccountTypeCash` sind bestehende Symbole. Die Top-Bar-Methode `showCashBookView` (Task 4 Step 1 Aufruf) wird in Task 4 Step 2 definiert.
