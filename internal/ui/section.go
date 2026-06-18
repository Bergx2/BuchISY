package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// section wraps content in a subtle card with a bold heading on top.
// Used to group related form fields ("Identifikation", "Beträge",
// "Ablage") so long dialogs don't read as a wall of inputs.
func section(title string, content fyne.CanvasObject) fyne.CanvasObject {
	header := widget.NewLabelWithStyle(title,
		fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

	bg := canvas.NewRectangle(cardBackgroundColor())
	bg.StrokeColor = theme.Color(theme.ColorNameInputBorder)
	bg.StrokeWidth = 1
	bg.CornerRadius = 6

	body := container.NewPadded(container.NewVBox(header,
		widget.NewSeparator(),
		content,
	))
	return container.NewStack(bg, body)
}
