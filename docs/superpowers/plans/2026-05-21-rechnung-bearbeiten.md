# Rechnung-bearbeiten überarbeiten + Dateiname-Verbesserungen Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Editierbares Dateiname-Feld in beiden Rechnungs-Dialogen, Komma im Dateinamen erhalten, Dezimal-Fix und änderbarer Ablagemonat in „Rechnung bearbeiten", und Umstellung dieses Dialogs auf ein größenänderbares Fenster mit Belegvorschau.

**Architecture:** Eine Ein-Zeichen-Korrektur an `SanitizeFilename` (Komma). In `invoicemodal.go` und `tableedit.go` wird die Dateiname-Vorschau ein editierbares Feld; `saveInvoice`/`updateInvoice` erhalten den Dateinamen als Parameter. `updateInvoice` kann zusätzlich in einen anderen Monatsordner verschieben. `showEditDialog` wird von einem Fyne-Dialog auf ein eigenes Fenster mit `buildDocumentPreview` umgestellt.

**Tech Stack:** Go 1.25, Fyne v2.6.3, Standard-`testing`.

**Hinweis zu Commits:** Git-Commits sind in diesem Plan bewusst ausgelassen — Auslieferung per Build + Kopie der `.exe`. Jede Aufgabe endet mit `go build`/`go vet`/`go test`.

---

### Task 1: Komma im Dateinamen erhalten (TDD)

**Files:**
- Modify: `internal/core/sanitize.go`
- Test: `internal/core/sanitize_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/core/sanitize_test.go`:

```go
package core

import "testing"

func TestSanitizeFilenameKeepsComma(t *testing.T) {
	got := SanitizeFilename("2026-05-21_Foo_EUR_15,23.pdf")
	want := "2026-05-21_Foo_EUR_15,23.pdf"
	if got != want {
		t.Errorf("SanitizeFilename = %q, want %q (comma must be kept)", got, want)
	}
}

func TestSanitizeFilenameRemovesUnsafe(t *testing.T) {
	got := SanitizeFilename(`a<b>c:d"e|f?g*h.pdf`)
	want := "abcdefgh.pdf"
	if got != want {
		t.Errorf("SanitizeFilename = %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run TestSanitizeFilename -v`
Expected: `TestSanitizeFilenameKeepsComma` FAILS (current result is `2026-05-21_Foo_EUR_1523.pdf` — the comma is stripped). `TestSanitizeFilenameRemovesUnsafe` passes.

- [ ] **Step 3: Remove the comma from the unsafe-character set**

In `internal/core/sanitize.go`, the line is:

```go
	unsafeChars := regexp.MustCompile(`[<>:"|?*,\x00-\x1f]`)
```

Change it to (drop the `,`):

```go
	unsafeChars := regexp.MustCompile(`[<>:"|?*\x00-\x1f]`)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run TestSanitizeFilename -v`
Expected: both tests PASS.

- [ ] **Step 5: Build and vet**

Run: `go build ./... && go vet ./internal/core/... && go test ./internal/core/...`
Expected: PASS.

---

### Task 2: Editierbares Dateiname-Feld — „Rechnungsdaten prüfen"

**Files:**
- Modify: `internal/ui/invoicemodal.go`

- [ ] **Step 1: Turn the filename preview into an editable entry**

In `internal/ui/invoicemodal.go`, this block currently is:

```go
	filenamePreview := newCopyableLabel(a.bundle, "")
	filenamePreview.Wrapping = fyne.TextWrapBreak
	updateFilenamePreview := func() {
```

Replace it with:

```go
	filenameEntry := widget.NewEntry()
	filenameEdited := false
	suppressFilenameChange := false
	updateFilenamePreview := func() {
		if filenameEdited {
			return
		}
```

(Note the added `if filenameEdited { return }` as the new first line of the function body.)

- [ ] **Step 2: Write the generated name through the suppress guard**

In the same `updateFilenamePreview`, the tail currently is:

```go
		if err != nil {
			filenamePreview.SetText("Fehler: " + err.Error())
		} else {
			filenamePreview.SetText(filename)
		}
	}
```

Replace it with:

```go
		suppressFilenameChange = true
		if err != nil {
			filenameEntry.SetText("Fehler: " + err.Error())
		} else {
			filenameEntry.SetText(filename)
		}
		suppressFilenameChange = false
	}

	filenameEntry.OnChanged = func(string) {
		if !suppressFilenameChange {
			filenameEdited = true
		}
	}
```

- [ ] **Step 3: Use the entry in the form layout**

In `internal/ui/invoicemodal.go`, the form layout currently contains:

```go
		widget.NewLabel(a.bundle.T("modal.filenamePreview")),
		filenamePreview,
```

Change the second line:

```go
		widget.NewLabel(a.bundle.T("modal.filenamePreview")),
		filenameEntry,
```

- [ ] **Step 4: Pass the filename into `saveInvoice`**

In `internal/ui/invoicemodal.go`, the `saveInvoice` signature currently ends:

```go
	partialPayment bool,
	rememberMapping bool,
) error {
```

Add a `filenameInput` parameter:

```go
	partialPayment bool,
	rememberMapping bool,
	filenameInput string,
) error {
```

In `saveInvoice`, the filename-generation block currently is:

```go
	// Generate filename
	filename, err := core.ApplyTemplate(
		a.settings.NamingTemplate,
		meta,
		core.TemplateOpts{DecimalSeparator: a.settings.DecimalSeparator},
	)
	if err != nil {
		return fmt.Errorf("failed to generate filename: %w", err)
	}
```

Replace it with (use the supplied name, sanitised; reject empty):

```go
	// Use the filename supplied by the editable field.
	filename := core.SanitizeFilename(strings.TrimSpace(filenameInput))
	if filename == "" {
		return fmt.Errorf("Bitte einen Dateinamen eingeben.")
	}
```

- [ ] **Step 5: Pass the field text from the save button**

In `internal/ui/invoicemodal.go`, the `saveBtn.OnTapped` call to `a.saveInvoice(...)` currently ends with `rememberCheck.Checked,` as its last argument:

```go
			rememberCheck.Checked,
		)
```

Add the filename text as the final argument:

```go
			rememberCheck.Checked,
			filenameEntry.Text,
		)
```

- [ ] **Step 6: Build and vet**

Run: `go build ./... && go vet ./...`
Expected: PASS. (If `fyne` becomes an unused import because `fyne.TextWrapBreak` was the last use — check; `invoicemodal.go` uses `fyne` widely elsewhere, so it stays imported.)

---

### Task 3: „Rechnung bearbeiten" — Fenster, Vorschau, editierbarer Name, Ablagemonat

**Files:**
- Modify: `internal/ui/tableedit.go`

- [ ] **Step 1: Replace the import block**

In `internal/ui/tableedit.go`, the import block currently is:

```go
import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/bergx2/buchisy/internal/core"
)
```

Replace it with (add `time`, drop the now-unused `dialog`):

```go
import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/bergx2/buchisy/internal/core"
)
```

- [ ] **Step 2: Replace `showEditDialog` entirely**

In `internal/ui/tableedit.go`, replace the whole `showEditDialog` function (from `// showEditDialog shows a dialog to edit an existing invoice.` through its closing `}` before `// updateInvoice updates …`) with:

```go
// showEditDialog shows a resizable window to edit an existing invoice,
// with a document preview on the right.
func (a *App) showEditDialog(row core.CSVRow) {
	meta := row.ToMeta()

	sourceFolder := a.storageManager.GetMonthFolder(a.currentYear, a.currentMonth)
	originalPath := filepath.Join(sourceFolder, row.Dateiname)

	// Forward-declared so the calendar buttons can target this window.
	var editWin fyne.Window

	companyEntry := widget.NewEntry()
	companyEntry.SetText(meta.Firmenname)
	companyEntry.SetPlaceHolder(a.bundle.T("field.company"))

	shortDescEntry := widget.NewEntry()
	shortDescEntry.SetText(meta.Kurzbezeichnung)
	shortDescEntry.SetPlaceHolder(a.bundle.T("field.shortdesc"))
	shortDescLabel := widget.NewLabel(fmt.Sprintf("%d / 80", len(meta.Kurzbezeichnung)))

	invoiceNumEntry := widget.NewEntry()
	invoiceNumEntry.SetText(meta.Rechnungsnummer)
	invoiceNumEntry.SetPlaceHolder(a.bundle.T("field.invoicenumber"))

	dateEntry := widget.NewEntry()
	dateEntry.SetText(meta.Rechnungsdatum)
	dateEntry.SetPlaceHolder(a.bundle.T("field.invoiceDate"))

	dateCalendarBtn := widget.NewButton("📅", func() {
		a.showDatePicker(editWin, dateEntry.Text, func(selectedDate string) {
			dateEntry.SetText(selectedDate)
		})
	})
	dateCalendarBtn.Importance = widget.LowImportance

	netEntry := widget.NewEntry()
	netEntry.SetText(formatDecimal(meta.BetragNetto, a.settings.DecimalSeparator))
	netEntry.SetPlaceHolder(a.bundle.T("field.net"))

	vatPercentEntry := widget.NewEntry()
	vatPercentEntry.SetText(formatDecimal(meta.SteuersatzProzent, a.settings.DecimalSeparator))
	vatPercentEntry.SetPlaceHolder(a.bundle.T("field.vatPercent"))

	vatAmountEntry := widget.NewEntry()
	vatAmountEntry.SetText(formatDecimal(meta.SteuersatzBetrag, a.settings.DecimalSeparator))
	vatAmountEntry.SetPlaceHolder(a.bundle.T("field.vatAmount"))

	grossEntry := widget.NewEntry()
	grossEntry.SetText(formatDecimal(meta.Bruttobetrag, a.settings.DecimalSeparator))
	grossEntry.SetPlaceHolder(a.bundle.T("field.gross"))

	currencySelect := widget.NewSelect([]string{"EUR", "USD"}, nil)
	if meta.Waehrung != "" {
		currencySelect.SetSelected(meta.Waehrung)
	} else {
		currencySelect.SetSelected(a.settings.CurrencyDefault)
	}

	accountOptions := make([]string, 0, len(a.settings.Accounts))
	accountMap := make(map[string]int)
	for _, acc := range a.settings.Accounts {
		label := fmt.Sprintf("%d - %s", acc.Code, acc.Label)
		accountOptions = append(accountOptions, label)
		accountMap[label] = acc.Code
	}
	accountSelect := widget.NewSelect(accountOptions, nil)
	for label, code := range accountMap {
		if code == meta.Gegenkonto {
			accountSelect.SetSelected(label)
			break
		}
	}

	bankAccountOptions := make([]string, 0, len(a.settings.BankAccounts))
	for _, ba := range a.settings.BankAccounts {
		bankAccountOptions = append(bankAccountOptions, ba.Name)
	}
	bankAccountSelect := widget.NewSelect(bankAccountOptions, nil)
	if meta.Bankkonto != "" {
		bankAccountSelect.SetSelected(meta.Bankkonto)
	} else {
		bankAccountSelect.SetSelected(a.settings.DefaultBankAccount)
	}

	paymentDateEntry := widget.NewEntry()
	paymentDateEntry.SetText(meta.Bezahldatum)
	paymentDateEntry.SetPlaceHolder(a.bundle.T("field.paymentDate"))

	paymentDateCalendarBtn := widget.NewButton("📅", func() {
		a.showDatePicker(editWin, paymentDateEntry.Text, func(selectedDate string) {
			paymentDateEntry.SetText(selectedDate)
		})
	})
	paymentDateCalendarBtn.Importance = widget.LowImportance

	partialPaymentCheck := widget.NewCheck(a.bundle.T("field.partialPayment"), nil)
	partialPaymentCheck.SetChecked(meta.Teilzahlung)

	// Ablagemonat (filing month) — prefilled with the folder the invoice
	// currently lives in.
	yearSelect := widget.NewSelect(generateYearOptions(), nil)
	yearSelect.SetSelected(fmt.Sprintf("%d", a.currentYear))
	monthSelect := widget.NewSelect(generateMonthOptions(a.bundle), nil)
	monthSelect.SetSelected(fmt.Sprintf("%02d - %-12s", int(a.currentMonth),
		a.bundle.T(fmt.Sprintf("month.%02d", int(a.currentMonth)))))

	// Editable filename field.
	filenameEntry := widget.NewEntry()
	filenameEdited := false
	suppressFilenameChange := false
	updateFilenamePreview := func() {
		if filenameEdited {
			return
		}
		currentMeta := core.Meta{
			Firmenname:        companyEntry.Text,
			Kurzbezeichnung:   shortDescEntry.Text,
			Rechnungsnummer:   invoiceNumEntry.Text,
			Rechnungsdatum:    dateEntry.Text,
			BetragNetto:       parseFloat(netEntry.Text),
			SteuersatzProzent: parseFloat(vatPercentEntry.Text),
			SteuersatzBetrag:  parseFloat(vatAmountEntry.Text),
			Bruttobetrag:      parseFloat(grossEntry.Text),
			Waehrung:          currencySelect.Selected,
		}
		parts := strings.Split(dateEntry.Text, ".")
		if len(parts) == 3 {
			currentMeta.Jahr = parts[2]
			currentMeta.Monat = parts[1]
		}
		filename, err := core.ApplyTemplate(
			a.settings.NamingTemplate,
			currentMeta,
			core.TemplateOpts{DecimalSeparator: a.settings.DecimalSeparator},
		)
		suppressFilenameChange = true
		if err != nil {
			filenameEntry.SetText("Fehler: " + err.Error())
		} else {
			filenameEntry.SetText(filename)
		}
		suppressFilenameChange = false
	}
	filenameEntry.OnChanged = func(string) {
		if !suppressFilenameChange {
			filenameEdited = true
		}
	}

	shortDescEntry.OnChanged = func(s string) {
		if len(s) > 80 {
			shortDescEntry.SetText(s[:80])
		}
		shortDescLabel.SetText(fmt.Sprintf("%d / 80", len(shortDescEntry.Text)))
		updateFilenamePreview()
	}
	onAnyChange := func(string) { updateFilenamePreview() }
	companyEntry.OnChanged = onAnyChange
	invoiceNumEntry.OnChanged = onAnyChange
	dateEntry.OnChanged = onAnyChange
	netEntry.OnChanged = onAnyChange
	vatPercentEntry.OnChanged = onAnyChange
	vatAmountEntry.OnChanged = onAnyChange
	grossEntry.OnChanged = onAnyChange
	currencySelect.OnChanged = onAnyChange

	updateFilenamePreview()

	form := container.NewVBox(
		widget.NewLabel("Datei: "+row.Dateiname),
		widget.NewSeparator(),
		widget.NewForm(
			widget.NewFormItem(a.bundle.T("field.company"), companyEntry),
			widget.NewFormItem(a.bundle.T("field.shortdesc"), container.NewBorder(nil, nil, nil, shortDescLabel, shortDescEntry)),
			widget.NewFormItem(a.bundle.T("field.invoicenumber"), invoiceNumEntry),
			widget.NewFormItem(a.bundle.T("field.invoiceDate"), container.NewBorder(nil, nil, nil, dateCalendarBtn, dateEntry)),
			widget.NewFormItem(a.bundle.T("field.paymentDate"), container.NewBorder(nil, nil, nil, paymentDateCalendarBtn, paymentDateEntry)),
			widget.NewFormItem(a.bundle.T("field.net"), netEntry),
			widget.NewFormItem(a.bundle.T("field.vatPercent"), vatPercentEntry),
			widget.NewFormItem(a.bundle.T("field.vatAmount"), vatAmountEntry),
			widget.NewFormItem(a.bundle.T("field.gross"), grossEntry),
			widget.NewFormItem(a.bundle.T("field.currency"), currencySelect),
			widget.NewFormItem(a.bundle.T("field.account"), accountSelect),
			widget.NewFormItem(a.bundle.T("field.bankAccount"), bankAccountSelect),
			widget.NewFormItem("Ablage (Jahr/Monat)", container.NewGridWithColumns(2, yearSelect, monthSelect)),
			widget.NewFormItem("", partialPaymentCheck),
		),
		widget.NewSeparator(),
		widget.NewLabel(a.bundle.T("modal.filenamePreview")),
		filenameEntry,
	)

	scrollForm := container.NewVScroll(form)
	scrollForm.SetMinSize(fyne.NewSize(420, 400))

	preview := buildDocumentPreview(originalPath, meta)
	split := container.NewHSplit(scrollForm, preview)
	splitOffset := a.settings.PreviewSplitOffset
	if splitOffset <= 0 || splitOffset >= 1 {
		splitOffset = 0.33
	}
	split.SetOffset(splitOffset)

	editWin = a.app.NewWindow("Rechnung bearbeiten")

	cancelBtn := widget.NewButton(a.bundle.T("btn.cancel"), func() {
		editWin.Close()
	})
	saveBtn := widget.NewButton(a.bundle.T("btn.save"), nil)
	saveBtn.Importance = widget.HighImportance
	saveBtn.OnTapped = func() {
		targetYear := a.currentYear
		fmt.Sscanf(yearSelect.Selected, "%d", &targetYear)
		targetMonth := a.currentMonth
		if len(monthSelect.Selected) >= 2 {
			var m int
			fmt.Sscanf(monthSelect.Selected[:2], "%d", &m)
			if m >= 1 && m <= 12 {
				targetMonth = time.Month(m)
			}
		}
		err := a.updateInvoice(
			row,
			originalPath,
			companyEntry.Text,
			shortDescEntry.Text,
			invoiceNumEntry.Text,
			dateEntry.Text,
			paymentDateEntry.Text,
			parseFloat(netEntry.Text),
			parseFloat(vatPercentEntry.Text),
			parseFloat(vatAmountEntry.Text),
			parseFloat(grossEntry.Text),
			currencySelect.Selected,
			accountMap[accountSelect.Selected],
			bankAccountSelect.Selected,
			partialPaymentCheck.Checked,
			filenameEntry.Text,
			targetYear,
			targetMonth,
		)
		if err != nil {
			dialog.ShowInformation(a.bundle.T("error.processing.title"), err.Error(), editWin)
			return
		}
		a.loadInvoices()
		editWin.Close()
	}

	buttonBar := container.NewBorder(nil, nil, nil,
		container.NewHBox(cancelBtn, saveBtn),
	)

	editWin.SetOnClosed(func() {
		a.settings.PreviewSplitOffset = split.Offset
		if err := a.settingsMgr.Save(a.settings); err != nil {
			a.logger.Warn("Failed to save preview split offset: %v", err)
		}
	})

	editWin.SetContent(container.NewBorder(nil, buttonBar, nil, nil, split))
	editWin.Resize(fyne.NewSize(1500, 850))
	editWin.CenterOnScreen()
	editWin.Show()
}
```

Note: the save button uses `dialog.ShowInformation`, so re-add `"fyne.io/fyne/v2/dialog"` to the import block from Step 1 — i.e. the import block keeps `dialog` after all. Use this import block instead:

```go
import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/bergx2/buchisy/internal/core"
)
```

- [ ] **Step 3: Replace `updateInvoice` entirely**

In `internal/ui/tableedit.go`, replace the whole `updateInvoice` function with:

```go
// updateInvoice updates an existing invoice: it renames/moves the main file
// (possibly into another month's folder) and updates the CSV(s).
func (a *App) updateInvoice(
	originalRow core.CSVRow,
	originalPath string,
	company string,
	shortDesc string,
	invoiceNum string,
	invoiceDate string,
	paymentDate string,
	net float64,
	vatPercent float64,
	vatAmount float64,
	gross float64,
	currency string,
	account int,
	bankAccount string,
	partialPayment bool,
	filenameInput string,
	targetYear int,
	targetMonth time.Month,
) error {
	newMeta := core.Meta{
		Firmenname:        company,
		Kurzbezeichnung:   shortDesc,
		Rechnungsnummer:   invoiceNum,
		Rechnungsdatum:    invoiceDate,
		Bezahldatum:       paymentDate,
		BetragNetto:       net,
		SteuersatzProzent: vatPercent,
		SteuersatzBetrag:  vatAmount,
		Bruttobetrag:      gross,
		Waehrung:          currency,
		Gegenkonto:        account,
		Bankkonto:         bankAccount,
		Teilzahlung:       partialPayment,
	}
	parts := strings.Split(invoiceDate, ".")
	if len(parts) == 3 {
		newMeta.Jahr = parts[2]
		newMeta.Monat = parts[1]
	}

	// Filename from the editable field.
	newFilename := core.SanitizeFilename(strings.TrimSpace(filenameInput))
	if newFilename == "" {
		return fmt.Errorf("Bitte einen Dateinamen eingeben.")
	}
	if mainExt := strings.ToLower(filepath.Ext(originalPath)); mainExt != "" {
		newFilename = core.ReplaceExtension(newFilename, mainExt)
	}

	// Target folder (may differ from the source month).
	targetFolder := a.storageManager.GetMonthFolder(targetYear, targetMonth)
	if err := os.MkdirAll(targetFolder, 0755); err != nil {
		return fmt.Errorf("failed to create target folder: %w", err)
	}
	sameMonth := targetYear == a.currentYear && targetMonth == a.currentMonth

	// Move/rename the main file if its path changes.
	finalName := newFilename
	finalPath := filepath.Join(targetFolder, finalName)
	if finalPath != originalPath {
		counter := 2
		for {
			if _, err := os.Stat(finalPath); os.IsNotExist(err) {
				break
			}
			ext := filepath.Ext(newFilename)
			base := strings.TrimSuffix(newFilename, ext)
			finalName = fmt.Sprintf("%s_%d%s", base, counter, ext)
			finalPath = filepath.Join(targetFolder, finalName)
			counter++
		}
		if err := os.Rename(originalPath, finalPath); err != nil {
			return fmt.Errorf("failed to move file: %w", err)
		}
	}

	newRow := newMeta.ToCSVRow()
	newRow.Dateiname = finalName
	newRow.HatAnhaenge = originalRow.HatAnhaenge
	newRow.AnzahlAnhaenge = originalRow.AnzahlAnhaenge

	sourceCSV := a.storageManager.GetCSVPath(a.currentYear, a.currentMonth)

	if sameMonth {
		// Update the row in place in the single CSV.
		rows, err := a.csvRepo.Load(sourceCSV)
		if err != nil {
			return fmt.Errorf("failed to load CSV: %w", err)
		}
		found := false
		for i := range rows {
			if rows[i].Dateiname == originalRow.Dateiname {
				rows[i] = newRow
				found = true
			}
		}
		if !found {
			return fmt.Errorf("original row not found in CSV")
		}
		if err := a.rewriteCSV(sourceCSV, rows); err != nil {
			return fmt.Errorf("failed to update CSV: %w", err)
		}
	} else {
		// Remove from the source CSV, add to the target CSV.
		srcRows, err := a.csvRepo.Load(sourceCSV)
		if err != nil {
			return fmt.Errorf("failed to load source CSV: %w", err)
		}
		kept := make([]core.CSVRow, 0, len(srcRows))
		for _, r := range srcRows {
			if r.Dateiname != originalRow.Dateiname {
				kept = append(kept, r)
			}
		}
		if err := a.rewriteCSV(sourceCSV, kept); err != nil {
			return fmt.Errorf("failed to update source CSV: %w", err)
		}
		targetCSV := a.storageManager.GetCSVPath(targetYear, targetMonth)
		tgtRows, err := a.csvRepo.Load(targetCSV)
		if err != nil {
			return fmt.Errorf("failed to load target CSV: %w", err)
		}
		tgtRows = append(tgtRows, newRow)
		if err := a.rewriteCSV(targetCSV, tgtRows); err != nil {
			return fmt.Errorf("failed to update target CSV: %w", err)
		}
	}

	a.logger.Info("Updated invoice: %s", finalName)
	a.showInfo("Gespeichert", fmt.Sprintf("Rechnung wurde aktualisiert: %s", finalName))
	return nil
}
```

- [ ] **Step 4: Build, vet, test**

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

Terminate any running `BuchISY` process (PowerShell `wmic process where "ProcessId=<id>" call terminate` per running PID).

- [ ] **Step 4: Deploy and restart**

Copy `cmd/buchisy/BuchISY.exe` over `C:\Users\istok\Desktop\BuchISY.exe`, then launch
`C:\Users\istok\Desktop\BuchISY.exe` with working directory `C:\Users\istok\Desktop`.

- [ ] **Step 5: Manual smoke test**

1. Eine Rechnung in der Tabelle per ✏️ bearbeiten → das Fenster ist in der
   Größe änderbar, rechts erscheint die Belegvorschau; Beträge mit Komma.
2. Das Dateiname-Feld ist editierbar; ohne manuelle Änderung folgt es den
   Feldänderungen, nach manueller Änderung bleibt es stehen.
3. Speichern mit geändertem Namen → die Datei trägt den neuen Namen;
   ein Betrag wie `15,23` bleibt im Namen erhalten.
4. „Ablage (Jahr/Monat)" auf einen anderen Monat setzen → nach dem
   Speichern liegen Datei und CSV-Eintrag im Zielmonat; im Quellmonat ist
   der Eintrag weg.
5. In „Rechnungsdaten prüfen" (neue PDF) ist das Dateiname-Feld ebenfalls
   editierbar und wird beim Speichern verwendet.
6. Dateiname-Feld leeren → Speichern zeigt eine Fehlermeldung, Dialog
   bleibt offen.

---

## Self-Review

**Spec coverage:**
- Dezimal-Fix (`showEditDialog` Beträge mit Trennzeichen) → Task 3 Step 2 (`formatDecimal`).
- Editierbares Dateiname-Feld, „Rechnungsdaten prüfen" → Task 2; „Rechnung bearbeiten" → Task 3 Step 2.
- `filenameEdited`/`suppressFilenameChange`-Logik → Task 2 Steps 1-2, Task 3 Step 2.
- Verwendung beim Speichern (`SanitizeFilename` + `ReplaceExtension`, leerer Name → Fehler) → `saveInvoice` Task 2 Step 4; `updateInvoice` Task 3 Step 3.
- Komma im Dateinamen erhalten → Task 1.
- Ablagemonat: Jahr/Monat-Auswahlzeile + Verschiebe-Logik (Ordner anlegen, Kollision gegen Zielordner, CSV in-place bzw. quell-/ziel-CSV) → Task 3 Steps 2-3.
- „Rechnung bearbeiten" als größenänderbares Fenster mit `buildDocumentPreview`, Split-Offset-Persistenz, Kalender-Buttons über dem neuen Fenster → Task 3 Step 2.

**Placeholder scan:** Keine TBD/TODO; alle Code-Schritte enthalten vollständigen Code.

**Type consistency:** `saveInvoice` erhält `filenameInput string` als letzten Parameter (Task 2 Step 4); der Aufruf (Step 5) übergibt `filenameEntry.Text` an dieser Position. `updateInvoice` erhält `filenameInput string, targetYear int, targetMonth time.Month` als letzte drei Parameter (Task 3 Step 3); der Aufruf in `showEditDialog` (Step 2) übergibt `filenameEntry.Text, targetYear, targetMonth` in dieser Reihenfolge. `SanitizeFilename`, `ReplaceExtension`, `ApplyTemplate`, `buildDocumentPreview`, `formatDecimal`, `generateYearOptions`, `generateMonthOptions`, `rewriteCSV`, `csvRepo.Load`, `storageManager.GetMonthFolder`/`GetCSVPath` sind bestehende Symbole mit unveränderten Signaturen. Das Import-Block-Endergebnis in Task 3 (mit `time` und `dialog`) ist in Step 2 explizit festgehalten.
