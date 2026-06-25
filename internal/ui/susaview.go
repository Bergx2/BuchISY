package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/bergx2/buchisy/internal/core"
)

// showSuSa displays the Summen-/Saldenliste for the current year.
func (a *App) showSuSa() {
	year := a.currentYear
	rows := a.collectInvoiceRows(year, 1, year, 12)
	bals := core.ComputeSuSa(rows, a.chart)

	fmtAmt := func(v float64) string {
		return formatMoney(v, "EUR", a.settings.DecimalSeparator)
	}

	headers := []string{
		a.bundle.T("susa.col.konto"),
		a.bundle.T("susa.col.name"),
		a.bundle.T("susa.col.soll"),
		a.bundle.T("susa.col.haben"),
		a.bundle.T("susa.col.saldo"),
	}

	// Compute totals
	var totalSoll, totalHaben, totalSaldo float64
	for _, b := range bals {
		totalSoll += b.SollSumme
		totalHaben += b.HabenSumme
		totalSaldo += b.Saldo
	}

	numDataRows := len(bals) + 1 // +1 for totals
	totalRows := numDataRows + 1  // +1 for header

	tbl := widget.NewTable(
		func() (int, int) { return totalRows, 5 },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(id widget.TableCellID, o fyne.CanvasObject) {
			lbl := o.(*widget.Label)
			lbl.TextStyle = fyne.TextStyle{}
			lbl.Alignment = fyne.TextAlignLeading

			// Header row
			if id.Row == 0 {
				lbl.TextStyle = fyne.TextStyle{Bold: true}
				if id.Col < len(headers) {
					lbl.SetText(headers[id.Col])
				}
				return
			}

			dataIdx := id.Row - 1

			// Totals row
			if dataIdx == len(bals) {
				lbl.TextStyle = fyne.TextStyle{Bold: true}
				switch id.Col {
				case 0:
					lbl.SetText(a.bundle.T("susa.total"))
				case 1:
					lbl.SetText("")
				case 2:
					lbl.Alignment = fyne.TextAlignTrailing
					lbl.SetText(fmtAmt(totalSoll))
				case 3:
					lbl.Alignment = fyne.TextAlignTrailing
					lbl.SetText(fmtAmt(totalHaben))
				case 4:
					lbl.Alignment = fyne.TextAlignTrailing
					lbl.SetText(fmtAmt(totalSaldo))
				}
				return
			}

			b := bals[dataIdx]
			switch id.Col {
			case 0:
				lbl.SetText(fmt.Sprintf("%d", b.Konto))
			case 1:
				lbl.SetText(b.Name)
			case 2:
				lbl.Alignment = fyne.TextAlignTrailing
				lbl.SetText(fmtAmt(b.SollSumme))
			case 3:
				lbl.Alignment = fyne.TextAlignTrailing
				lbl.SetText(fmtAmt(b.HabenSumme))
			case 4:
				lbl.Alignment = fyne.TextAlignTrailing
				lbl.SetText(fmtAmt(b.Saldo))
			}
		},
	)
	tbl.SetColumnWidth(0, 70)
	tbl.SetColumnWidth(1, 220)
	tbl.SetColumnWidth(2, 100)
	tbl.SetColumnWidth(3, 100)
	tbl.SetColumnWidth(4, 100)

	scroll := container.NewVScroll(tbl)
	scroll.SetMinSize(fyne.NewSize(640, 380))

	pdfBtn := widget.NewButton(a.bundle.T("report.pdf"), func() {
		title := fmt.Sprintf("%s %d", a.bundle.T("susa.title"), year)
		data, err := core.BuildSuSaPDF(bals, title, a.profile)
		if err != nil {
			a.showError(a.bundle.T("error.processing.title"), err.Error())
			return
		}
		a.savePDF(fmt.Sprintf("SuSa_%d.pdf", year), data)
	})

	header := container.NewBorder(nil, nil, nil, pdfBtn,
		widget.NewLabelWithStyle(
			fmt.Sprintf("%s %d", a.bundle.T("susa.title"), year),
			fyne.TextAlignLeading,
			fyne.TextStyle{Bold: true},
		),
	)

	content := container.NewBorder(header, nil, nil, nil, scroll)
	d := dialog.NewCustom(
		fmt.Sprintf("%s %d", a.bundle.T("susa.title"), year),
		a.bundle.T("common.close"),
		content,
		a.window,
	)
	d.Resize(fyne.NewSize(700, 520))
	d.Show()
}

// showGuV displays the Gewinn- und Verlustrechnung for the current year.
func (a *App) showGuV() {
	year := a.currentYear
	rows := a.collectInvoiceRows(year, 1, year, 12)
	bals := core.ComputeSuSa(rows, a.chart)
	g := core.ComputeGuV(bals, a.chart)

	fmtAmt := func(v float64) string {
		return formatMoney(v, "EUR", a.settings.DecimalSeparator)
	}

	body := container.NewVBox()

	addSection := func(labelKey string, items []core.AccountBalance, gesamt float64) {
		body.Add(widget.NewLabelWithStyle(
			a.bundle.T(labelKey),
			fyne.TextAlignLeading,
			fyne.TextStyle{Bold: true},
		))
		for _, b := range items {
			line := fmt.Sprintf("    %d  %s", b.Konto, b.Name)
			body.Add(newCopyableLabel(a.bundle, line))
		}
		total := widget.NewLabelWithStyle(
			fmt.Sprintf("    %s: %s", a.bundle.T("susa.total"), fmtAmt(gesamt)),
			fyne.TextAlignLeading,
			fyne.TextStyle{Bold: true},
		)
		body.Add(total)
		body.Add(widget.NewSeparator())
	}

	addSection("guv.erloese", g.ErloesPosten, g.ErloeseGesamt)
	addSection("guv.aufwand", g.AufwandPosten, g.AufwandGesamt)

	ergebnisLabel := fmt.Sprintf("%s: %s", a.bundle.T("guv.ergebnis"), fmtAmt(g.Ergebnis))
	body.Add(widget.NewLabelWithStyle(ergebnisLabel, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))

	scroll := container.NewVScroll(body)
	scroll.SetMinSize(fyne.NewSize(520, 380))

	pdfBtn := widget.NewButton(a.bundle.T("report.pdf"), func() {
		title := fmt.Sprintf("%s %d", a.bundle.T("guv.title"), year)
		data, err := core.BuildGuVPDF(g, title, a.profile)
		if err != nil {
			a.showError(a.bundle.T("error.processing.title"), err.Error())
			return
		}
		a.savePDF(fmt.Sprintf("GuV_%d.pdf", year), data)
	})

	header := container.NewBorder(nil, nil, nil, pdfBtn,
		widget.NewLabelWithStyle(
			fmt.Sprintf("%s %d", a.bundle.T("guv.title"), year),
			fyne.TextAlignLeading,
			fyne.TextStyle{Bold: true},
		),
	)

	content := container.NewBorder(header, nil, nil, nil, scroll)
	d := dialog.NewCustom(
		fmt.Sprintf("%s %d", a.bundle.T("guv.title"), year),
		a.bundle.T("common.close"),
		content,
		a.window,
	)
	d.Resize(fyne.NewSize(600, 520))
	d.Show()
}
