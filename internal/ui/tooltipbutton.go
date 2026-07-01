package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// tooltipButton is an icon button that shows a small help popup while hovered,
// explaining what the icon does (Fyne 2.6's widget.Button has no native
// tooltip). Used for the compact upload-card icons.
//
// The tooltip is rendered into a content-tree layer (App.tooltipLayer) rather
// than a widget.PopUp: a PopUp registers a full-canvas overlay, and Fyne routes
// the *next* click to that overlay instead of the button beneath it, so the
// icon would visibly highlight on hover but never fire on tap.
type tooltipButton struct {
	widget.Button
	tip  string
	show func(tip string, over fyne.CanvasObject)
	hide func()
}

// newTooltipButton builds a low-importance icon button with a hover tooltip.
// show/hide drive a shared, non-blocking tooltip layer (see App.showTooltip).
func newTooltipButton(icon fyne.Resource, tip string, show func(string, fyne.CanvasObject), hide func(), tapped func()) *tooltipButton {
	b := &tooltipButton{tip: tip, show: show, hide: hide}
	b.ExtendBaseWidget(b)
	b.Icon = icon
	b.OnTapped = tapped
	b.Importance = widget.LowImportance
	return b
}

func (b *tooltipButton) MouseIn(e *desktop.MouseEvent) {
	b.Button.MouseIn(e)
	if b.show != nil {
		b.show(b.tip, b)
	}
}

func (b *tooltipButton) MouseOut() {
	b.Button.MouseOut()
	if b.hide != nil {
		b.hide()
	}
}
