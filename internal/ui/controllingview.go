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

	table := widget.NewTable(
		func() (int, int) { return len(sums), 3 },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(id widget.TableCellID, o fyne.CanvasObject) {
			lbl := o.(*widget.Label)
			s := sums[id.Row]
			switch id.Col {
			case 0:
				lbl.SetText(fmt.Sprintf("%d", s.Konto))
			case 1:
				lbl.SetText(s.Name)
			default:
				lbl.Alignment = fyne.TextAlignTrailing
				lbl.SetText(strings.Replace(fmt.Sprintf("%.2f", s.Summe), ".", ",", 1))
			}
		},
	)
	table.SetColumnWidth(0, 70)
	table.SetColumnWidth(1, 300)
	table.SetColumnWidth(2, 110)
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
		table.Refresh()
	}

	toggle := widget.NewRadioGroup([]string{a.bundle.T("export.month"), a.bundle.T("export.year")}, func(sel string) {
		yearMode = sel == a.bundle.T("export.year")
		reload()
	})
	toggle.Horizontal = true
	toggle.SetSelected(a.bundle.T("export.month"))
	reload()

	header := widget.NewLabelWithStyle(a.bundle.T("controlling.heading"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	content := container.NewBorder(container.NewVBox(header, toggle), totalLabel, nil, nil, table)
	d := dialog.NewCustom(a.bundle.T("controlling.title"), a.bundle.T("common.close"), content, a.window)
	d.Resize(fyne.NewSize(520, 480))
	d.Show()
}
