package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
)

// savePDF opens a file-save dialog and writes the given PDF bytes to the
// chosen location. Cancel is silently ignored; write errors are shown to the
// user. The helper mirrors the saveExportCSV pattern in csvexport.go.
func (a *App) savePDF(defaultName string, data []byte) {
	d := dialog.NewFileSave(func(w fyne.URIWriteCloser, err error) {
		if w == nil {
			return // user cancelled
		}
		defer w.Close()
		if err != nil {
			a.showError(a.bundle.T("error.processing.title"), err.Error())
			return
		}
		if _, werr := w.Write(data); werr != nil {
			a.showError(a.bundle.T("error.processing.title"), werr.Error())
		}
	}, a.window)
	d.SetFileName(defaultName)
	d.Show()
}
