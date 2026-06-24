package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// showAutoRulesDialog lists all learned booking templates and lets the user
// enable/disable the Autobook flag per supplier. When Autobook is on, matching
// invoices are booked silently without the confirmation modal.
func (a *App) showAutoRulesDialog() {
	entries := a.bookingTemplates.List()

	win := a.app.NewWindow(a.bundle.T("autorules.title"))

	// Warning banner: make the opt-in consequence explicit.
	warnLabel := widget.NewLabelWithStyle(
		a.bundle.T("autorules.warn"),
		fyne.TextAlignLeading,
		fyne.TextStyle{Bold: true},
	)
	warnLabel.Importance = widget.WarningImportance
	warnLabel.Wrapping = fyne.TextWrapWord

	if len(entries) == 0 {
		msg := widget.NewLabel("Noch keine Buchungs-Regeln gelernt.\nBuche Rechnungen über das Modal — die Regeln werden dabei automatisch gespeichert.")
		msg.Wrapping = fyne.TextWrapWord
		content := container.NewVBox(warnLabel, msg)
		win.SetContent(container.NewPadded(content))
		win.Resize(fyne.NewSize(560, 220))
		win.CenterOnScreen()
		win.Show()
		return
	}

	// Header row.
	header := container.NewGridWithColumns(4,
		widget.NewLabelWithStyle(a.bundle.T("autorules.col.supplier"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabelWithStyle(a.bundle.T("autorules.col.konto"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabelWithStyle(a.bundle.T("autorules.col.kategorie"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabelWithStyle(a.bundle.T("autorules.col.autobook"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
	)

	rows := make([]fyne.CanvasObject, 0, len(entries))
	for _, e := range entries {
		company := e.Company
		tpl := e.Tpl

		// Konto label.
		kontoText := fmt.Sprintf("%d", tpl.ExpenseKonto)
		if acc, ok := a.chart.Find(tpl.ExpenseKonto); ok {
			kontoText = accountLabel(acc)
		}

		companyLbl := widget.NewLabel(company)
		companyLbl.Wrapping = fyne.TextWrapWord
		kontoLbl := widget.NewLabel(kontoText)
		kontoLbl.Wrapping = fyne.TextWrapWord
		kategorieLbl := widget.NewLabel(tpl.Kategorie)

		check := widget.NewCheck("", nil)
		check.SetChecked(tpl.Autobook)
		check.OnChanged = func(checked bool) {
			tpl.Autobook = checked
			if err := a.bookingTemplates.Set(company, tpl); err != nil {
				a.logger.Warn("autorulesview: failed to save template for %s: %v", company, err)
			}
		}

		rows = append(rows, container.NewGridWithColumns(4,
			companyLbl, kontoLbl, kategorieLbl, check,
		))
	}

	list := container.NewVBox(rows...)
	scroll := container.NewVScroll(list)
	scroll.SetMinSize(fyne.NewSize(600, 300))

	content := container.NewVBox(
		warnLabel,
		widget.NewSeparator(),
		header,
		widget.NewSeparator(),
		scroll,
	)
	win.SetContent(container.NewPadded(content))
	win.Resize(fyne.NewSize(700, 450))
	win.CenterOnScreen()
	win.Show()
}
