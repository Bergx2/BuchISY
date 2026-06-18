# Multi-File-Auswahl & Anhänge — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Im Datei-Picker mehrere Dateien auswählbar machen, eine als Hauptdatei (Rechnung) markieren, die übrigen als Anhänge neben der Rechnung ablegen; zusätzlich Office-, LibreOffice- und Bilddateien akzeptieren.

**Architecture:** Reine Domänen-Helfer (Dateitypen, Namensbildung) kommen nach `internal/core`. Der Picker (`custompicker.go`) erhält Checkboxen + eine Auswahl-Ablage mit Hauptdatei-Markierung. Ein neuer Einstieg `processSubmission` ersetzt `processPDFAsync` und reicht Hauptdatei + Anhänge bis in `saveInvoice` durch, das die Anhänge als `<Rechnung>_AnhangN<ext>` ablegt. Zwei neue CSV-Spalten halten den Anhang-Status fest.

**Tech Stack:** Go 1.25, Fyne v2.6.3, Standard-`testing`.

**Hinweis zu Commits:** Git-Commits sind in diesem Plan bewusst ausgelassen — die Auslieferung erfolgt wie in der bisherigen Session per Build + Kopie der `.exe`. Jede Aufgabe endet stattdessen mit `go build` / `go vet` als Verifikation.

---

### Task 1: Core — Datei-Typ- und Namens-Helfer

**Files:**
- Create: `internal/core/files.go`
- Test: `internal/core/files_test.go`

- [ ] **Step 1: Write the failing test**

`internal/core/files_test.go`:

```go
package core

import "testing"

func TestIsSupportedFile(t *testing.T) {
	cases := map[string]bool{
		"rechnung.pdf": true,
		"Rechnung.PDF": true,
		"tabelle.xlsx": true,
		"alt.xls":      true,
		"brief.docx":   true,
		"brief.doc":    true,
		"folien.pptx":  true,
		"calc.ods":     true,
		"text.odt":     true,
		"foto.JPG":     true,
		"bild.png":     true,
		"scan.tiff":    true,
		"archiv.zip":   false,
		"daten.csv":    false,
		"noext":        false,
	}
	for name, want := range cases {
		if got := IsSupportedFile(name); got != want {
			t.Errorf("IsSupportedFile(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestIsPDF(t *testing.T) {
	if !IsPDF("a.pdf") {
		t.Error("a.pdf should be PDF")
	}
	if !IsPDF("A.PDF") {
		t.Error("A.PDF should be PDF")
	}
	if IsPDF("a.xlsx") {
		t.Error("a.xlsx should not be PDF")
	}
}

func TestReplaceExtension(t *testing.T) {
	if got := ReplaceExtension("2025-08_AWS_EUR.pdf", ".xlsx"); got != "2025-08_AWS_EUR.xlsx" {
		t.Errorf("got %q", got)
	}
	if got := ReplaceExtension("noext", ".pdf"); got != "noext.pdf" {
		t.Errorf("got %q", got)
	}
	if got := ReplaceExtension("file.pdf", ""); got != "file" {
		t.Errorf("got %q", got)
	}
}

func TestAttachmentName(t *testing.T) {
	if got := AttachmentName("2025-08-01_AWS_EUR.pdf", 1, ".xlsx"); got != "2025-08-01_AWS_EUR_Anhang1.xlsx" {
		t.Errorf("got %q", got)
	}
	if got := AttachmentName("inv.pdf", 2, ".pdf"); got != "inv_Anhang2.pdf" {
		t.Errorf("got %q", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run "TestIsSupportedFile|TestIsPDF|TestReplaceExtension|TestAttachmentName" -v`
Expected: FAIL — `undefined: IsSupportedFile` (and the other functions).

- [ ] **Step 3: Write the implementation**

`internal/core/files.go`:

```go
package core

import (
	"fmt"
	"path/filepath"
	"strings"
)

// supportedExtensions is the set of file extensions BuchISY accepts as an
// invoice main file or as an attachment (lower-case, leading dot).
var supportedExtensions = map[string]struct{}{
	".pdf":  {},
	".doc":  {}, ".docx": {},
	".xls":  {}, ".xlsx": {},
	".ppt":  {}, ".pptx": {},
	".odt":  {}, ".ods": {}, ".odp": {},
	".jpg":  {}, ".jpeg": {}, ".png": {}, ".gif": {},
	".bmp":  {}, ".tif": {}, ".tiff": {}, ".webp": {}, ".heic": {}, ".svg": {},
}

// IsSupportedFile reports whether the file name has an extension BuchISY
// accepts as an invoice main file or attachment.
func IsSupportedFile(name string) bool {
	_, ok := supportedExtensions[strings.ToLower(filepath.Ext(name))]
	return ok
}

// IsPDF reports whether the file name is a PDF.
func IsPDF(name string) bool {
	return strings.ToLower(filepath.Ext(name)) == ".pdf"
}

// ReplaceExtension returns name with its extension replaced by newExt.
// newExt must include the leading dot (or be empty to strip the extension).
func ReplaceExtension(name, newExt string) string {
	ext := filepath.Ext(name)
	return name[:len(name)-len(ext)] + newExt
}

// AttachmentName builds the filed name for the index-th attachment (1-based)
// of an invoice whose final file name is mainName. attachmentExt is the
// attachment's own extension, including the leading dot.
func AttachmentName(mainName string, index int, attachmentExt string) string {
	return fmt.Sprintf("%s_Anhang%d%s", ReplaceExtension(mainName, ""), index, attachmentExt)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run "TestIsSupportedFile|TestIsPDF|TestReplaceExtension|TestAttachmentName" -v`
Expected: PASS — all four tests ok.

---

### Task 2: Core — CSV-Spalten `HatAnhaenge` und `AnzahlAnhaenge`

**Files:**
- Modify: `internal/core/types.go` (struct `CSVRow`, ~Zeile 117-134)
- Modify: `internal/core/csvrepo.go` (`DefaultCSVColumns`, `ColumnDisplayNames`, `ColumnTranslationKeys`, `Load`, `rowToRecord`)
- Test: `internal/core/csvrepo_test.go`

- [ ] **Step 1: Write the failing test**

`internal/core/csvrepo_test.go`:

```go
package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCSVRoundTripWithAttachments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invoices.csv")
	repo := NewCSVRepository()

	row := CSVRow{
		Dateiname:      "2025-08-01_AWS_EUR.pdf",
		Firmenname:     "AWS",
		Bruttobetrag:   37.64,
		Waehrung:       "EUR",
		HatAnhaenge:    true,
		AnzahlAnhaenge: 2,
	}
	if err := repo.Append(path, row); err != nil {
		t.Fatalf("append: %v", err)
	}

	rows, err := repo.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if !rows[0].HatAnhaenge {
		t.Error("HatAnhaenge should be true")
	}
	if rows[0].AnzahlAnhaenge != 2 {
		t.Errorf("AnzahlAnhaenge = %d, want 2", rows[0].AnzahlAnhaenge)
	}
}

func TestCSVLoadLegacyWithoutAttachmentColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invoices.csv")
	legacy := "Dateiname,Firmenname\nalt.pdf,Telekom\n"
	if err := os.WriteFile(path, []byte(legacy), 0644); err != nil {
		t.Fatal(err)
	}
	rows, err := NewCSVRepository().Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows", len(rows))
	}
	if rows[0].HatAnhaenge || rows[0].AnzahlAnhaenge != 0 {
		t.Error("legacy row should default attachments to false/0")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run "TestCSV" -v`
Expected: FAIL — `unknown field HatAnhaenge in struct literal of type CSVRow`.

- [ ] **Step 3: Add fields to `CSVRow`**

In `internal/core/types.go`, the `CSVRow` struct currently ends:

```go
	Bezahldatum       string
	Teilzahlung       bool
}
```

Replace with:

```go
	Bezahldatum       string
	Teilzahlung       bool
	HatAnhaenge       bool
	AnzahlAnhaenge    int
}
```

- [ ] **Step 4: Register the columns in `csvrepo.go`**

In `internal/core/csvrepo.go`, `DefaultCSVColumns` currently ends with `"Teilzahlung",`. Change the tail:

```go
	"Bezahldatum",
	"Teilzahlung",
	"HatAnhaenge",
	"AnzahlAnhaenge",
}
```

In `ColumnDisplayNames`, after `"Teilzahlung": "Teilzahlung",` add:

```go
	"HatAnhaenge":        "Hat Anhänge",
	"AnzahlAnhaenge":     "Anzahl Anhänge",
```

In `ColumnTranslationKeys`, after `"Teilzahlung": "table.col.partialpayment",` add:

```go
	"HatAnhaenge":        "table.col.hasattachments",
	"AnzahlAnhaenge":     "table.col.attachmentcount",
```

- [ ] **Step 5: Read/write the new columns**

In `csvrepo.go` `Load`, the `row := CSVRow{...}` literal ends with
`Teilzahlung: parseBool(valueForColumn(record, headerMap, "Teilzahlung")),`.
Add two lines before the closing `}`:

```go
			Teilzahlung:       parseBool(valueForColumn(record, headerMap, "Teilzahlung")),
			HatAnhaenge:       parseBool(valueForColumn(record, headerMap, "HatAnhaenge")),
			AnzahlAnhaenge:    parseInt(valueForColumn(record, headerMap, "AnzahlAnhaenge")),
		}
```

In `csvrepo.go` `rowToRecord`, the `valueMap` ends with
`"Teilzahlung": formatBool(row.Teilzahlung),`. Add two entries:

```go
		"Teilzahlung":        formatBool(row.Teilzahlung),
		"HatAnhaenge":        formatBool(row.HatAnhaenge),
		"AnzahlAnhaenge":     strconv.Itoa(row.AnzahlAnhaenge),
	}
```

(`strconv` is already imported in `csvrepo.go`.)

- [ ] **Step 6: Run test to verify it passes**

Run: `go test ./internal/core/ -run "TestCSV" -v`
Expected: PASS — both tests ok.

Note: `validColumns` is derived from `DefaultCSVColumns` at package init, so the new columns are automatically recognized by `parseHeader`. Legacy CSVs lacking the columns resolve to header index `-1`, and `valueForColumn` returns `""` → `false`/`0`.

---

### Task 3: i18n — Spaltenüberschriften

**Files:**
- Modify: `assets/i18n/de.json`
- Modify: `assets/i18n/en.json`

- [ ] **Step 1: Add German keys**

In `assets/i18n/de.json`, the line `"table.col.filename": "Dateiname",` exists. Add directly after it:

```json
  "table.col.hasattachments": "Hat Anhänge",
  "table.col.attachmentcount": "Anzahl Anhänge",
```

- [ ] **Step 2: Add English keys**

In `assets/i18n/en.json`, find `"table.col.filename": "Filename",` and add directly after it:

```json
  "table.col.hasattachments": "Has Attachments",
  "table.col.attachmentcount": "Attachment Count",
```

- [ ] **Step 3: Verify JSON validity**

Run: `go run -tags ignore ./... 2>NUL & node -e "require('./assets/i18n/de.json');require('./assets/i18n/en.json');console.log('json ok')"`
If `node` is unavailable, instead verify by building in Task 9 (the i18n loader will fail at runtime on broken JSON). Minimum check: ensure no trailing comma was introduced and both new lines sit inside the top-level object.

---

### Task 4: Picker — Dateityp-Filter erweitern

**Files:**
- Modify: `internal/ui/custompicker.go`

- [ ] **Step 1: Replace the PDF-only filter**

In `internal/ui/custompicker.go`, inside `loadFiles`, the filter loop reads:

```go
		// Filter: only directories and PDF files
		allFiles = []os.DirEntry{}
		for _, entry := range entries {
			if entry.IsDir() || strings.HasSuffix(strings.ToLower(entry.Name()), ".pdf") {
				allFiles = append(allFiles, entry)
			}
		}
```

Replace with:

```go
		// Filter: directories and all supported file types
		allFiles = []os.DirEntry{}
		for _, entry := range entries {
			if entry.IsDir() || core.IsSupportedFile(entry.Name()) {
				allFiles = append(allFiles, entry)
			}
		}
```

- [ ] **Step 2: Verify `strings` is still used**

`strings` is still used by `applyFilter` (`strings.ToLower`, `strings.Contains`, `strings.TrimSpace`). No import change needed. `core` is already imported.

- [ ] **Step 3: Build**

Run: `go build ./internal/ui/`
Expected: builds without error.

---

### Task 5: Picker — Mehrfachauswahl mit Checkboxen + Auswahl-Ablage

**Files:**
- Modify: `internal/ui/custompicker.go` (function `showCustomFilePicker`)

This task rewrites `showCustomFilePicker`. The helpers `getStartingFolder` and `splitPathSegments` at the bottom of the file stay unchanged.

- [ ] **Step 1: Replace the body of `showCustomFilePicker`**

Replace the entire function `showCustomFilePicker` (from `func (a *App) showCustomFilePicker() {` up to and including its closing `}`) with:

```go
// showCustomFilePicker shows a custom multi-select file picker with search.
func (a *App) showCustomFilePicker() {
	startFolder := a.getStartingFolder()
	currentPath := startFolder
	allFiles := []os.DirEntry{}
	filteredFiles := []os.DirEntry{}

	// Persistent selection across folder navigation.
	selected := []string{} // absolute paths, in selection order
	mainPath := ""          // which selected path is the invoice main file

	isSelected := func(path string) bool {
		for _, s := range selected {
			if s == path {
				return true
			}
		}
		return false
	}

	// Clickable breadcrumb path bar. loadFiles is forward-declared so the
	// breadcrumb segment buttons can navigate.
	var loadFiles func(path string)
	breadcrumb := container.NewHBox()
	breadcrumbScroll := container.NewHScroll(breadcrumb)

	updateBreadcrumb := func(path string) {
		breadcrumb.RemoveAll()
		segments := splitPathSegments(path)
		cumulative := ""
		for i, seg := range segments {
			if i == 0 {
				cumulative = seg
			} else {
				cumulative = filepath.Join(cumulative, seg)
			}
			if i == len(segments)-1 {
				breadcrumb.Add(widget.NewLabelWithStyle(
					seg, fyne.TextAlignLeading, fyne.TextStyle{Bold: true},
				))
				continue
			}
			target := cumulative
			segBtn := widget.NewButton(seg, func() { loadFiles(target) })
			segBtn.Importance = widget.LowImportance
			breadcrumb.Add(segBtn)
			breadcrumb.Add(widget.NewLabel("›"))
		}
		breadcrumb.Refresh()
		breadcrumbScroll.ScrollToOffset(fyne.NewPos(breadcrumb.MinSize().Width, 0))
	}

	searchEntry := widget.NewEntry()
	searchEntry.SetPlaceHolder("Dateiname suchen (z.B. rechnung, 2025-10, GmbH)...")

	// Selection tray (rebuilt on every change).
	selectionList := container.NewVBox()
	var fileList *widget.List
	var refreshSelection func()
	refreshSelection = func() {
		selectionList.RemoveAll()
		if len(selected) == 0 {
			selectionList.Add(widget.NewLabel("Noch keine Dateien ausgewählt."))
		}
		for _, p := range selected {
			path := p
			isMain := path == mainPath

			star := "☆"
			if isMain {
				star = "★"
			}
			starBtn := widget.NewButton(star, func() {
				mainPath = path
				refreshSelection()
			})
			if isMain {
				starBtn.Importance = widget.HighImportance
			} else {
				starBtn.Importance = widget.LowImportance
			}

			removeBtn := widget.NewButton("Entfernen", func() {
				next := make([]string, 0, len(selected))
				for _, s := range selected {
					if s != path {
						next = append(next, s)
					}
				}
				selected = next
				if mainPath == path {
					mainPath = ""
					if len(selected) > 0 {
						mainPath = selected[0]
					}
				}
				refreshSelection()
				if fileList != nil {
					fileList.Refresh()
				}
			})
			removeBtn.Importance = widget.LowImportance

			nameLabel := widget.NewLabel(filepath.Base(path))
			nameLabel.Truncation = fyne.TextTruncateEllipsis
			selectionList.Add(container.NewBorder(nil, nil, starBtn, removeBtn, nameLabel))
		}
		selectionList.Refresh()
	}

	toggleSelected := func(path string, checked bool) {
		if checked {
			if !isSelected(path) {
				selected = append(selected, path)
				if mainPath == "" {
					mainPath = path
				}
			}
		} else {
			next := make([]string, 0, len(selected))
			for _, s := range selected {
				if s != path {
					next = append(next, s)
				}
			}
			selected = next
			if mainPath == path {
				mainPath = ""
				if len(selected) > 0 {
					mainPath = selected[0]
				}
			}
		}
		refreshSelection()
	}

	fileList = widget.NewList(
		func() int { return len(filteredFiles) },
		func() fyne.CanvasObject {
			return container.NewHBox(
				widget.NewCheck("", nil),
				widget.NewIcon(nil),
				widget.NewLabel("template"),
			)
		},
		func(id widget.ListItemID, item fyne.CanvasObject) {
			if id >= len(filteredFiles) {
				return
			}
			entry := filteredFiles[id]
			box := item.(*fyne.Container)
			check := box.Objects[0].(*widget.Check)
			icon := box.Objects[1].(*widget.Icon)
			label := box.Objects[2].(*widget.Label)
			fullPath := filepath.Join(currentPath, entry.Name())

			if entry.IsDir() {
				check.OnChanged = nil
				check.SetChecked(false)
				check.Hide()
				icon.SetResource(theme.FolderIcon())
				label.SetText("📁 " + entry.Name())
				return
			}

			check.Show()
			check.OnChanged = nil // avoid SetChecked firing the handler
			check.SetChecked(isSelected(fullPath))
			check.OnChanged = func(checked bool) {
				toggleSelected(fullPath, checked)
			}
			icon.SetResource(theme.FileIcon())
			label.SetText("📄 " + entry.Name())
		},
	)

	loadFiles = func(path string) {
		entries, err := os.ReadDir(path)
		if err != nil {
			a.logger.Error("Failed to read directory: %v", err)
			return
		}
		allFiles = []os.DirEntry{}
		for _, entry := range entries {
			if entry.IsDir() || core.IsSupportedFile(entry.Name()) {
				allFiles = append(allFiles, entry)
			}
		}
		filteredFiles = allFiles
		currentPath = path
		updateBreadcrumb(currentPath)
		searchEntry.SetText("")
		fileList.Refresh()
	}

	applyFilter := func(query string) {
		query = strings.ToLower(strings.TrimSpace(query))
		if query == "" {
			filteredFiles = allFiles
		} else {
			filteredFiles = []os.DirEntry{}
			for _, entry := range allFiles {
				if strings.Contains(strings.ToLower(entry.Name()), query) {
					filteredFiles = append(filteredFiles, entry)
				}
			}
		}
		fileList.Refresh()
	}
	searchEntry.OnChanged = func(query string) { applyFilter(query) }

	upBtn := widget.NewButton("⬆️ Übergeordneter Ordner", func() {
		parent := filepath.Dir(currentPath)
		if parent != currentPath {
			loadFiles(parent)
		}
	})
	homeBtn := widget.NewButton("🏠 Dokumente", func() {
		if docsDir, err := core.GetDocumentsDir(); err == nil {
			loadFiles(docsDir)
		}
	})

	fileList.OnSelected = func(id widget.ListItemID) {
		if id < len(filteredFiles) && filteredFiles[id].IsDir() {
			loadFiles(filepath.Join(currentPath, filteredFiles[id].Name()))
		}
		fileList.UnselectAll()
	}

	loadFiles(startFolder)
	refreshSelection()

	selectionScroll := container.NewVScroll(selectionList)
	selectionScroll.SetMinSize(fyne.NewSize(0, 150))

	content := container.NewBorder(
		container.NewVBox(
			container.NewHBox(upBtn, homeBtn),
			breadcrumbScroll,
			searchEntry,
			widget.NewSeparator(),
		),
		container.NewVBox(
			widget.NewSeparator(),
			widget.NewLabelWithStyle(
				"Auswahl (★ = Hauptdatei)", fyne.TextAlignLeading, fyne.TextStyle{Bold: true},
			),
			selectionScroll,
		),
		nil, nil,
		fileList,
	)

	customDialog := dialog.NewCustomConfirm(
		"Dateien auswählen",
		"Öffnen",
		"Abbrechen",
		content,
		func(open bool) {
			if !open {
				return
			}
			if len(selected) == 0 {
				a.showError("Fehler", "Bitte mindestens eine Datei auswählen.")
				return
			}
			if mainPath == "" {
				a.showError("Fehler", "Bitte eine Hauptdatei markieren (★).")
				return
			}
			attachments := make([]string, 0, len(selected))
			for _, s := range selected {
				if s != mainPath {
					attachments = append(attachments, s)
				}
			}
			a.settings.LastUsedFolder = currentPath
			if err := a.settingsMgr.Save(a.settings); err != nil {
				a.logger.Warn("Failed to save last used folder: %v", err)
			}
			a.processSubmission(mainPath, attachments)
		},
		a.window,
	)

	customDialog.Resize(fyne.NewSize(1000, 760))
	customDialog.Show()
}
```

- [ ] **Step 2: Build**

Run: `go build ./internal/ui/`
Expected: FAIL — `a.processSubmission undefined`. This is expected; Task 6 defines it. Proceed to Task 6 before re-verifying.

---

### Task 6: Verarbeitung — `processSubmission` und `handleNoTextPDF`

**Files:**
- Modify: `internal/ui/filepicker.go` (rename `processPDFAsync` → `processSubmission`)
- Modify: `internal/ui/app.go` (`handleNoTextPDF` signature)

- [ ] **Step 1: Replace `processPDFAsync` with `processSubmission`**

In `internal/ui/filepicker.go`, replace the whole function (from `// processPDFAsync processes a PDF file in the background.` through its closing `}`) with:

```go
// processSubmission processes a selected main file plus its attachments.
// A PDF main file is run through metadata extraction; a non-PDF main file
// skips extraction and opens the confirmation modal for manual entry.
func (a *App) processSubmission(mainPath string, attachments []string) {
	a.logger.Info("Processing submission: main=%s, attachments=%d", mainPath, len(attachments))

	if !core.IsPDF(mainPath) {
		emptyMeta := core.Meta{
			Waehrung:   a.settings.CurrencyDefault,
			Gegenkonto: a.settings.DefaultAccount,
		}
		a.showConfirmationModal(mainPath, attachments, emptyMeta)
		return
	}

	progress := dialog.NewProgressInfinite(
		a.bundle.T("processing.title"),
		a.bundle.T("processing.message"),
		a.window,
	)
	progress.Show()

	go func() {
		ctx := context.Background()
		meta, err := a.extractPDFData(ctx, mainPath)
		progress.Hide()
		time.Sleep(150 * time.Millisecond)

		if err != nil {
			a.logger.Error("Failed to process PDF: %v", err)
			if err.Error() == "no text found in PDF" {
				a.handleNoTextPDF(mainPath, attachments)
			} else {
				a.showError(
					a.bundle.T("error.processing.title"),
					a.bundle.T("error.processing.message", err.Error()),
				)
			}
			return
		}

		a.logger.Info("Showing confirmation modal for: %s", filepath.Base(mainPath))
		a.showConfirmationModal(mainPath, attachments, meta)
	}()
}
```

(Imports `context`, `path/filepath`, `time`, `dialog`, `core` are already present in `filepicker.go`.)

- [ ] **Step 2: Update `handleNoTextPDF` in `app.go`**

In `internal/ui/app.go`, replace the `handleNoTextPDF` function with:

```go
// handleNoTextPDF handles PDFs without extractable text (e.g., scanned images).
// Must be called from UI thread.
func (a *App) handleNoTextPDF(path string, attachments []string) {
	dialog.ShowConfirm(
		a.bundle.T("error.noText"),
		a.bundle.T("error.manualEntry"),
		func(manual bool) {
			if manual {
				emptyMeta := core.Meta{
					Waehrung:   a.settings.CurrencyDefault,
					Gegenkonto: a.settings.DefaultAccount,
				}
				a.showConfirmationModal(path, attachments, emptyMeta)
			}
		},
		a.window,
	)
}
```

- [ ] **Step 3: Build**

Run: `go build ./internal/ui/`
Expected: FAIL — `a.showConfirmationModal` called with 3 args but defined with 2. Expected; Task 7 fixes the signature. Proceed.

---

### Task 7: Modal — Anhänge durchreichen und anzeigen

**Files:**
- Modify: `internal/ui/invoicemodal.go` (`showConfirmationModal` signature + form + save call)

- [ ] **Step 1: Change the signature**

In `internal/ui/invoicemodal.go`, change:

```go
func (a *App) showConfirmationModal(originalPath string, meta core.Meta) {
```

to:

```go
func (a *App) showConfirmationModal(originalPath string, attachments []string, meta core.Meta) {
```

- [ ] **Step 2: Show the attachments in the form**

In `showConfirmationModal`, the form is built as `form := container.NewVBox( ... )` starting with
`widget.NewLabel(a.bundle.T("modal.originalFile")),`. Replace the whole
`form := container.NewVBox( ... )` statement with:

```go
		// Form layout
		formItems := []fyne.CanvasObject{
			widget.NewLabel(a.bundle.T("modal.originalFile")),
			container.NewBorder(nil, nil, nil, openOriginalBtn, originalEntry),
		}
		if len(attachments) > 0 {
			names := make([]string, len(attachments))
			for i, p := range attachments {
				names[i] = filepath.Base(p)
			}
			attLabel := widget.NewLabel(fmt.Sprintf(
				"Anhänge (%d): %s", len(attachments), strings.Join(names, ", "),
			))
			attLabel.Wrapping = fyne.TextWrapWord
			formItems = append(formItems, attLabel)
		}
		formItems = append(formItems,
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
				widget.NewFormItem("", partialPaymentCheck),
			),
			rememberCheck,
			widget.NewSeparator(),
			widget.NewLabel(a.bundle.T("modal.filenamePreview")),
			filenamePreview,
		)
		form := container.NewVBox(formItems...)
```

(`fmt`, `strings`, `filepath` are already imported in `invoicemodal.go`.)

- [ ] **Step 3: Pass attachments to `saveInvoice`**

In the dialog confirm callback, the call `err := a.saveInvoice(` currently passes
`originalPath,` as the first argument. Change the first arguments so the call begins:

```go
				err := a.saveInvoice(
					originalPath,
					attachments,
					companyEntry.Text,
```

(The remaining arguments stay unchanged.)

- [ ] **Step 4: Build**

Run: `go build ./internal/ui/`
Expected: FAIL — `too many arguments in call to a.saveInvoice`. Expected; Task 8 updates `saveInvoice`. Proceed.

---

### Task 8: Modal — Anhänge ablegen, Endung anpassen, CSV-Felder setzen

**Files:**
- Modify: `internal/ui/invoicemodal.go` (`saveInvoice`)

- [ ] **Step 1: Add `attachments` parameter to `saveInvoice`**

Change the signature of `saveInvoice`. It currently begins:

```go
func (a *App) saveInvoice(
	originalPath string,
	company string,
```

Insert the `attachments` parameter:

```go
func (a *App) saveInvoice(
	originalPath string,
	attachments []string,
	company string,
```

- [ ] **Step 2: Adjust the generated filename's extension**

In `saveInvoice`, immediately after the `filename, err := core.ApplyTemplate(...)` block
(after its `}` closing the `if err != nil`), add:

```go
	// The naming template ends with a literal ".pdf"; use the main file's
	// real extension instead (no-op when the main file is a PDF).
	if mainExt := strings.ToLower(filepath.Ext(originalPath)); mainExt != "" {
		filename = core.ReplaceExtension(filename, mainExt)
	}
```

- [ ] **Step 3: File attachments and set CSV fields in `completeSave`**

In `saveInvoice`, replace the entire `completeSave := func() error { ... }` closure with:

```go
	// Helper function to complete the save
	completeSave := func() error {
		// Move and rename the main invoice file
		finalFilename, err := a.storageManager.MoveAndRename(originalPath, targetFolder, filename)
		if err != nil {
			return fmt.Errorf("failed to move file: %w", err)
		}

		// File each attachment as <invoice>_AnhangN<ext>. seq numbers only
		// successfully filed attachments, so the suffixes stay contiguous.
		var failed []string
		seq := 0
		for _, attPath := range attachments {
			attExt := strings.ToLower(filepath.Ext(attPath))
			attName := core.AttachmentName(finalFilename, seq+1, attExt)
			if _, mvErr := a.storageManager.MoveAndRename(attPath, targetFolder, attName); mvErr != nil {
				a.logger.Warn("Failed to move attachment %s: %v", attPath, mvErr)
				failed = append(failed, filepath.Base(attPath))
				continue
			}
			seq++
		}

		// Update filename + attachment info
		meta.Dateiname = finalFilename
		newRow.Dateiname = finalFilename
		newRow.HatAnhaenge = seq > 0
		newRow.AnzahlAnhaenge = seq

		// Append to CSV
		if err := a.csvRepo.Append(csvPath, newRow); err != nil {
			return fmt.Errorf("failed to append to CSV: %w", err)
		}

		// Remember company mapping if requested
		if rememberMapping && company != "" {
			a.companyMap.Set(company, account)
			if err := a.companyMap.Save(); err != nil {
				a.logger.Warn("Failed to save company mapping: %v", err)
			}
		}

		// Attachment move failures are non-fatal: the invoice is filed.
		if len(failed) > 0 {
			a.showError(
				a.bundle.T("error.processing.title"),
				"Folgende Anhänge konnten nicht abgelegt werden: "+strings.Join(failed, ", "),
			)
		}

		a.logger.Info("Saved invoice: %s (%d attachments)", finalFilename, seq)
		return nil
	}
```

- [ ] **Step 4: Build the whole project**

Run: `go build ./...`
Expected: PASS — no errors. (`processSubmission`, the new 3-arg `showConfirmationModal`, and the new `saveInvoice` signature now all line up.)

- [ ] **Step 5: Vet**

Run: `go vet ./...`
Expected: PASS — no diagnostics.

- [ ] **Step 6: Run the full test suite**

Run: `go test ./...`
Expected: PASS — `internal/core` tests from Tasks 1 & 2 pass; other packages report `no test files`.

---

### Task 9: Build, Paketierung und Auslieferung

**Files:** none (build/deploy only)

- [ ] **Step 1: Final build + vet + tests**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: all succeed.

- [ ] **Step 2: Package the Windows executable**

Run (from the repo root `C:\Users\istok\Desktop\Dev\BuchISY`):
`fyne package -os windows -name BuchISY -src ./cmd/buchisy`
Expected: `cmd/buchisy/BuchISY.exe` is produced (~66 MB).

- [ ] **Step 3: Stop the running app**

Terminate any running `BuchISY` process (PowerShell `wmic process where "ProcessId=<id>" call terminate` per running PID, as established in this session).

- [ ] **Step 4: Deploy and restart**

Copy `cmd/buchisy/BuchISY.exe` over `C:\Users\istok\Desktop\BuchISY.exe`, then launch
`C:\Users\istok\Desktop\BuchISY.exe` with working directory `C:\Users\istok\Desktop`.

- [ ] **Step 5: Manual smoke test**

In the running app, open "PDF auswählen" / Datei auswählen and verify:
1. Office/LibreOffice/image files now appear in the list alongside PDFs.
2. Selecting one PDF only, marking it ★, "Öffnen" → extraction + modal as before (regression).
3. Selecting a PDF (★) plus two other files → modal shows "Anhänge (2): …"; after save, the month folder contains `<invoice>.pdf`, `<invoice>_Anhang1<ext>`, `<invoice>_Anhang2<ext>`, and `invoices.csv` shows `HatAnhaenge` = true, `AnzahlAnhaenge` = 2.
4. Selecting a non-PDF (e.g. `.xlsx`) as ★ → modal opens with empty fields; after manual entry + save, the filed main file keeps the `.xlsx` extension.

---

## Self-Review

**Spec coverage:**
- Mehrfachauswahl persistent über Ordnerwechsel → Task 5 (`selected` slice).
- Hauptdatei markieren → Task 5 (`mainPath`, ★-Button).
- Anhänge mit Suffix im Monatsordner → Task 8 (`core.AttachmentName`).
- Nicht-PDF-Hauptdatei → manuelle Eingabe → Task 6 (`processSubmission` PDF-Weiche).
- Erweiterte Dateitypen → Task 1 (`IsSupportedFile`) + Task 4 (Filter).
- Endungs-Ersetzung → Task 8 Step 2.
- CSV-Spalten `HatAnhaenge` / `AnzahlAnhaenge` → Tasks 2 & 3.
- Anhang-Fehler nicht fatal → Task 8 Step 3 (`failed`, `showError` als Warnung).
- Sortierung Hauptdatei vor Anhängen → automatisch (`.` < `_`), kein Code nötig.

**Placeholder scan:** keine TBD/TODO; alle Code-Schritte enthalten vollständigen Code.

**Type consistency:** `processSubmission(string, []string)`, `handleNoTextPDF(string, []string)`, `showConfirmationModal(string, []string, core.Meta)`, `saveInvoice(originalPath string, attachments []string, …)` — die Reihenfolge `(main/path, attachments, …)` ist überall identisch. `core.IsSupportedFile`, `core.IsPDF`, `core.ReplaceExtension`, `core.AttachmentName` werden in Task 1 definiert und in Tasks 4/6/8 mit exakt diesen Namen verwendet. CSV-Felder `HatAnhaenge bool` / `AnzahlAnhaenge int` konsistent in `types.go`, `csvrepo.go` und im Save-Flow.
