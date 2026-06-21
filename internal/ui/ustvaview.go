package ui

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/bergx2/buchisy/internal/core"
)

// showUStVADialog shows the deductible input VAT (Vorsteuer) per rate for the
// current month or whole year.
func (a *App) showUStVADialog() {
	var u core.UStVA
	yearMode := false

	list := widget.NewList(
		func() int { return len(u.Zeilen) },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			z := u.Zeilen[i]
			amount := strings.Replace(fmt.Sprintf("%.2f", z.Vorsteuer), ".", ",", 1)
			o.(*widget.Label).SetText(fmt.Sprintf("%s %d %%   (%d)   %s", a.bundle.T("ustva.vorsteuer"), z.Satz, z.Konto, amount))
		},
	)
	totalLabel := widget.NewLabel("")

	reload := func() {
		fromY, fromM, toY, toM := a.currentYear, int(a.currentMonth), a.currentYear, int(a.currentMonth)
		if yearMode {
			fromM, toM = 1, 12
		}
		rows := a.collectInvoiceRows(fromY, fromM, toY, toM)
		u = core.ComputeUStVA(rows, a.bookingRules)
		amount := strings.Replace(fmt.Sprintf("%.2f", u.VorsteuerGesamt), ".", ",", 1)
		totalLabel.SetText(a.bundle.T("ustva.total", amount))
		list.Refresh()
	}

	toggle := widget.NewRadioGroup([]string{a.bundle.T("export.month"), a.bundle.T("export.year")}, func(sel string) {
		yearMode = sel == a.bundle.T("export.year")
		reload()
	})
	toggle.Horizontal = true
	toggle.SetSelected(a.bundle.T("export.month"))
	reload()

	header := widget.NewLabelWithStyle(a.bundle.T("ustva.heading"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	content := container.NewBorder(container.NewVBox(header, toggle), totalLabel, nil, nil, list)
	d := dialog.NewCustom(a.bundle.T("ustva.title"), a.bundle.T("common.close"), content, a.window)
	d.Resize(fyne.NewSize(460, 380))
	d.Show()
}
