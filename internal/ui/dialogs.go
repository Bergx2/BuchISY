package ui

import (
	"errors"

	"fyne.io/fyne/v2/dialog"
)

// showError displays an error dialog.
func (a *App) showError(title, message string) {
	dialog.ShowError(errors.New(message), a.window)
}

// showInfo displays an info dialog.
func (a *App) showInfo(title, message string) {
	dialog.ShowInformation(title, message, a.window)
}

// showConfirm displays a confirmation dialog.
func (a *App) showConfirm(title, message string, callback func(bool)) {
	dialog.ShowConfirm(title, message, callback, a.window)
}
