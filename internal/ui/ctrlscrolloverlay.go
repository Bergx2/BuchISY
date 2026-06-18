package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
)

// ctrlScrollOverlay is a fully transparent canvas object that only
// implements fyne.Scrollable. While added to the canvas' OverlayStack
// it intercepts mouse-wheel events (which Fyne's driver routes to the
// topmost Scrollable at the cursor position). It deliberately does not
// implement Tappable or Hoverable, so clicks and hover effects fall
// through to the widgets underneath.
type ctrlScrollOverlay struct {
	widget.BaseWidget
	onScroll func(*fyne.ScrollEvent)
	bg       *canvas.Rectangle
}

func newCtrlScrollOverlay(onScroll func(*fyne.ScrollEvent)) *ctrlScrollOverlay {
	o := &ctrlScrollOverlay{
		onScroll: onScroll,
		bg:       canvas.NewRectangle(color.Transparent),
	}
	o.ExtendBaseWidget(o)
	return o
}

// Scrolled implements fyne.Scrollable.
func (o *ctrlScrollOverlay) Scrolled(ev *fyne.ScrollEvent) {
	if o.onScroll != nil {
		o.onScroll(ev)
	}
}

func (o *ctrlScrollOverlay) CreateRenderer() fyne.WidgetRenderer {
	return &ctrlScrollOverlayRenderer{overlay: o}
}

type ctrlScrollOverlayRenderer struct {
	overlay *ctrlScrollOverlay
}

func (r *ctrlScrollOverlayRenderer) Layout(size fyne.Size) {
	r.overlay.bg.Resize(size)
}

func (r *ctrlScrollOverlayRenderer) MinSize() fyne.Size {
	return fyne.NewSize(0, 0)
}

func (r *ctrlScrollOverlayRenderer) Refresh() {
	r.overlay.bg.Refresh()
}

func (r *ctrlScrollOverlayRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.overlay.bg}
}

func (r *ctrlScrollOverlayRenderer) Destroy() {}
