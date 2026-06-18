package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"github.com/bergx2/buchisy/internal/i18n"
)

// formField pairs a label string with its content widget. Used by
// selectableForm to render copyable labels.
type formField struct {
	Label   string
	Content fyne.CanvasObject
}

// fi is shorthand to construct a formField from a label + content.
// Keeps call sites readable when several rows are listed in sequence.
func fi(label string, content fyne.CanvasObject) formField {
	return formField{Label: label, Content: content}
}

// selectableForm builds a form-layout grid (label | content) where each
// label cell is a copyableLabel — right-click → "Kopieren" — instead
// of widget.Form's non-selectable internal label. Visual matches the
// stock Form (labels right-aligned via the same FormLayout that
// widget.Form uses internally).
func selectableForm(bundle *i18n.Bundle, items ...formField) fyne.CanvasObject {
	objs := make([]fyne.CanvasObject, 0, len(items)*2)
	for _, it := range items {
		var labelObj fyne.CanvasObject
		if it.Label == "" {
			labelObj = widget.NewLabel("") // spacer column
		} else {
			lbl := newCopyableLabel(bundle, it.Label)
			lbl.Alignment = fyne.TextAlignTrailing
			labelObj = lbl
		}
		objs = append(objs, labelObj, it.Content)
	}
	return container.New(layout.NewFormLayout(), objs...)
}
