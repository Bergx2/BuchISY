# CSV-Export — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ein „CSV-Export"-Knopf erzeugt auf Wunsch eine CSV für einen Monat oder einen Zeitraum und bietet sie über einen Speichern-Dialog an.

**Architecture:** `CSVRepository` bekommt ein writer-basiertes `WriteTo`; ein neuer Export-Dialog (`csvexport.go`) sammelt die Rechnungen der gewählten Monate aus den vorhandenen `invoices.csv`-Dateien und schreibt sie an einen vom Nutzer gewählten Ort. Die interne Speicherung bleibt unverändert.

**Tech Stack:** Go 1.25, Fyne v2.6.3, Standard-`testing`.

**Hinweis zu Commits:** Git-Commits sind in diesem Plan bewusst ausgelassen. Jede Aufgabe endet mit `go build`/`go vet`/`go test`.

---

### Task 1: `CSVRepository.WriteTo` (writer-basiert, TDD)

**Files:**
- Modify: `internal/core/csvrepo.go`
- Test: `internal/core/csvrepo_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/core/csvrepo_test.go` (ergänze `"bytes"` und `"strings"` im Import-Block der Testdatei, falls noch nicht vorhanden):

```go
func TestCSVWriteTo(t *testing.T) {
	repo := NewCSVRepository()
	var buf bytes.Buffer
	rows := []CSVRow{
		{Dateiname: "a.pdf", Firmenname: "Foo GmbH"},
		{Dateiname: "b.pdf", Firmenname: "Bar AG"},
	}
	if err := repo.WriteTo(&buf, rows); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Dateiname") {
		t.Errorf("header missing in output: %q", out)
	}
	if !strings.Contains(out, "a.pdf") || !strings.Contains(out, "Foo GmbH") ||
		!strings.Contains(out, "b.pdf") || !strings.Contains(out, "Bar AG") {
		t.Errorf("row data missing in output: %q", out)
	}
	// Header + 2 data rows = 3 non-empty lines.
	lines := 0
	for _, ln := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if strings.TrimSpace(ln) != "" {
			lines++
		}
	}
	if lines != 3 {
		t.Errorf("expected 3 lines, got %d: %q", lines, out)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run TestCSVWriteTo -v`
Expected: FAIL — `WriteTo` undefined.

- [ ] **Step 3: Add `WriteTo` and route `Rewrite` through it**

In `internal/core/csvrepo.go`, the current `Rewrite` is:

```go
// Rewrite overwrites the CSV file with the provided rows using the current column order.
func (r *CSVRepository) Rewrite(path string, rows []CSVRow) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to recreate CSV: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	if err := writer.Write(r.GetHeader()); err != nil {
		return fmt.Errorf("failed to write CSV header: %w", err)
	}

	for _, row := range rows {
		if err := writer.Write(r.rowToRecord(row)); err != nil {
			return fmt.Errorf("failed to write CSV row: %w", err)
		}
	}

	if err := writer.Error(); err != nil {
		return fmt.Errorf("failed to flush CSV writer: %w", err)
	}

	return nil
}
```

Replace it with:

```go
// WriteTo writes the header and all rows as CSV to w, using the current
// column order.
func (r *CSVRepository) WriteTo(w io.Writer, rows []CSVRow) error {
	writer := csv.NewWriter(w)
	if err := writer.Write(r.GetHeader()); err != nil {
		return fmt.Errorf("failed to write CSV header: %w", err)
	}
	for _, row := range rows {
		if err := writer.Write(r.rowToRecord(row)); err != nil {
			return fmt.Errorf("failed to write CSV row: %w", err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return fmt.Errorf("failed to flush CSV writer: %w", err)
	}
	return nil
}

// Rewrite overwrites the CSV file with the provided rows using the current column order.
func (r *CSVRepository) Rewrite(path string, rows []CSVRow) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to recreate CSV: %w", err)
	}
	defer file.Close()
	return r.WriteTo(file, rows)
}
```

(`io` is already imported in `csvrepo.go`.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run TestCSVWriteTo -v`
Expected: PASS.

- [ ] **Step 5: Build, vet, full core tests**

Run: `go build ./... && go vet ./internal/core/... && go test ./internal/core/...`
Expected: PASS.

---

### Task 2: Export-Dialog `csvexport.go` + Knopf

**Files:**
- Create: `internal/ui/csvexport.go`
- Modify: `internal/ui/app.go`

- [ ] **Step 1: Create `internal/ui/csvexport.go`**

Create `internal/ui/csvexport.go`:

```go
package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/bergx2/buchisy/internal/core"
)

// showCSVExportDialog lets the user export the invoices of a month or a
// date range to a CSV file at a location of their choosing.
func (a *App) showCSVExportDialog() {
	const modeMonth = "Monat"
	const modeRange = "Zeitraum"

	yearSelect := widget.NewSelect(generateYearOptions(), nil)
	yearSelect.SetSelected(fmt.Sprintf("%d", a.currentYear))
	monthSelect := widget.NewSelect(generateMonthOptions(a.bundle), nil)
	monthSelect.SetSelected(fmt.Sprintf("%02d - %-12s", int(a.currentMonth),
		a.bundle.T(fmt.Sprintf("month.%02d", int(a.currentMonth)))))
	monthForm := container.NewGridWithColumns(2, yearSelect, monthSelect)

	fromEntry := widget.NewEntry()
	fromEntry.SetPlaceHolder("TT.MM.JJJJ")
	toEntry := widget.NewEntry()
	toEntry.SetPlaceHolder("TT.MM.JJJJ")
	rangeForm := container.NewVBox(
		container.NewBorder(nil, nil, widget.NewLabel("Von"), nil, fromEntry),
		container.NewBorder(nil, nil, widget.NewLabel("Bis"), nil, toEntry),
	)

	modeRadio := widget.NewRadioGroup([]string{modeMonth, modeRange}, func(sel string) {
		if sel == modeRange {
			monthForm.Hide()
			rangeForm.Show()
		} else {
			rangeForm.Hide()
			monthForm.Show()
		}
	})
	modeRadio.SetSelected(modeMonth)

	content := container.NewVBox(modeRadio, monthForm, rangeForm)

	dialog.ShowCustomConfirm("CSV-Export", "Exportieren", "Abbrechen", content,
		func(ok bool) {
			if !ok {
				return
			}
			fromY, fromM, toY, toM, err := csvExportRange(
				modeRadio.Selected == modeRange,
				yearSelect.Selected, monthSelect.Selected,
				fromEntry.Text, toEntry.Text)
			if err != nil {
				a.showError(a.bundle.T("error.processing.title"), err.Error())
				return
			}
			rows := a.collectInvoiceRows(fromY, fromM, toY, toM)
			if len(rows) == 0 {
				a.showInfo("CSV-Export", "Keine Rechnungen im gewählten Bereich.")
				return
			}
			a.saveExportCSV(rows)
		}, a.window)
}

// csvExportRange turns the dialog inputs into an inclusive month range
// (fromYear, fromMonth) .. (toYear, toMonth).
func csvExportRange(isRange bool, yearSel, monthSel, fromTxt, toTxt string) (int, int, int, int, error) {
	if isRange {
		fy, fm, ok1 := parseDateMonth(fromTxt)
		ty, tm, ok2 := parseDateMonth(toTxt)
		if !ok1 || !ok2 {
			return 0, 0, 0, 0, fmt.Errorf("Bitte gültige Datumsangaben (TT.MM.JJJJ) eingeben.")
		}
		if ty < fy || (ty == fy && tm < fm) {
			return 0, 0, 0, 0, fmt.Errorf("Das Bis-Datum liegt vor dem Von-Datum.")
		}
		return fy, fm, ty, tm, nil
	}
	y, _ := strconv.Atoi(strings.TrimSpace(yearSel))
	m := 0
	if len(monthSel) >= 2 {
		m, _ = strconv.Atoi(strings.TrimSpace(monthSel[:2]))
	}
	if y == 0 || m < 1 || m > 12 {
		return 0, 0, 0, 0, fmt.Errorf("Bitte Jahr und Monat wählen.")
	}
	return y, m, y, m, nil
}

// parseDateMonth parses a TT.MM.JJJJ string and returns its year and month.
func parseDateMonth(s string) (year, month int, ok bool) {
	parts := strings.Split(strings.TrimSpace(s), ".")
	if len(parts) != 3 {
		return 0, 0, false
	}
	m, e1 := strconv.Atoi(parts[1])
	y, e2 := strconv.Atoi(parts[2])
	if e1 != nil || e2 != nil || m < 1 || m > 12 || y < 1 {
		return 0, 0, false
	}
	return y, m, true
}

// collectInvoiceRows gathers all invoice rows from the monthly CSVs in the
// inclusive month range.
func (a *App) collectInvoiceRows(fromY, fromM, toY, toM int) []core.CSVRow {
	var rows []core.CSVRow
	y, m := fromY, fromM
	for y < toY || (y == toY && m <= toM) {
		monthRows, err := a.csvRepo.Load(a.storageManager.GetCSVPath(y, time.Month(m)))
		if err != nil {
			a.logger.Warn("CSV-Export: Monat %04d-%02d übersprungen: %v", y, m, err)
		} else {
			rows = append(rows, monthRows...)
		}
		m++
		if m > 12 {
			m = 1
			y++
		}
	}
	return rows
}

// saveExportCSV asks for a target file and writes the rows there.
func (a *App) saveExportCSV(rows []core.CSVRow) {
	d := dialog.NewFileSave(func(w fyne.URIWriteCloser, err error) {
		if err != nil || w == nil {
			return
		}
		defer w.Close()
		if werr := a.csvRepo.WriteTo(w, rows); werr != nil {
			a.showError(a.bundle.T("error.processing.title"), werr.Error())
			return
		}
		a.logger.Info("CSV-Export: %d Rechnungen exportiert", len(rows))
	}, a.window)
	d.SetFileName("Export.csv")
	d.Show()
}
```

- [ ] **Step 2: Add the „CSV-Export" button to the top bar**

In `internal/ui/app.go`, `buildTopBar`, the Kassenbuch button is currently:

```go
	// Kassenbuch button
	cashBookBtn := widget.NewButton("Kassenbuch", func() {
		a.showCashBookView()
	})
```

Directly after it, add:

```go
	// CSV export button
	csvExportBtn := widget.NewButton("CSV-Export", func() {
		a.showCSVExportDialog()
	})
```

In the same function, the returned `container.NewHBox(...)` currently ends:

```go
		openFolderBtn,
		cashBookBtn,
		settingsBtn,
	)
```

Change it to:

```go
		openFolderBtn,
		cashBookBtn,
		csvExportBtn,
		settingsBtn,
	)
```

- [ ] **Step 3: Build, vet, test**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS.

---

### Task 3: Build, Paketierung, Auslieferung

**Files:** none (build/deploy only)

- [ ] **Step 1: Final build + vet + tests**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: all succeed.

- [ ] **Step 2: Package the Windows executable**

Run (from `C:\Users\istok\Desktop\Dev\BuchISY`):
`fyne package -os windows -name BuchISY -src ./cmd/buchisy`
Expected: `cmd/buchisy/BuchISY.exe` produced.

- [ ] **Step 3: Stop the running app**

Terminate any running `BuchISY` process (PowerShell `wmic process where "ProcessId=<id>" call terminate` per running PID).

- [ ] **Step 4: Deploy and restart**

Copy `cmd/buchisy/BuchISY.exe` over `C:\Users\istok\Desktop\BuchISY.exe`, then launch
`C:\Users\istok\Desktop\BuchISY.exe` with working directory `C:\Users\istok\Desktop`.

- [ ] **Step 5: Manual smoke test**

1. Im Hauptfenster gibt es den Knopf „CSV-Export".
2. Klick → Dialog mit Auswahl „Monat" / „Zeitraum".
3. „Monat" → ein Monat mit Rechnungen wählen → Exportieren → Speichern-Dialog → die CSV enthält die Rechnungen des Monats.
4. „Zeitraum" → von/bis über zwei Monate → eine CSV mit den Zeilen beider Monate.
5. Monat/Zeitraum ohne Rechnungen → Hinweis „Keine Rechnungen im gewählten Bereich".
6. Die internen `invoices.csv` in den Monatsordnern sind unverändert.

---

## Self-Review

**Spec coverage:**
- Export-Knopf im Hauptfenster → Task 2 Step 2 (`csvExportBtn` in `buildTopBar`).
- Dialog mit „Monat" / „Zeitraum" → Task 2 Step 1 (`modeRadio`, `monthForm`, `rangeForm`).
- Sammeln aus den monatlichen `invoices.csv` → `collectInvoiceRows` (Monatsbereich, `csvRepo.Load` je Monat).
- Eine CSV, gleiche Spalten wie intern → `csvRepo.WriteTo` nutzt dieselbe Spaltenreihenfolge.
- Speichern-Dialog, freier Ort → `saveExportCSV` via `dialog.NewFileSave`.
- Interne Speicherung unverändert → es wird nur gelesen (`Load`) und in die vom Nutzer gewählte Datei geschrieben; keine Monatsordner-CSV wird angefasst.
- Edge: kein Treffer → Hinweis statt leerer Datei (`len(rows) == 0`); ungültige Datumseingabe → Fehlerhinweis (`csvExportRange`); Bis vor Von → Fehlerhinweis.
- Unit-Test `WriteTo` → Task 1.

**Placeholder scan:** Keine TBD/TODO; `csvexport.go` und der `WriteTo`/`Rewrite`-Block sind vollständig angegeben.

**Type consistency:** `(r *CSVRepository) WriteTo(io.Writer, []CSVRow) error` (Task 1) wird in `saveExportCSV` (Task 2) genutzt. `Rewrite` ruft `WriteTo` mit derselben Signatur. `a.csvRepo.Load(string) ([]CSVRow, error)`, `a.storageManager.GetCSVPath(int, time.Month) string`, `generateYearOptions()`, `generateMonthOptions(*i18n.Bundle)`, `a.showError`/`a.showInfo`, `a.currentYear`/`a.currentMonth` sind bestehende Member/Funktionen. `csvExportRange` und `parseDateMonth` sind neue paket-lokale Funktionen in `csvexport.go`, genutzt von `showCSVExportDialog`.
