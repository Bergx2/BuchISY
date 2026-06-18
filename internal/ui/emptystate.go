package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// emptyState renders a friendly, centered "nothing here yet" panel
// with an icon, a headline, a subtitle and (optionally) a call-to-
// action button. Use instead of a bare widget.NewLabel("…") wherever
// a list / preview / table is empty.
func emptyState(icon fyne.Resource, title, hint string, cta *widget.Button) fyne.CanvasObject {
	iconObj := widget.NewIcon(icon)
	titleLbl := widget.NewLabelWithStyle(title,
		fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	hintLbl := widget.NewLabelWithStyle(hint,
		fyne.TextAlignCenter, fyne.TextStyle{})
	hintLbl.Wrapping = fyne.TextWrapWord

	// Bigger icon than the default 16×16.
	iconWrap := container.New(fixedWidthLayout{width: theme.IconInlineSize() * 3}, iconObj)
	iconCenter := container.NewCenter(iconWrap)

	items := []fyne.CanvasObject{iconCenter, titleLbl, hintLbl}
	if cta != nil {
		items = append(items, container.NewCenter(cta))
	}
	box := container.NewVBox(items...)

	// Subtle card background so the empty state reads as deliberate.
	bg := canvas.NewRectangle(cardBackgroundColor())
	bg.StrokeColor = theme.Color(theme.ColorNameInputBorder)
	bg.StrokeWidth = 1
	bg.CornerRadius = 8

	card := container.NewStack(bg,
		container.NewPadded(container.NewPadded(box)))

	// Constrain to a sensible width then centre it within the parent.
	bounded := container.New(fixedWidthLayout{width: 360}, card)
	return container.NewCenter(bounded)
}
