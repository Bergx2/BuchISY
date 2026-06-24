package ui

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// showToastWithAction shows a non-modal floating notification at the bottom-right
// of the main window that includes a tappable action button and auto-dismisses
// after ~8 seconds (longer than showToast) so the user has time to act.
func (a *App) showToastWithAction(text, actionLabel string, action func()) {
	if a.window == nil {
		return
	}
	cv := a.window.Canvas()
	if cv == nil {
		return
	}

	lbl := widget.NewLabel(text)
	lbl.TextStyle = fyne.TextStyle{Bold: true}

	bg := canvas.NewRectangle(cardBackgroundColor())
	bg.StrokeColor = theme.Color(theme.ColorNamePrimary)
	bg.StrokeWidth = 2
	bg.CornerRadius = 8

	var pop *widget.PopUp

	btn := widget.NewButton(actionLabel, func() {
		if pop != nil {
			pop.Hide()
		}
		action()
	})

	row := container.NewHBox(lbl, btn)
	content := container.NewStack(bg, container.NewPadded(row))

	pop = widget.NewPopUp(content, cv)
	pop.Show()

	cs := cv.Size()
	ps := pop.MinSize()
	pop.Move(fyne.NewPos(cs.Width-ps.Width-16, cs.Height-ps.Height-48))

	go func() {
		time.Sleep(8000 * time.Millisecond)
		fyne.DoAndWait(func() { pop.Hide() })
	}()
}

// showToast shows a non-modal floating notification at the bottom-
// right of the main window that auto-dismisses after a short delay.
// Drop-in replacement for "Gespeichert"-style modal info dialogs —
// gives feedback without breaking the user's flow.
func (a *App) showToast(text string) {
	if a.window == nil {
		return
	}
	cv := a.window.Canvas()
	if cv == nil {
		return
	}

	lbl := widget.NewLabel(text)
	lbl.TextStyle = fyne.TextStyle{Bold: true}

	bg := canvas.NewRectangle(cardBackgroundColor())
	bg.StrokeColor = theme.Color(theme.ColorNamePrimary)
	bg.StrokeWidth = 2
	bg.CornerRadius = 8

	content := container.NewStack(bg, container.NewPadded(lbl))

	pop := widget.NewPopUp(content, cv)
	pop.Show()

	cs := cv.Size()
	ps := pop.MinSize()
	pop.Move(fyne.NewPos(cs.Width-ps.Width-16, cs.Height-ps.Height-48))

	go func() {
		time.Sleep(2500 * time.Millisecond)
		fyne.DoAndWait(func() { pop.Hide() })
	}()
}
