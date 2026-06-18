# Beleg-Vorschau Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Das „Rechnungsdaten prüfen"-Fenster wird zweigeteilt — links das (schmalere) Formular, rechts eine Vorschau der Belegdatei; bei PDFs werden die extrahierten Werte gelb markiert.

**Architecture:** Ein neuer go-fitz/MuPDF-Wrapper rendert PDF-Seiten zu Bildern. Eine eigene Markierungs-Komponente ermittelt über die bestehende `ledongthuc/pdf`-Lib die Wortpositionen, sucht die extrahierten Werte darin und rechnet PDF-Punkte in Bildpixel um. Eine Vorschau-UI-Komponente zeigt PDF-Seitenbilder (mit gelben Overlay-Rechtecken), Bilddateien direkt oder einen Platzhalter. Das Modal-Fenster wird ein `HSplit` aus Formular und Vorschau.

**Tech Stack:** Go 1.25, Fyne v2.6.3, `github.com/gen2brain/go-fitz` (MuPDF, CGO), `github.com/ledongthuc/pdf`.

**Hinweis zu Commits:** Git-Commits sind in diesem Plan bewusst ausgelassen — Auslieferung erfolgt per Build + Kopie der `.exe`. Jede Aufgabe endet mit `go build`/`go vet`/`go test` als Verifikation.

**Render-Auflösung:** Konstante 110 DPI (A4 ≈ 910 px breit, passt ohne Horizontal-Scroll in das Vorschau-Panel). Seitenbilder werden in nativer Pixelgröße angezeigt; die Markierungs-Rechtecke liegen 1:1 in denselben Pixelkoordinaten.

---

### Task 1: go-fitz-Abhängigkeit + PDF-Rendering

**Files:**
- Modify: `go.mod`, `go.sum`
- Create: `internal/core/pdfrender.go`

Diese Aufgabe bindet die native Render-Bibliothek ein und verifiziert früh, dass sie auf diesem Rechner baut (Haupt-Risiko des Features).

- [ ] **Step 1: Add the dependency**

Run: `go get github.com/gen2brain/go-fitz@latest`
Expected: `go.mod`/`go.sum` erhalten den neuen Eintrag, keine Fehler.

- [ ] **Step 2: Write the renderer**

Create `internal/core/pdfrender.go`:

```go
package core

import (
	"fmt"
	"image"

	"github.com/gen2brain/go-fitz"
)

// RenderPDF renders every page of the PDF at the given DPI to an image.
// Pages are returned in document order (index 0 = page 1).
func RenderPDF(path string, dpi float64) ([]image.Image, error) {
	doc, err := fitz.New(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open PDF for rendering: %w", err)
	}
	defer doc.Close()

	pages := make([]image.Image, 0, doc.NumPage())
	for n := 0; n < doc.NumPage(); n++ {
		img, err := doc.ImageDPI(n, dpi)
		if err != nil {
			return nil, fmt.Errorf("failed to render PDF page %d: %w", n+1, err)
		}
		pages = append(pages, img)
	}
	return pages, nil
}
```

- [ ] **Step 3: Build to verify go-fitz compiles and links**

Run: `go build ./...`
Expected: PASS. This proves the CGO/MuPDF dependency builds in this environment.

If the compiler reports `doc.ImageDPI undefined`, the resolved go-fitz version is older: replace `doc.ImageDPI(n, dpi)` with `doc.Image(n)` and report this as DONE_WITH_CONCERNS so the controller can pin the DPI constant accordingly. If `go build` fails because MuPDF cannot link, report BLOCKED — the feature's PDF preview is not viable in this environment.

- [ ] **Step 4: Vet**

Run: `go vet ./internal/core/...`
Expected: PASS.

---

### Task 2: Markierungs-Geometrie — reine Hilfsfunktionen (TDD)

**Files:**
- Create: `internal/core/pdfhighlight.go`
- Test: `internal/core/pdfhighlight_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/core/pdfhighlight_test.go`:

```go
package core

import (
	"testing"

	"github.com/ledongthuc/pdf"
)

func TestPdfBoxToPixel(t *testing.T) {
	// dpi 144 -> scale 2.0. Page 842pt high. Box at (100,700) size 50x10.
	// Y flips: imageY = (842 - 700 - 10) * 2 = 264.
	got := pdfBoxToPixel(pdfBox{x: 100, y: 700, w: 50, h: 10}, 842, 144)
	want := Rect{X: 200, Y: 264, W: 100, H: 20}
	if got != want {
		t.Errorf("pdfBoxToPixel = %+v, want %+v", got, want)
	}
}

func TestValueBoxInRow(t *testing.T) {
	frags := []pdf.Text{
		{X: 10, Y: 100, W: 30, FontSize: 12, S: "Rechnung "},
		{X: 40, Y: 100, W: 25, FontSize: 12, S: "Nr. "},
		{X: 65, Y: 100, W: 40, FontSize: 12, S: "2025-0815"},
	}

	// Single fragment, case-insensitive.
	box, ok := valueBoxInRow(frags, "2025-0815")
	if !ok {
		t.Fatal("expected to find 2025-0815")
	}
	if box != (pdfBox{x: 65, y: 100, w: 40, h: 12}) {
		t.Errorf("single-fragment box = %+v", box)
	}

	// Spanning two fragments: "Nr. 2025" covers frag 2 and frag 3.
	box, ok = valueBoxInRow(frags, "Nr. 2025")
	if !ok {
		t.Fatal("expected to find spanning value")
	}
	if box != (pdfBox{x: 40, y: 100, w: 65, h: 12}) {
		t.Errorf("spanning box = %+v", box)
	}

	// Not present.
	if _, ok := valueBoxInRow(frags, "Telekom"); ok {
		t.Error("Telekom should not be found")
	}

	// Empty value never matches.
	if _, ok := valueBoxInRow(frags, "  "); ok {
		t.Error("empty value should not match")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run "TestPdfBoxToPixel|TestValueBoxInRow" -v`
Expected: FAIL — `undefined: pdfBoxToPixel`, `undefined: pdfBox`, `undefined: valueBoxInRow`, `undefined: Rect`.

- [ ] **Step 3: Write the implementation**

Create `internal/core/pdfhighlight.go`:

```go
package core

import (
	"strings"

	"github.com/ledongthuc/pdf"
)

// Rect is a rectangle in image pixels (origin top-left).
type Rect struct {
	X, Y, W, H float32
}

// pdfBox is a rectangle in PDF points (origin bottom-left).
type pdfBox struct {
	x, y, w, h float64
}

// pdfBoxToPixel converts a PDF-point box to an image-pixel Rect.
// pageHeight is the page height in PDF points; dpi the render resolution.
// The Y axis is flipped (PDF origin bottom-left, image origin top-left).
func pdfBoxToPixel(b pdfBox, pageHeight, dpi float64) Rect {
	scale := dpi / 72.0
	return Rect{
		X: float32(b.x * scale),
		Y: float32((pageHeight - b.y - b.h) * scale),
		W: float32(b.w * scale),
		H: float32(b.h * scale),
	}
}

// valueBoxInRow searches one row of text fragments for value (verbatim,
// case-insensitive, whitespace-trimmed) and returns the PDF-point box
// enclosing the matching fragments. ok is false if value is empty or does
// not appear in the row.
func valueBoxInRow(frags []pdf.Text, value string) (box pdfBox, ok bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return pdfBox{}, false
	}

	var sb strings.Builder
	starts := make([]int, len(frags))
	ends := make([]int, len(frags))
	for i, f := range frags {
		starts[i] = sb.Len()
		sb.WriteString(f.S)
		ends[i] = sb.Len()
	}

	idx := strings.Index(strings.ToLower(sb.String()), strings.ToLower(value))
	if idx < 0 {
		return pdfBox{}, false
	}
	matchStart, matchEnd := idx, idx+len(value)

	first := true
	var minX, minY, maxX, maxY float64
	for i, f := range frags {
		if ends[i] <= matchStart || starts[i] >= matchEnd {
			continue
		}
		fh := f.FontSize
		if fh <= 0 {
			fh = 8
		}
		fx0, fy0 := f.X, f.Y
		fx1, fy1 := f.X+f.W, f.Y+fh
		if first {
			minX, minY, maxX, maxY = fx0, fy0, fx1, fy1
			first = false
			continue
		}
		if fx0 < minX {
			minX = fx0
		}
		if fy0 < minY {
			minY = fy0
		}
		if fx1 > maxX {
			maxX = fx1
		}
		if fy1 > maxY {
			maxY = fy1
		}
	}
	if first {
		return pdfBox{}, false
	}
	return pdfBox{x: minX, y: minY, w: maxX - minX, h: maxY - minY}, true
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run "TestPdfBoxToPixel|TestValueBoxInRow" -v`
Expected: PASS — both tests ok.

---

### Task 3: HighlightRects — Integration über die ganze PDF

**Files:**
- Modify: `internal/core/pdfhighlight.go`

- [ ] **Step 1: Add the integration function**

Append to `internal/core/pdfhighlight.go` (the `pdf` import is already present):

```go
// HighlightRects opens the PDF and returns, per page (index 0 = page 1),
// the list of yellow highlight rectangles in image pixels for the render
// resolution dpi. dpi must match the value passed to RenderPDF. Values not
// found verbatim in a page's text produce no rectangle. A page whose text
// cannot be read yields a nil slice (no highlights, no error).
func HighlightRects(path string, values []string, dpi float64) ([][]Rect, error) {
	f, r, err := pdf.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open PDF for highlighting: %w", err)
	}
	defer f.Close()

	total := r.NumPage()
	result := make([][]Rect, 0, total)
	for pageNum := 1; pageNum <= total; pageNum++ {
		page := r.Page(pageNum)
		if page.V.IsNull() {
			result = append(result, nil)
			continue
		}
		rows, err := page.GetTextByRow()
		if err != nil {
			result = append(result, nil)
			continue
		}
		pageHeight := pageHeightPoints(page)

		var rects []Rect
		for _, row := range rows {
			frags := []pdf.Text(row.Content)
			for _, value := range values {
				if box, ok := valueBoxInRow(frags, value); ok {
					rects = append(rects, pdfBoxToPixel(box, pageHeight, dpi))
				}
			}
		}
		result = append(result, rects)
	}
	return result, nil
}

// pageHeightPoints returns the page height in PDF points from its MediaBox,
// falling back to A4 height if the MediaBox is missing or malformed.
func pageHeightPoints(page pdf.Page) float64 {
	mb := page.MediaBox()
	if mb.Len() < 4 {
		return 842
	}
	return mb.Index(3).Float64() - mb.Index(1).Float64()
}
```

- [ ] **Step 2: Add the `fmt` import**

`internal/core/pdfhighlight.go` now uses `fmt`. Change the import block at the top of the file from:

```go
import (
	"strings"

	"github.com/ledongthuc/pdf"
)
```

to:

```go
import (
	"fmt"
	"strings"

	"github.com/ledongthuc/pdf"
)
```

- [ ] **Step 3: Build and test**

Run: `go build ./... && go test ./internal/core/... && go vet ./internal/core/...`
Expected: PASS — build clean, the Task 2 tests still pass, vet clean. (`HighlightRects` itself has no unit test — it needs a real PDF and is verified in the Task 6 smoke test.)

---

### Task 4: Vorschau-Komponente

**Files:**
- Create: `internal/ui/documentpreview.go`

- [ ] **Step 1: Write the preview component**

Create `internal/ui/documentpreview.go`:

```go
package ui

import (
	"fmt"
	"image/color"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/bergx2/buchisy/internal/core"
)

// previewDPI is the resolution used both for rendering PDF pages and for
// computing highlight rectangles — the two must match.
const previewDPI = 110.0

// imagePreviewExtensions are the file types shown directly as an image.
var imagePreviewExtensions = map[string]struct{}{
	".jpg": {}, ".jpeg": {}, ".png": {}, ".gif": {},
	".bmp": {}, ".tif": {}, ".tiff": {}, ".webp": {},
}

// buildDocumentPreview returns a panel showing a preview of the main file.
// Rendering runs in the background; a placeholder is shown until it is ready.
func buildDocumentPreview(mainPath string, meta core.Meta) fyne.CanvasObject {
	holder := container.NewStack(widget.NewLabel("Vorschau wird geladen …"))

	go func() {
		content := renderPreviewContent(mainPath, meta)
		holder.Objects = []fyne.CanvasObject{content}
		holder.Refresh()
	}()

	return holder
}

// renderPreviewContent builds the actual preview for the main file.
func renderPreviewContent(mainPath string, meta core.Meta) fyne.CanvasObject {
	if core.IsPDF(mainPath) {
		images, err := core.RenderPDF(mainPath, previewDPI)
		if err != nil || len(images) == 0 {
			return previewPlaceholder(mainPath, "PDF-Vorschau nicht verfügbar")
		}
		rectsPerPage, _ := core.HighlightRects(mainPath, highlightValues(meta), previewDPI)

		pages := container.NewVBox()
		for i, img := range images {
			var rects []core.Rect
			if i < len(rectsPerPage) {
				rects = rectsPerPage[i]
			}
			pages.Add(buildPdfPage(img, rects))
		}
		return container.NewVScroll(pages)
	}

	if _, ok := imagePreviewExtensions[strings.ToLower(filepath.Ext(mainPath))]; ok {
		img := canvas.NewImageFromFile(mainPath)
		img.FillMode = canvas.ImageFillContain
		return container.NewVScroll(img)
	}

	return previewPlaceholder(mainPath, "Keine Bildvorschau für dieses Dateiformat verfügbar")
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
// Amounts are searched both dot- and comma-formatted.
func highlightValues(meta core.Meta) []string {
	vals := []string{meta.Firmenname, meta.Rechnungsnummer, meta.Waehrung}
	for _, amt := range []float64{meta.BetragNetto, meta.SteuersatzBetrag, meta.Bruttobetrag} {
		dot := fmt.Sprintf("%.2f", amt)
		vals = append(vals, dot, strings.ReplaceAll(dot, ".", ","))
	}
	return vals
}

// buildPdfPage stacks one rendered page image with its highlight rectangles.
func buildPdfPage(img fyne.Resource, rects []core.Rect) fyne.CanvasObject {
	_ = img
	return nil // replaced in Step 2
}
```

(The `buildPdfPage` stub in Step 1 is replaced in Step 2 — it is written separately because it needs the custom layout type.)

- [ ] **Step 2: Replace the page builder with the real implementation + custom layout**

In `internal/ui/documentpreview.go`, replace the stub function

```go
// buildPdfPage stacks one rendered page image with its highlight rectangles.
func buildPdfPage(img fyne.Resource, rects []core.Rect) fyne.CanvasObject {
	_ = img
	return nil // replaced in Step 2
}
```

with:

```go
// buildPdfPage stacks one rendered page image with its highlight rectangles.
// The page is shown at native pixel size; rectangles overlay 1:1.
func buildPdfPage(img *canvas.Image, imgSize fyne.Size, rects []core.Rect) fyne.CanvasObject {
	objects := []fyne.CanvasObject{img}
	for range rects {
		r := canvas.NewRectangle(color.NRGBA{R: 255, G: 235, B: 0, A: 90})
		objects = append(objects, r)
	}
	return container.New(&pageOverlayLayout{imgSize: imgSize, rects: rects}, objects...)
}

// pageOverlayLayout places a page image at the origin in its native size
// and each highlight rectangle at its native pixel coordinates.
type pageOverlayLayout struct {
	imgSize fyne.Size
	rects   []core.Rect
}

func (l *pageOverlayLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	return l.imgSize
}

func (l *pageOverlayLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	if len(objects) == 0 {
		return
	}
	objects[0].Resize(l.imgSize)
	objects[0].Move(fyne.NewPos(0, 0))
	for i, r := range l.rects {
		if 1+i >= len(objects) {
			break
		}
		objects[1+i].Resize(fyne.NewSize(r.W, r.H))
		objects[1+i].Move(fyne.NewPos(r.X, r.Y))
	}
}
```

- [ ] **Step 3: Update the PDF branch to use the real `buildPdfPage` signature**

In `renderPreviewContent`, replace the PDF page loop

```go
		pages := container.NewVBox()
		for i, img := range images {
			var rects []core.Rect
			if i < len(rectsPerPage) {
				rects = rectsPerPage[i]
			}
			pages.Add(buildPdfPage(img, rects))
		}
		return container.NewVScroll(pages)
```

with:

```go
		pages := container.NewVBox()
		for i, img := range images {
			var rects []core.Rect
			if i < len(rectsPerPage) {
				rects = rectsPerPage[i]
			}
			canvasImg := canvas.NewImageFromImage(img)
			canvasImg.FillMode = canvas.ImageFillOriginal
			imgSize := fyne.NewSize(
				float32(img.Bounds().Dx()),
				float32(img.Bounds().Dy()),
			)
			pages.Add(buildPdfPage(canvasImg, imgSize, rects))
		}
		return container.NewVScroll(pages)
```

- [ ] **Step 4: Build and vet**

Run: `go build ./... && go vet ./internal/ui/...`
Expected: PASS — no unused imports, no errors. (`image/color` is used by `buildPdfPage`; `fmt` by `highlightValues`; `path/filepath` and `strings` by `renderPreviewContent`.)

---

### Task 5: Fenster zweiteilen (HSplit Formular + Vorschau)

**Files:**
- Modify: `internal/ui/invoicemodal.go`

- [ ] **Step 1: Narrow the form scroll min size**

In `internal/ui/invoicemodal.go`, find:

```go
	// Scroll container for long forms
	scrollForm := container.NewVScroll(form)
	scrollForm.SetMinSize(fyne.NewSize(700, 400))
```

Change the min size so the form fits in the left third of the window:

```go
	// Scroll container for long forms
	scrollForm := container.NewVScroll(form)
	scrollForm.SetMinSize(fyne.NewSize(420, 400))
```

- [ ] **Step 2: Put form and preview side by side**

In `internal/ui/invoicemodal.go`, find:

```go
	confirmWin.SetContent(container.NewBorder(nil, buttonBar, nil, nil, scrollForm))
	confirmWin.Resize(fyne.NewSize(1150, 850))
```

Replace with:

```go
	preview := buildDocumentPreview(originalPath, meta)
	split := container.NewHSplit(scrollForm, preview)
	split.SetOffset(0.33) // form ~1/3, preview ~2/3

	confirmWin.SetContent(container.NewBorder(nil, buttonBar, nil, nil, split))
	confirmWin.Resize(fyne.NewSize(1500, 850))
```

- [ ] **Step 3: Build, vet, test**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS — build and vet clean; `internal/core` tests pass; other packages report `no test files`.

---

### Task 6: Build, Paketierung, Auslieferung, Smoke-Test

**Files:** none (build/deploy only)

- [ ] **Step 1: Final build + vet + tests**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: all succeed.

- [ ] **Step 2: Package the Windows executable**

Run (from `C:\Users\istok\Desktop\Dev\BuchISY`):
`fyne package -os windows -name BuchISY -src ./cmd/buchisy`
Expected: `cmd/buchisy/BuchISY.exe` produced. Note: the .exe is now noticeably larger (MuPDF is bundled).

- [ ] **Step 3: Stop the running app**

Terminate any running `BuchISY` process (PowerShell `wmic process where "ProcessId=<id>" call terminate` per running PID, as established in this session).

- [ ] **Step 4: Deploy and restart**

Copy `cmd/buchisy/BuchISY.exe` over `C:\Users\istok\Desktop\BuchISY.exe`, then launch
`C:\Users\istok\Desktop\BuchISY.exe` with working directory `C:\Users\istok\Desktop`.

- [ ] **Step 5: Manual smoke test**

In the running app, file an invoice and check the "Rechnungsdaten prüfen" window:
1. The window is split — form on the left (~1/3 wide), preview on the right.
2. A PDF invoice: all pages render and scroll vertically; extracted values
   (company, invoice number, amounts) appear yellow-highlighted where they
   occur verbatim in the document.
3. An image file as main file: the image is shown.
4. An Office file as main file: the placeholder text appears.
5. The split divider can be dragged; the window can be resized.

---

## Self-Review

**Spec coverage:**
- Fenster zweigeteilt, Felder ~1/3 → Task 5 (`HSplit`, offset 0.33, `scrollForm` min width 420).
- PDF alle Seiten gerendert, scrollbar → Task 4 (`renderPreviewContent` PDF-Zweig, `VBox` in `VScroll`).
- Bilddateien direkt → Task 4 (`imagePreviewExtensions`-Zweig).
- Office-Platzhalter → Task 4 (`previewPlaceholder`).
- PDF-Rendering via go-fitz → Task 1 (`RenderPDF`).
- Gelbe Markierung, Best-Effort, PDF-only → Tasks 2 & 3 (`valueBoxInRow`, `pdfBoxToPixel`, `HighlightRects`) + Task 4 (`buildPdfPage` Overlay).
- Koordinaten-Umrechnung mit Y-Spiegelung → Task 2 (`pdfBoxToPixel`, getestet).
- Hintergrund-Rendering, „wird geladen" → Task 4 (`buildDocumentPreview` goroutine + Platzhalterlabel).
- go.mod-Eintrag → Task 1.
- Unit-Tests Koordinaten + Wert-Suche → Task 2.

**Placeholder scan:** Der einzige bewusste Zwischenstand ist der `buildPdfPage`-Stub in Task 4 Step 1, der in Step 2 vollständig ersetzt wird (Grund inline genannt) — kein offener Platzhalter im Endzustand.

**Type consistency:** `core.Rect{X,Y,W,H float32}` in Task 2 definiert, in Tasks 3/4 verwendet. `RenderPDF(string, float64) ([]image.Image, error)` (Task 1) — in Task 4 als `core.RenderPDF` aufgerufen. `HighlightRects(string, []string, float64) ([][]Rect, error)` (Task 3) — in Task 4 als `core.HighlightRects` aufgerufen, gleiches `previewDPI`. `buildPdfPage(*canvas.Image, fyne.Size, []core.Rect)` — Signatur in Task 4 Step 2 definiert, in Step 3 mit genau diesen Argumenten aufgerufen. `previewDPI` ist die einzige DPI-Quelle für Render und Highlight.
