package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

// draggableRow is a list row whose vertical drag gestures are forwarded
// to its owner, so reorderable lists (the column-order list on the
// "Erweitert" settings page, the file picker's selection tray) can be
// reordered by dragging. The owner does the actual reordering and keeps
// the index field current. rowLabel/upBtn/downBtn are optional slots for
// owners that need to address per-row controls (the settings page does;
// the picker tray leaves them nil).
type draggableRow struct {
	widget.BaseWidget
	content   fyne.CanvasObject
	rowLabel  *widget.Label
	upBtn     *widget.Button
	downBtn   *widget.Button
	index     int
	dragAccum float32
	onDrag    func(row *draggableRow, dy float32)
	onDragEnd func(row *draggableRow)
}

func newDraggableRow(content fyne.CanvasObject, index int) *draggableRow {
	r := &draggableRow{content: content, index: index}
	r.ExtendBaseWidget(r)
	return r
}

func (r *draggableRow) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(r.content)
}

// Dragged implements fyne.Draggable.
func (r *draggableRow) Dragged(e *fyne.DragEvent) {
	if r.onDrag != nil {
		r.onDrag(r, e.Dragged.DY)
	}
}

// DragEnd implements fyne.Draggable.
func (r *draggableRow) DragEnd() {
	if r.onDragEnd != nil {
		r.onDragEnd(r)
	}
}
