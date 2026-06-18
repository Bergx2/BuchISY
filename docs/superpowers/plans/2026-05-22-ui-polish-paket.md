# UI-Polish-Paket Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Beleg im Kassenbuch sichtbar/klickbar (+ Löschen-Button im Bearbeiten-Dialog), MwSt.-Labels + Datums-Labels umbenannt und je in eine Zeile gelegt, und eine zoombare Belegvorschau.

**Architecture:** i18n-Werte werden umbenannt. Die Belegvorschau (`documentpreview.go`) bekommt einen Zoomfaktor + Zoom-Buttons + Strg-Scroll und wird zu einem Typ `*documentPreview` mit Zoom-Methoden (rückwärtskompatibel, da weiterhin `fyne.CanvasObject`). Die beiden Rechnungs-Dialoge fassen Betrags-/Datumsfelder zu je einer Zeile zusammen und verdrahten Strg-Scroll. Die Kassenbuch-Bar-Ausgaben werden klickbar.

**Tech Stack:** Go 1.25, Fyne v2.6.3.

**Hinweis zu Commits:** Git-Commits sind in diesem Plan bewusst ausgelassen. Jede Aufgabe endet mit `go build`/`go vet`/`go test`.

---

### Task 1: i18n-Label-Umbenennungen

**Files:**
- Modify: `assets/i18n/de.json`
- Modify: `assets/i18n/en.json`

- [ ] **Step 1: Rename the German labels**

In `assets/i18n/de.json`, change these four values:

- `"field.vatPercent": "Steuersatz in %"` → `"field.vatPercent": "MwSt. %"`
- `"field.vatAmount": "Steuerbetrag"` → `"field.vatAmount": "MwSt.-Betrag"`
- `"table.col.vatPercent": "Steuersatz in %"` → `"table.col.vatPercent": "MwSt. %"`
- `"table.col.vatAmount": "Steuerbetrag"` → `"table.col.vatAmount": "MwSt.-Betrag"`
- `"field.invoiceDate": "Rechnungsdatum (DD.MM.YYYY)"` → `"field.invoiceDate": "Rechnungsdatum"`
- `"field.paymentDate": "Bezahldatum (DD.MM.YYYY)"` → `"field.paymentDate": "Bezahldatum"`

- [ ] **Step 2: Rename the English labels**

In `assets/i18n/en.json`, change:

- `"field.vatPercent": "Tax Rate in %"` → `"field.vatPercent": "VAT %"`
- `"field.vatAmount": "Tax Amount"` → `"field.vatAmount": "VAT Amount"`
- `"table.col.vatPercent": "Tax Rate in %"` → `"table.col.vatPercent": "VAT %"`
- `"table.col.vatAmount": "Tax Amount"` → `"table.col.vatAmount": "VAT Amount"`
- `"field.invoiceDate": "Invoice Date (DD.MM.YYYY)"` → `"field.invoiceDate": "Invoice Date"`
- `"field.paymentDate": "Payment Date (DD.MM.YYYY)"` → `"field.paymentDate": "Payment Date"`

- [ ] **Step 3: Build**

Run: `go build ./...`
Expected: PASS (the i18n keys are unchanged, only their values).

---

### Task 2: Zoombare Belegvorschau

**Files:**
- Modify: `internal/ui/documentpreview.go` (full rewrite)

- [ ] **Step 1: Rewrite `documentpreview.go`**

Replace the entire contents of `internal/ui/documentpreview.go` with:

```go
package ui

import (
	"fmt"
	"image"
	"image/color"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"github.com/bergx2/buchisy/internal/core"
)

const (
	// previewDPI is used both for rendering PDF pages and for computing
	// highlight rectangles — the two must match.
	previewDPI = 110.0
	// pdfPreviewPageGap is the vertical gap between rendered pages, px.
	pdfPreviewPageGap = 8
	previewZoomStep   = 0.25
	previewZoomMin    = 0.5
	previewZoomMax    = 4.0
)

// imagePreviewExtensions are the file types shown directly as an image.
var imagePreviewExtensions = map[string]struct{}{
	".jpg": {}, ".jpeg": {}, ".png": {}, ".gif": {},
	".bmp": {}, ".tif": {}, ".tiff": {}, ".webp": {},
}

// documentPreview is the preview panel returned by buildDocumentPreview.
// It is a fyne.CanvasObject via the embedded container and additionally
// exposes zoom controls used by the host window's Ctrl+scroll wiring.
type documentPreview struct {
	*fyne.Container
	strip *pdfPreviewStrip // non-nil only for PDF previews
}

// SetCtrlPressed tells the preview whether a Ctrl key is held, so that
// scroll events over the strip zoom instead of scroll.
func (p *documentPreview) SetCtrlPressed(v bool) {
	if p.strip != nil {
		p.strip.ctrlHeld = v
	}
}

// buildDocumentPreview returns a preview panel for the main file.
// Rendering runs in the background; a placeholder is shown until ready.
func buildDocumentPreview(mainPath string, meta core.Meta) *documentPreview {
	holder := container.NewStack(widget.NewLabel("Vorschau wird geladen …"))
	p := &documentPreview{Container: holder}

	go func() {
		content, strip := renderPreviewContent(mainPath, meta)
		fyne.DoAndWait(func() {
			p.strip = strip
			holder.Objects = []fyne.CanvasObject{content}
			holder.Refresh()
		})
	}()

	return p
}

// trackCtrlForPreview makes Ctrl+scroll over the preview zoom it, by
// tracking the Ctrl key state on the given window's canvas.
func trackCtrlForPreview(win fyne.Window, p *documentPreview) {
	dc, ok := win.Canvas().(desktop.Canvas)
	if !ok {
		return
	}
	isCtrl := func(n fyne.KeyName) bool {
		return n == desktop.KeyControlLeft || n == desktop.KeyControlRight
	}
	dc.SetOnKeyDown(func(ev *fyne.KeyEvent) {
		if isCtrl(ev.Name) {
			p.SetCtrlPressed(true)
		}
	})
	dc.SetOnKeyUp(func(ev *fyne.KeyEvent) {
		if isCtrl(ev.Name) {
			p.SetCtrlPressed(false)
		}
	})
}

// renderPreviewContent builds the preview content and, for PDFs, the strip.
func renderPreviewContent(mainPath string, meta core.Meta) (fyne.CanvasObject, *pdfPreviewStrip) {
	if core.IsPDF(mainPath) {
		images, err := core.RenderPDF(mainPath, previewDPI)
		if err != nil || len(images) == 0 {
			return previewPlaceholder(mainPath, "PDF-Vorschau nicht verfügbar"), nil
		}
		rectsPerPage, _ := core.HighlightRects(mainPath, highlightValues(meta), previewDPI)

		strip := newPdfPreviewStrip(images, rectsPerPage)
		scroll := container.NewScroll(strip)
		strip.scroll = scroll
		// When the strip's width changes (split drag, zoom), the scroll
		// must re-read the strip's height-for-that-width.
		strip.onWidthChange = scroll.Refresh

		zoomOut := widget.NewButton("−", func() { strip.setZoom(strip.zoom - previewZoomStep) })
		zoomReset := widget.NewButton("100%", func() { strip.setZoom(1.0) })
		zoomIn := widget.NewButton("+", func() { strip.setZoom(strip.zoom + previewZoomStep) })
		toolbar := container.NewHBox(zoomOut, zoomReset, zoomIn)

		return container.NewBorder(toolbar, nil, nil, nil, scroll), strip
	}

	if _, ok := imagePreviewExtensions[strings.ToLower(filepath.Ext(mainPath))]; ok {
		img := canvas.NewImageFromFile(mainPath)
		img.FillMode = canvas.ImageFillContain
		return container.NewScroll(img), nil
	}

	return previewPlaceholder(mainPath, "Keine Bildvorschau für dieses Dateiformat verfügbar"), nil
}

// previewPlaceholder is the centered fallback panel.
func previewPlaceholder(mainPath, message string) fyne.CanvasObject {
	msg := widget.NewLabel(message)
	msg.Alignment = fyne.TextAlignCenter
	name := widget.NewLabel(filepath.Base(mainPath))
	name.Alignment = fyne.TextAlignCenter
	return container.NewCenter(container.NewVBox(msg, name))
}

// highlightValues collects the extracted values to look for in the document.
func highlightValues(meta core.Meta) []string {
	vals := []string{meta.Firmenname, meta.Rechnungsnummer, meta.Waehrung}
	for _, amt := range []float64{meta.BetragNetto, meta.SteuersatzBetrag, meta.Bruttobetrag} {
		dot := fmt.Sprintf("%.2f", amt)
		vals = append(vals, dot, strings.ReplaceAll(dot, ".", ","))
	}
	return vals
}

// pdfPreviewStrip displays rendered PDF pages stacked vertically, scaling
// every page (and its highlight rectangles) to the widget's current width.
type pdfPreviewStrip struct {
	widget.BaseWidget
	pageImgs      []*canvas.Image
	pageNative    []fyne.Size
	rects         [][]core.Rect
	rectObjs      [][]*canvas.Rectangle
	lastWidth     float32
	zoom          float32           // 1.0 = fit width
	baseWidth     float32           // strip width observed at zoom == 1
	scroll        *container.Scroll // enclosing scroll, for forwarding scroll events
	ctrlHeld      bool              // set by the host window's Ctrl tracking
	onWidthChange func()
}

func newPdfPreviewStrip(images []image.Image, rectsPerPage [][]core.Rect) *pdfPreviewStrip {
	s := &pdfPreviewStrip{zoom: 1.0}
	for i, img := range images {
		ci := canvas.NewImageFromImage(img)
		ci.FillMode = canvas.ImageFillStretch
		s.pageImgs = append(s.pageImgs, ci)

		b := img.Bounds()
		s.pageNative = append(s.pageNative, fyne.NewSize(float32(b.Dx()), float32(b.Dy())))

		var rcs []core.Rect
		if i < len(rectsPerPage) {
			rcs = rectsPerPage[i]
		}
		s.rects = append(s.rects, rcs)

		objs := make([]*canvas.Rectangle, len(rcs))
		for j := range rcs {
			objs[j] = canvas.NewRectangle(color.NRGBA{R: 255, G: 235, B: 0, A: 90})
		}
		s.rectObjs = append(s.rectObjs, objs)
	}
	s.ExtendBaseWidget(s)
	return s
}

// setZoom clamps and applies a new zoom factor.
func (s *pdfPreviewStrip) setZoom(z float32) {
	if z < previewZoomMin {
		z = previewZoomMin
	}
	if z > previewZoomMax {
		z = previewZoomMax
	}
	if z == s.zoom {
		return
	}
	s.zoom = z
	s.Refresh()
	if s.onWidthChange != nil {
		s.onWidthChange()
	}
}

// Scrolled zooms when Ctrl is held; otherwise it forwards the scroll to
// the enclosing scroll container so normal scrolling still works.
func (s *pdfPreviewStrip) Scrolled(ev *fyne.ScrollEvent) {
	if s.ctrlHeld {
		if ev.Scrolled.DY > 0 {
			s.setZoom(s.zoom + previewZoomStep)
		} else if ev.Scrolled.DY < 0 {
			s.setZoom(s.zoom - previewZoomStep)
		}
		return
	}
	if s.scroll != nil {
		s.scroll.Scrolled(ev)
	}
}

// Resize records the new width; at zoom 1 it also captures the fit width.
func (s *pdfPreviewStrip) Resize(size fyne.Size) {
	widthChanged := size.Width != s.lastWidth
	s.lastWidth = size.Width
	if s.zoom == 1 {
		s.baseWidth = size.Width
	}
	s.BaseWidget.Resize(size)
	if widthChanged && s.onWidthChange != nil {
		s.onWidthChange()
	}
}

func (s *pdfPreviewStrip) CreateRenderer() fyne.WidgetRenderer {
	return &pdfPreviewStripRenderer{strip: s}
}

type pdfPreviewStripRenderer struct {
	strip *pdfPreviewStrip
}

func (r *pdfPreviewStripRenderer) Layout(size fyne.Size) {
	s := r.strip
	if size.Width <= 0 {
		return
	}
	y := float32(0)
	for i, img := range s.pageImgs {
		native := s.pageNative[i]
		if native.Width <= 0 {
			continue
		}
		scale := size.Width / native.Width
		pageH := native.Height * scale
		img.Resize(fyne.NewSize(size.Width, pageH))
		img.Move(fyne.NewPos(0, y))
		for j, rc := range s.rects[i] {
			ro := s.rectObjs[i][j]
			ro.Resize(fyne.NewSize(rc.W*scale, rc.H*scale))
			ro.Move(fyne.NewPos(rc.X*scale, y+rc.Y*scale))
		}
		y += pageH + pdfPreviewPageGap
	}
}

// MinSize keeps the width small at zoom 1 (so a split divider moves
// freely); when zoomed it forces the width to baseWidth*zoom so the
// enclosing scroll shows horizontal + vertical scrollbars.
func (r *pdfPreviewStripRenderer) MinSize() fyne.Size {
	s := r.strip
	w := s.lastWidth
	if w <= 0 {
		w = 600
	}
	minW := float32(120)
	if s.zoom != 1 && s.baseWidth > 0 {
		w = s.baseWidth * s.zoom
		minW = w
	}
	total := float32(0)
	for i := range s.pageImgs {
		native := s.pageNative[i]
		if native.Width <= 0 {
			continue
		}
		total += native.Height*(w/native.Width) + pdfPreviewPageGap
	}
	return fyne.NewSize(minW, total)
}

func (r *pdfPreviewStripRenderer) Refresh() {
	r.Layout(r.strip.Size())
	for _, o := range r.Objects() {
		o.Refresh()
	}
}

func (r *pdfPreviewStripRenderer) Objects() []fyne.CanvasObject {
	var objs []fyne.CanvasObject
	for i, img := range r.strip.pageImgs {
		objs = append(objs, img)
		for _, ro := range r.strip.rectObjs[i] {
			objs = append(objs, ro)
		}
	}
	return objs
}

func (r *pdfPreviewStripRenderer) Destroy() {}
```

- [ ] **Step 2: Build, vet, test**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS. `buildDocumentPreview` now returns `*documentPreview`; the existing callers (`invoicemodal.go`, `tableedit.go`) assign it to a variable and pass it to `container.NewHSplit` — `*documentPreview` is a `fyne.CanvasObject` via the embedded `*fyne.Container`, so they still compile unchanged.

---

### Task 3: „Rechnungsdaten prüfen" — MwSt./Datum-Zeilen + Zoom-Verdrahtung

**Files:**
- Modify: `internal/ui/invoicemodal.go`

- [ ] **Step 1: Merge the date and amount form rows**

In `internal/ui/invoicemodal.go`, the form inside `showConfirmationModal` currently is:

```go
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
```

Replace it with (the two date FormItems become one row; the three amount FormItems become one row):

```go
		widget.NewForm(
			widget.NewFormItem(a.bundle.T("field.company"), companyEntry),
			widget.NewFormItem(a.bundle.T("field.shortdesc"), container.NewBorder(nil, nil, nil, shortDescLabel, shortDescEntry)),
			widget.NewFormItem(a.bundle.T("field.invoicenumber"), invoiceNumEntry),
			widget.NewFormItem("", container.NewGridWithColumns(2,
				container.NewBorder(nil, nil, widget.NewLabel(a.bundle.T("field.invoiceDate")), dateCalendarBtn, dateEntry),
				container.NewBorder(nil, nil, widget.NewLabel(a.bundle.T("field.paymentDate")), paymentDateCalendarBtn, paymentDateEntry),
			)),
			widget.NewFormItem("", container.NewGridWithColumns(3,
				container.NewBorder(nil, nil, widget.NewLabel(a.bundle.T("field.net")), nil, netEntry),
				container.NewBorder(nil, nil, widget.NewLabel(a.bundle.T("field.vatPercent")), nil, vatPercentEntry),
				container.NewBorder(nil, nil, widget.NewLabel(a.bundle.T("field.vatAmount")), nil, vatAmountEntry),
			)),
			widget.NewFormItem(a.bundle.T("field.gross"), grossEntry),
			widget.NewFormItem(a.bundle.T("field.currency"), currencySelect),
			widget.NewFormItem(a.bundle.T("field.account"), accountSelect),
			widget.NewFormItem(a.bundle.T("field.bankAccount"), bankAccountSelect),
			widget.NewFormItem("", partialPaymentCheck),
		),
```

- [ ] **Step 2: Wire Ctrl+scroll for the preview**

In `internal/ui/invoicemodal.go`, find the line `preview := buildDocumentPreview(originalPath, meta)`. Immediately after the `confirmWin` is created and before `confirmWin.Show()`, the preview's Ctrl tracking must be registered. Add this line right after `confirmWin = a.app.NewWindow(a.bundle.T("modal.title"))`:

```go
	confirmWin = a.app.NewWindow(a.bundle.T("modal.title"))
	trackCtrlForPreview(confirmWin, preview)
```

(`preview` is the `*documentPreview` returned by `buildDocumentPreview`; it is defined earlier in the function. `trackCtrlForPreview` is defined in `documentpreview.go`.)

- [ ] **Step 3: Build, vet, test**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS.

---

### Task 4: „Rechnung bearbeiten" + Kassenbuch — Zeilen, Löschen, klickbarer Beleg, Zoom

**Files:**
- Modify: `internal/ui/tableedit.go`
- Modify: `internal/ui/table.go`
- Modify: `internal/ui/kassenbuchview.go`

- [ ] **Step 1: `showEditDialog` gains an `onClose` parameter**

In `internal/ui/tableedit.go`, change the `showEditDialog` signature:

```go
func (a *App) showEditDialog(row core.CSVRow) {
```

to:

```go
func (a *App) showEditDialog(row core.CSVRow, onClose func()) {
```

In the same function, the `editWin.SetOnClosed` handler currently is:

```go
	editWin.SetOnClosed(func() {
		a.settings.PreviewSplitOffset = split.Offset
		if err := a.settingsMgr.Save(a.settings); err != nil {
			a.logger.Warn("Failed to save preview split offset: %v", err)
		}
	})
```

Change it to also fire `onClose`:

```go
	editWin.SetOnClosed(func() {
		a.settings.PreviewSplitOffset = split.Offset
		if err := a.settingsMgr.Save(a.settings); err != nil {
			a.logger.Warn("Failed to save preview split offset: %v", err)
		}
		if onClose != nil {
			onClose()
		}
	})
```

- [ ] **Step 2: Merge the date and amount form rows in `showEditDialog`**

In `internal/ui/tableedit.go`, the form inside `showEditDialog` currently contains these FormItems:

```go
			widget.NewFormItem(a.bundle.T("field.invoiceDate"), container.NewBorder(nil, nil, nil, dateCalendarBtn, dateEntry)),
			widget.NewFormItem(a.bundle.T("field.paymentDate"), container.NewBorder(nil, nil, nil, paymentDateCalendarBtn, paymentDateEntry)),
			widget.NewFormItem(a.bundle.T("field.net"), netEntry),
			widget.NewFormItem(a.bundle.T("field.vatPercent"), vatPercentEntry),
			widget.NewFormItem(a.bundle.T("field.vatAmount"), vatAmountEntry),
```

Replace those five lines with:

```go
			widget.NewFormItem("", container.NewGridWithColumns(2,
				container.NewBorder(nil, nil, widget.NewLabel(a.bundle.T("field.invoiceDate")), dateCalendarBtn, dateEntry),
				container.NewBorder(nil, nil, widget.NewLabel(a.bundle.T("field.paymentDate")), paymentDateCalendarBtn, paymentDateEntry),
			)),
			widget.NewFormItem("", container.NewGridWithColumns(3,
				container.NewBorder(nil, nil, widget.NewLabel(a.bundle.T("field.net")), nil, netEntry),
				container.NewBorder(nil, nil, widget.NewLabel(a.bundle.T("field.vatPercent")), nil, vatPercentEntry),
				container.NewBorder(nil, nil, widget.NewLabel(a.bundle.T("field.vatAmount")), nil, vatAmountEntry),
			)),
```

- [ ] **Step 3: Add a "Löschen" button and wire Ctrl+scroll**

In `internal/ui/tableedit.go`, `showEditDialog`, the `buttonBar` is currently:

```go
	buttonBar := container.NewBorder(nil, nil, nil,
		container.NewHBox(cancelBtn, saveBtn),
	)
```

Replace it with (add a delete button on the left):

```go
	deleteBtn := widget.NewButton("Löschen", func() {
		dialog.ShowConfirm(
			a.bundle.T("table.delete.confirm.title"),
			a.bundle.T("table.delete.confirm.message", row.Dateiname, row.Firmenname, row.Bruttobetrag, row.Waehrung),
			func(confirm bool) {
				if confirm {
					a.deleteInvoice(row)
					editWin.Close()
				}
			},
			editWin,
		)
	})
	deleteBtn.Importance = widget.DangerImportance

	buttonBar := container.NewBorder(nil, nil, deleteBtn,
		container.NewHBox(cancelBtn, saveBtn),
	)
```

In the same function, immediately after `editWin = a.app.NewWindow("Rechnung bearbeiten")`, add:

```go
	editWin = a.app.NewWindow("Rechnung bearbeiten")
	trackCtrlForPreview(editWin, preview)
```

(`preview` is the `*documentPreview` from `buildDocumentPreview`, defined earlier in `showEditDialog`.)

- [ ] **Step 4: Update the table caller of `showEditDialog`**

In `internal/ui/table.go`, the call `it.app.showEditDialog(it.filtered[dataRow])` becomes:

```go
						it.app.showEditDialog(it.filtered[dataRow], nil)
```

- [ ] **Step 5: Make the Kassenbuch cash-expense rows show the filename and open the edit dialog**

In `internal/ui/kassenbuchview.go`, inside `showCashBookView`'s `rebuild` closure, the cash-expense list is currently:

```go
		// Read-only cash invoices + computed end balance
		invoices := a.cashInvoicesFor(account)
		entries, endbestand := core.ComputeCashReport(*book, invoices)

		outflowList := container.NewVBox()
		for _, e := range entries {
			if e.Ausgabe == 0 {
				continue
			}
			outflowList.Add(widget.NewLabel(fmt.Sprintf(
				"  %s  —  %s  —  %s", e.Datum, e.Beschreibung, formatDecimal(e.Ausgabe, a.settings.DecimalSeparator))))
		}
		if len(outflowList.Objects) == 0 {
			outflowList.Add(widget.NewLabel("  (keine Bar-Ausgaben in diesem Monat)"))
		}
```

Replace it with (each cash expense becomes a clickable button showing the filename; clicking opens the edit dialog, and on close the cash book is rebuilt):

```go
		// Cash invoices (clickable) + computed end balance.
		invoices := a.cashInvoicesFor(account)
		entries, endbestand := core.ComputeCashReport(*book, invoices)

		rowByName := make(map[string]core.CSVRow, len(invoices))
		for _, inv := range invoices {
			rowByName[inv.Dateiname] = inv
		}

		outflowList := container.NewVBox()
		for _, e := range entries {
			if e.Ausgabe == 0 {
				continue
			}
			row, ok := rowByName[e.Beleg]
			label := fmt.Sprintf("%s  —  %s  —  %s  —  %s",
				e.Datum, e.Beschreibung,
				formatDecimal(e.Ausgabe, a.settings.DecimalSeparator), e.Beleg)
			if !ok {
				outflowList.Add(widget.NewLabel("  " + label))
				continue
			}
			btn := widget.NewButton(label, func() {
				a.showEditDialog(row, rebuild)
			})
			btn.Importance = widget.LowImportance
			btn.Alignment = widget.ButtonAlignLeading
			outflowList.Add(btn)
		}
		if len(outflowList.Objects) == 0 {
			outflowList.Add(widget.NewLabel("  (keine Bar-Ausgaben in diesem Monat)"))
		}
```

- [ ] **Step 6: Build, vet, test**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS — `showEditDialog` now takes `(core.CSVRow, func())`; both call sites (`table.go` with `nil`, `kassenbuchview.go` with `rebuild`) match; `dialog` is already imported in `tableedit.go`.

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

Terminate any running `BuchISY` process (PowerShell `wmic process where "ProcessId=<id>" call terminate` per running PID).

- [ ] **Step 4: Deploy and restart**

Copy `cmd/buchisy/BuchISY.exe` over `C:\Users\istok\Desktop\BuchISY.exe`, then launch
`C:\Users\istok\Desktop\BuchISY.exe` with working directory `C:\Users\istok\Desktop`.

- [ ] **Step 5: Manual smoke test**

1. Kassenbuch öffnen → Bar-Ausgaben zeigen den Dateinamen und sind klickbar → „Rechnung bearbeiten" öffnet; nach Schließen ist das Kassenbuch aktualisiert.
2. „Rechnung bearbeiten" hat unten links einen „Löschen"-Button → nach Bestätigung Rechnung + Datei weg, Fenster schließt.
3. Beide Dialoge: Rechnungsdatum/Bezahldatum in einer Zeile (ohne „(DD.MM.YYYY)"); Betrag netto / MwSt. % / MwSt.-Betrag in einer Zeile.
4. Belegvorschau: „+/−/100%" zoomen; Strg + Mausrad über der Vorschau zoomt; bei Zoom > 100 % erscheinen Scrollbalken.

---

## Self-Review

**Spec coverage:**
- F1 Beleg im Kassenbuch sichtbar + klickbar → Task 4 Step 5; `showEditDialog`-`onClose` → Task 4 Step 1; „Löschen"-Button → Task 4 Step 3; Tabellen-Aufruf angepasst → Task 4 Step 4.
- F2 MwSt.-Umbenennung → Task 1; Betrag netto/MwSt. %/MwSt.-Betrag eine Zeile → Task 3 Step 1 + Task 4 Step 2.
- F3 zoombare Vorschau (Zoomfaktor, +/−/100%-Buttons, Strg-Scroll, beidseitiges Scrollen) → Task 2; Strg-Verdrahtung der Fenster → Task 3 Step 2 + Task 4 Step 3.
- F4 Datums-Labels ohne „(DD.MM.YYYY)" → Task 1; Rechnungsdatum/Bezahldatum eine Zeile → Task 3 Step 1 + Task 4 Step 2.

**Placeholder scan:** Keine TBD/TODO; alle Code-Schritte enthalten vollständigen Code.

**Type consistency:** `buildDocumentPreview` liefert `*documentPreview` (Task 2); die bestehenden Aufrufstellen weisen das Ergebnis einer Variablen zu und übergeben es an `container.NewHSplit` — `*documentPreview` erfüllt `fyne.CanvasObject` via eingebettetem `*fyne.Container`, daher kompilieren sie unverändert. `trackCtrlForPreview(fyne.Window, *documentPreview)` (Task 2) wird in Task 3/4 mit genau diesen Typen aufgerufen. `showEditDialog(core.CSVRow, func())` (Task 4 Step 1) — beide Aufrufstellen (`table.go` Step 4, `kassenbuchview.go` Step 5) übergeben das zweite Argument. `pdfPreviewStrip` implementiert durch die neue `Scrolled`-Methode `fyne.Scrollable`. `widget.ButtonAlignLeading` und `widget.DangerImportance` sind bestehende Fyne-Konstanten.
