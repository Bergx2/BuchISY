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

// showUStVADialog shows the VAT return for the current month or whole year:
// output VAT owed (Umsatzsteuer, incl. §13b), deductible input VAT (Vorsteuer,
// incl. §13b) and the resulting Zahllast.
func (a *App) showUStVADialog() {
	yearMode := false
	body := container.NewVBox()
	scroll := container.NewVScroll(body)

	fmtAmt := func(v float64) string {
		return strings.Replace(fmt.Sprintf("%.2f", v), ".", ",", 1)
	}
	addSection := func(headingKey string, zeilen []core.UStVAZeile, total float64) {
		body.Add(widget.NewLabelWithStyle(a.bundle.T(headingKey), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
		for _, z := range zeilen {
			body.Add(widget.NewLabel(fmt.Sprintf("    %d %%   (%d)   %s €", z.Satz, z.Konto, fmtAmt(z.Betrag))))
		}
		if len(zeilen) == 0 {
			body.Add(widget.NewLabel("    —"))
		}
		body.Add(widget.NewLabel("    " + a.bundle.T("ustva.summe", fmtAmt(total))))
		body.Add(widget.NewSeparator())
	}

	reload := func() {
		fromY, fromM, toY, toM := a.currentYear, int(a.currentMonth), a.currentYear, int(a.currentMonth)
		if yearMode {
			fromM, toM = 1, 12
		}
		rows := a.collectInvoiceRows(fromY, fromM, toY, toM)
		u := core.ComputeUStVA(rows, a.bookingRules)

		body.Objects = nil
		addSection("ustva.umsatzsteuer.heading", u.Umsatzsteuer, u.UmsatzsteuerGesamt)
		addSection("ustva.vorsteuer.heading", u.Vorsteuer, u.VorsteuerGesamt)

		zKey, zVal := "ustva.zahllast", u.Zahllast
		if u.Zahllast < 0 {
			zKey, zVal = "ustva.ueberschuss", -u.Zahllast
		}
		body.Add(widget.NewLabelWithStyle(a.bundle.T(zKey, fmtAmt(zVal)), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
		body.Refresh()
	}

	toggle := widget.NewRadioGroup([]string{a.bundle.T("export.month"), a.bundle.T("export.year")}, func(sel string) {
		yearMode = sel == a.bundle.T("export.year")
		reload()
	})
	toggle.Horizontal = true
	toggle.SetSelected(a.bundle.T("export.month"))
	reload()

	header := widget.NewLabelWithStyle(a.bundle.T("ustva.heading"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	content := container.NewBorder(container.NewVBox(header, toggle), nil, nil, nil, scroll)
	d := dialog.NewCustom(a.bundle.T("ustva.title"), a.bundle.T("common.close"), content, a.window)
	d.Resize(fyne.NewSize(480, 480))
	d.Show()
}
