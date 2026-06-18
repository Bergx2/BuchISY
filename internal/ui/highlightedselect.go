package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// highlightedSelect behaves like widget.Select but draws a thin border
// around the option whose text matches `highlight` when the popup opens.
// Used in the top-bar year/month pickers to mark today, so the user can
// still spot the current period after navigating to another month.
type highlightedSelect struct {
	widget.Select

	highlight string
}

func newHighlightedSelect(options []string, highlight string, onChange func(string)) *highlightedSelect {
	s := &highlightedSelect{highlight: highlight}
	s.Options = options
	s.OnChanged = onChange
	s.ExtendBaseWidget(s)
	return s
}

// Tapped replaces widget.Select's default popup with one that wraps the
// `highlight` option in a thin bordered rectangle.
func (s *highlightedSelect) Tapped(_ *fyne.PointEvent) {
	if s.Disabled() {
		return
	}
	c := fyne.CurrentApp().Driver().CanvasForObject(s)
	if c == nil {
		return
	}

	var popup *widget.PopUp
	rows := container.NewVBox()
	for _, opt := range s.Options {
		opt := opt
		btn := widget.NewButton(opt, func() {
			s.SetSelected(opt)
			popup.Hide()
		})
		btn.Alignment = widget.ButtonAlignLeading
		btn.Importance = widget.LowImportance

		var row fyne.CanvasObject = btn
		if opt == s.highlight {
			border := canvas.NewRectangle(color.Transparent)
			border.StrokeColor = theme.Color(theme.ColorNamePrimary)
			border.StrokeWidth = 1
			row = container.NewStack(border, btn)
		}
		rows.Add(row)
	}

	popup = widget.NewPopUp(rows, c)
	pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(s)
	pos.Y += s.Size().Height
	popup.ShowAtPosition(pos)
	popup.Resize(fyne.NewSize(s.Size().Width, popup.MinSize().Height))
}
