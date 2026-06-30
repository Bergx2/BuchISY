package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// tooltipButton is an icon button that shows a small help popup while hovered,
// explaining what the icon does (Fyne 2.6's widget.Button has no native
// tooltip). Used for the compact upload-card icons.
type tooltipButton struct {
	widget.Button
	tip    string
	canvas fyne.Canvas
	popup  *widget.PopUp
}

// newTooltipButton builds a low-importance icon button with a hover tooltip.
func newTooltipButton(icon fyne.Resource, tip string, canvas fyne.Canvas, tapped func()) *tooltipButton {
	b := &tooltipButton{tip: tip, canvas: canvas}
	b.ExtendBaseWidget(b)
	b.Icon = icon
	b.OnTapped = tapped
	b.Importance = widget.LowImportance
	return b
}

func (b *tooltipButton) MouseIn(e *desktop.MouseEvent) {
	b.Button.MouseIn(e)
	if b.tip == "" || b.canvas == nil {
		return
	}
	lbl := widget.NewLabel(b.tip)
	lbl.Wrapping = fyne.TextWrapOff
	b.popup = widget.NewPopUp(container.NewPadded(lbl), b.canvas)
	pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(b)
	pos.Y += b.Size().Height + 4
	b.popup.ShowAtPosition(pos)
}

func (b *tooltipButton) MouseOut() {
	b.Button.MouseOut()
	if b.popup != nil {
		b.popup.Hide()
		b.popup = nil
	}
}
