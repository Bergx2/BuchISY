package ui

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"

	// Register image decoders so image.Decode can read the formats the
	// app accepts (PDF preview path already handles PDFs separately).
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"

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

// buildDocumentPreview returns a preview panel for the main file, plus
// the underlying preview strip (non-nil for PDFs/images, used by the
// host window's Ctrl+scroll wiring). Renders synchronously: blocks the
// calling thread for a few hundred ms while MuPDF / the image decoder
// run, but eliminates an async-swap race where a fyne.DoAndWait
// callback occasionally left the preview pane blank on Windows.
//
// Returns a plain *fyne.Container so HSplit (and any other Fyne layout)
// sees the standard concrete type — a previous custom-struct wrapper
// embedding *fyne.Container appears to have confused some renderers.
func buildDocumentPreview(mainPath string, meta core.Meta) (*fyne.Container, *pdfPreviewStrip) {
	content, strip := renderPreviewContent(mainPath, meta, hlYellowFill)
	return container.NewStack(content), strip
}

// buildPreviewToolbar assembles the zoom + page-navigation controls
// shown above a pdfPreviewStrip. Hides the page controls when the
// document is a single page (image or one-page PDF).
func buildPreviewToolbar(strip *pdfPreviewStrip) fyne.CanvasObject {
	zoomOut := widget.NewButton("−", func() { strip.setZoom(strip.zoom - previewZoomStep) })
	zoomReset := widget.NewButton("100%", func() { strip.setZoom(1.0) })
	zoomIn := widget.NewButton("+", func() { strip.setZoom(strip.zoom + previewZoomStep) })

	prevBtn := widget.NewButton("◀", nil)
	nextBtn := widget.NewButton("▶", nil)
	pageLabel := widget.NewLabel("")

	updateLabel := func() {
		total := strip.totalPages()
		if total <= 0 {
			pageLabel.SetText("Seite: –")
			prevBtn.Disable()
			nextBtn.Disable()
			return
		}
		cur := strip.currentPage()
		pageLabel.SetText(fmt.Sprintf("Seite %d / %d", cur+1, total))
		if cur > 0 {
			prevBtn.Enable()
		} else {
			prevBtn.Disable()
		}
		if cur < total-1 {
			nextBtn.Enable()
		} else {
			nextBtn.Disable()
		}
	}

	prevBtn.OnTapped = func() {
		strip.scrollToPage(strip.currentPage() - 1)
		updateLabel()
	}
	nextBtn.OnTapped = func() {
		strip.scrollToPage(strip.currentPage() + 1)
		updateLabel()
	}

	// Update when the user scrolls manually or after re-layout (zoom).
	if strip.scroll != nil {
		strip.scroll.OnScrolled = func(_ fyne.Position) { updateLabel() }
	}
	strip.onPagesLaidOut = updateLabel
	updateLabel()

	// Page label is always visible (also for single-page docs — gives
	// users a clear "Seite 1 / 1" so they know the doc isn't truncated).
	// Prev/next arrows are still hidden when there's nothing to flip to.
	if strip.totalPages() <= 1 {
		prevBtn.Hide()
		nextBtn.Hide()
	}

	return container.NewHBox(zoomOut, zoomReset, zoomIn,
		widget.NewSeparator(), prevBtn, pageLabel, nextBtn)
}

// renderPreviewContent builds the preview content and, for PDFs, the strip.
// previewHighlight styles the rectangles drawn over matched values in the
// preview. The default is a soft yellow fill (receipts); a green frame is used
// for a linked bank statement so the matched booking stands out.
type previewHighlight struct {
	fill        color.NRGBA
	stroke      color.NRGBA
	strokeWidth float32
	fullWidth   bool     // expand each rect to the full page width (frame the whole row)
	blockMode   bool     // frame the whole statement booking block (all its lines), not one row
	values      []string // when set, search ONLY these (instead of all meta values)
}

var (
	hlYellowFill = previewHighlight{fill: color.NRGBA{R: 255, G: 235, B: 0, A: 90}}
	hlGreenFrame = previewHighlight{fill: color.NRGBA{R: 0, G: 210, B: 0, A: 55}, stroke: color.NRGBA{R: 0, G: 150, B: 0, A: 255}, strokeWidth: 3, fullWidth: true, blockMode: true}
)

func renderPreviewContent(mainPath string, meta core.Meta, hl previewHighlight) (fyne.CanvasObject, *pdfPreviewStrip) {
	if core.IsPDF(mainPath) {
		if !core.FileExists(mainPath) {
			return previewPlaceholder(mainPath, "PDF nicht gefunden:\n"+mainPath), nil
		}
		images, err := core.RenderPDF(mainPath, previewDPI)
		if err != nil {
			return previewPlaceholder(mainPath, "PDF-Render-Fehler:\n"+err.Error()), nil
		}
		if len(images) == 0 {
			return previewPlaceholder(mainPath, "PDF ohne Seiten"), nil
		}
		searchVals := highlightValues(meta)
		if hl.values != nil {
			searchVals = hl.values // statement: only the booking amount, so a single line is framed
		}
		var rectsPerPage [][]core.Rect
		if hl.blockMode {
			// Statement: frame the entire booking block (date row + detail rows).
			rectsPerPage, _ = core.StatementBlockRects(mainPath, searchVals, previewDPI)
		} else {
			rectsPerPage, _ = core.HighlightRects(mainPath, searchVals, previewDPI)
		}

		// Frame the whole booking (not just the matched value): widen each rect to
		// the full page width and pad it vertically. A single matched row needs
		// generous padding; a whole block is already tall, so pad it only lightly.
		if hl.fullWidth {
			padFrac := float32(0.30)
			if hl.blockMode {
				padFrac = 0.06
			}
			for pi := range rectsPerPage {
				if pi >= len(images) {
					continue
				}
				pw := float32(images[pi].Bounds().Dx())
				margin := pw * 0.03
				for j := range rectsPerPage[pi] {
					pad := rectsPerPage[pi][j].H * padFrac
					rectsPerPage[pi][j].X = margin
					rectsPerPage[pi][j].W = pw - 2*margin
					rectsPerPage[pi][j].Y -= pad
					rectsPerPage[pi][j].H += 2 * pad
				}
			}
		}

		strip := newPdfPreviewStrip(images, rectsPerPage, hl)
		scroll := container.NewScroll(strip)
		strip.scroll = scroll
		strip.onWidthChange = scroll.Refresh
		toolbar := buildPreviewToolbar(strip)

		return container.NewBorder(toolbar, nil, nil, nil, scroll), strip
	}

	if _, ok := imagePreviewExtensions[strings.ToLower(filepath.Ext(mainPath))]; ok {
		// Decode the image up-front and reuse the existing PDF preview
		// strip with a single page — gives images the same zoom buttons,
		// Ctrl+wheel zoom, and horizontal/vertical scrolling.
		f, err := os.Open(mainPath)
		if err != nil {
			return previewPlaceholder(mainPath, "Bild kann nicht geöffnet werden"), nil
		}
		defer f.Close()
		img, _, err := image.Decode(f)
		if err != nil {
			return previewPlaceholder(mainPath, "Bild kann nicht dekodiert werden"), nil
		}

		strip := newPdfPreviewStrip([]image.Image{img}, [][]core.Rect{nil}, hl)
		scroll := container.NewScroll(strip)
		strip.scroll = scroll
		strip.onWidthChange = scroll.Refresh
		toolbar := buildPreviewToolbar(strip)

		return container.NewBorder(toolbar, nil, nil, nil, scroll), strip
	}

	return previewPlaceholder(mainPath, "Keine Vorschau verfügbar"), nil
}

// previewPlaceholder is the centered fallback panel. Uses no text
// wrapping + ellipsis truncation, so a narrow preview pane truncates
// the message gracefully instead of stacking each character on its
// own line.
func previewPlaceholder(mainPath, message string) fyne.CanvasObject {
	msg := widget.NewLabel(message)
	msg.Alignment = fyne.TextAlignCenter
	msg.Wrapping = fyne.TextWrapOff
	msg.Truncation = fyne.TextTruncateEllipsis
	name := widget.NewLabel(filepath.Base(mainPath))
	name.Alignment = fyne.TextAlignCenter
	name.Wrapping = fyne.TextWrapOff
	name.Truncation = fyne.TextTruncateEllipsis
	box := container.NewVBox(msg, widget.NewSeparator(), name)
	return container.NewPadded(container.NewCenter(box))
}

// highlightValues collects the extracted values to look for in the document.
func highlightValues(meta core.Meta) []string {
	vals := []string{meta.Auftraggeber, meta.Rechnungsnummer, meta.Waehrung}
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
	pageImgs       []*canvas.Image
	pageNative     []fyne.Size
	rects          [][]core.Rect
	rectObjs       [][]*canvas.Rectangle
	pageOffsets    []float32 // Y of each page's top, updated by Layout
	lastWidth      float32
	zoom           float32
	baseWidth      float32
	scroll         *container.Scroll
	ctrlHeld       bool
	onWidthChange  func()
	onPagesLaidOut func() // fired after Layout re-computes pageOffsets
}

// totalPages returns the number of pages in this strip.
func (s *pdfPreviewStrip) totalPages() int { return len(s.pageImgs) }

// currentPage returns the 0-based index of the page whose top is at or
// just above the current scroll position. 0 when there's no scroll.
func (s *pdfPreviewStrip) currentPage() int {
	if s.scroll == nil || len(s.pageOffsets) == 0 {
		return 0
	}
	// Small bias so a partially visible page above doesn't count.
	y := s.scroll.Offset.Y + 1
	cur := 0
	for i, off := range s.pageOffsets {
		if off > y {
			break
		}
		cur = i
	}
	return cur
}

// scrollToPage moves the enclosing scroll container so the given
// page's top aligns with the viewport's top.
func (s *pdfPreviewStrip) scrollToPage(idx int) {
	if s.scroll == nil || idx < 0 || idx >= len(s.pageOffsets) {
		return
	}
	s.scroll.ScrollToOffset(fyne.NewPos(0, s.pageOffsets[idx]))
}

func newPdfPreviewStrip(images []image.Image, rectsPerPage [][]core.Rect, hl previewHighlight) *pdfPreviewStrip {
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
			r := canvas.NewRectangle(hl.fill)
			if hl.strokeWidth > 0 {
				r.StrokeColor = hl.stroke
				r.StrokeWidth = hl.strokeWidth
			}
			objs[j] = r
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

// Scrolled always zooms — user preference is plain wheel = zoom. The
// modal's Ctrl+wheel overlay still routes here when Ctrl is held, so
// either gesture works.
func (s *pdfPreviewStrip) Scrolled(ev *fyne.ScrollEvent) {
	if ev.Scrolled.DY > 0 {
		s.setZoom(s.zoom + previewZoomStep)
	} else if ev.Scrolled.DY < 0 {
		s.setZoom(s.zoom - previewZoomStep)
	}
}

// Dragged pans the enclosing scroll viewport by the drag delta, so the
// user can grab the preview and move it around like a map. No-op when
// the preview fits entirely inside the viewport (scroll clamps to its
// own bounds).
func (s *pdfPreviewStrip) Dragged(ev *fyne.DragEvent) {
	if s.scroll == nil {
		return
	}
	pos := s.scroll.Offset
	pos.X -= ev.Dragged.DX
	pos.Y -= ev.Dragged.DY
	s.scroll.ScrollToOffset(pos)
}

// DragEnd is the required other half of the fyne.Draggable interface.
func (s *pdfPreviewStrip) DragEnd() {}

// Cursor returns the pointer cursor so users know the preview is
// grab-able. Fyne doesn't ship a dedicated "grab" cursor variant.
func (s *pdfPreviewStrip) Cursor() desktop.Cursor {
	return desktop.PointerCursor
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
	s.pageOffsets = s.pageOffsets[:0]
	y := float32(0)
	for i, img := range s.pageImgs {
		native := s.pageNative[i]
		if native.Width <= 0 {
			s.pageOffsets = append(s.pageOffsets, y)
			continue
		}
		scale := size.Width / native.Width
		pageH := native.Height * scale
		s.pageOffsets = append(s.pageOffsets, y)
		img.Resize(fyne.NewSize(size.Width, pageH))
		img.Move(fyne.NewPos(0, y))
		for j, rc := range s.rects[i] {
			ro := s.rectObjs[i][j]
			ro.Resize(fyne.NewSize(rc.W*scale, rc.H*scale))
			ro.Move(fyne.NewPos(rc.X*scale, y+rc.Y*scale))
		}
		y += pageH + pdfPreviewPageGap
	}
	if s.onPagesLaidOut != nil {
		s.onPagesLaidOut()
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

// addHighlight appends a highlight rectangle to the given page of the strip
// and triggers a refresh. Must be called on the main (UI) thread.
// page is 0-based; rc is in native image pixels (matching pageNative).
func (s *pdfPreviewStrip) addHighlight(page int, rc core.Rect, hl previewHighlight) {
	if page < 0 || page >= len(s.rects) {
		return
	}
	s.rects[page] = append(s.rects[page], rc)
	r := canvas.NewRectangle(hl.fill)
	if hl.strokeWidth > 0 {
		r.StrokeColor = hl.stroke
		r.StrokeWidth = hl.strokeWidth
	}
	s.rectObjs[page] = append(s.rectObjs[page], r)
	s.Refresh()
}
