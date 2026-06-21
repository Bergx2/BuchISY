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

// showControllingDialog shows per-account booked sums for the current month or
// the whole current year, toggled by a segmented control.
func (a *App) showControllingDialog() {
	var sums []core.AccountSum
	var total float64
	yearMode := false

	list := widget.NewList(
		func() int { return len(sums) },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			s := sums[i]
			amount := strings.Replace(fmt.Sprintf("%.2f", s.Summe), ".", ",", 1)
			o.(*widget.Label).SetText(fmt.Sprintf("%d  %s   %s", s.Konto, s.Name, amount))
		},
	)
	totalLabel := widget.NewLabel("")

	reload := func() {
		fromY, fromM, toY, toM := a.currentYear, int(a.currentMonth), a.currentYear, int(a.currentMonth)
		if yearMode {
			fromM, toM = 1, 12
		}
		rows := a.collectInvoiceRows(fromY, fromM, toY, toM)
		sums, total = core.AggregateBookingsByAccount(rows, a.chart)
		amount := strings.Replace(fmt.Sprintf("%.2f", total), ".", ",", 1)
		totalLabel.SetText(a.bundle.T("controlling.total", amount))
		list.Refresh()
	}

	toggle := widget.NewRadioGroup([]string{a.bundle.T("export.month"), a.bundle.T("export.year")}, func(sel string) {
		yearMode = sel == a.bundle.T("export.year")
		reload()
	})
	toggle.Horizontal = true
	toggle.SetSelected(a.bundle.T("export.month"))
	reload()

	header := widget.NewLabelWithStyle(a.bundle.T("controlling.heading"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	content := container.NewBorder(container.NewVBox(header, toggle), totalLabel, nil, nil, list)
	d := dialog.NewCustom(a.bundle.T("controlling.title"), a.bundle.T("common.close"), content, a.window)
	d.Resize(fyne.NewSize(520, 480))
	d.Show()
}
