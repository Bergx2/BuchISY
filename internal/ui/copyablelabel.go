package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"

	"github.com/bergx2/buchisy/internal/i18n"
)

// copyableLabel is a label with a right-click "Kopieren" context menu that
// copies the label's text to the clipboard. It embeds widget.Label, so
// Text, Wrapping and Alignment stay settable by callers.
type copyableLabel struct {
	widget.Label
	bundle *i18n.Bundle
}

// newCopyableLabel creates a copyable label showing the given text.
func newCopyableLabel(bundle *i18n.Bundle, text string) *copyableLabel {
	l := &copyableLabel{bundle: bundle}
	l.Text = text
	l.ExtendBaseWidget(l)
	return l
}

// TappedSecondary shows the "Kopieren" context menu on right-click.
// Tolerates a nil bundle (e.g. when used by helpers without easy i18n
// access) — falls back to the hardcoded German label.
func (l *copyableLabel) TappedSecondary(e *fyne.PointEvent) {
	canvas := fyne.CurrentApp().Driver().CanvasForObject(l)
	if canvas == nil {
		return
	}
	label := "Kopieren"
	if l.bundle != nil {
		label = l.bundle.T("menu.copy")
	}
	menu := fyne.NewMenu("",
		fyne.NewMenuItem(label, func() {
			fyne.CurrentApp().Clipboard().SetContent(l.Text)
		}),
	)
	widget.ShowPopUpMenuAtPosition(menu, canvas, e.AbsolutePosition)
}
