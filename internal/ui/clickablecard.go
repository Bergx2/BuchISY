package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// clickableCard wraps any content and makes the whole area tappable
// (with pointer cursor). Used for profile/account cards where the
// entire visual block should behave like a single big button without
// looking like a stock widget.NewButton.
type clickableCard struct {
	widget.BaseWidget
	content fyne.CanvasObject
	onTap   func()
}

func newClickableCard(content fyne.CanvasObject, onTap func()) *clickableCard {
	c := &clickableCard{content: content, onTap: onTap}
	c.ExtendBaseWidget(c)
	return c
}

func (c *clickableCard) Tapped(_ *fyne.PointEvent) {
	if c.onTap != nil {
		c.onTap()
	}
}

func (c *clickableCard) Cursor() desktop.Cursor { return desktop.PointerCursor }

func (c *clickableCard) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(c.content)
}
