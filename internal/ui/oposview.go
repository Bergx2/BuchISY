package ui

import (
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/bergx2/buchisy/internal/core"
)

// showOpenItems displays the Offene-Posten-Liste (open receivables and payables)
// for the current year. Two tables are shown: Debitoren (Forderungen) and
// Kreditoren (Verbindlichkeiten), each with a totals line.
func (a *App) showOpenItems() {
	year := a.currentYear
	rows := a.collectInvoiceRows(year, 1, year, 12)
	oi := core.ComputeOpenItems(rows, time.Now())

	fmtAmt := func(v float64) string {
		return strings.Replace(fmt.Sprintf("%.2f", v), ".", ",", 1)
	}

	headers := []string{
		a.bundle.T("opos.col.belegnr"),
		a.bundle.T("opos.col.datum"),
		a.bundle.T("opos.col.partner"),
		a.bundle.T("opos.col.betrag"),
		a.bundle.T("opos.col.alter"),
		a.bundle.T("opos.col.bucket"),
	}

	// buildTable creates a widget.Table for a list of OpenItems plus a totals line.
	buildTable := func(items []core.OpenItem, gesamt float64) *widget.Table {
		numDataRows := len(items) + 1 // +1 for totals
		totalRows := numDataRows + 1  // +1 for header

		tbl := widget.NewTable(
			func() (int, int) { return totalRows, 6 },
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
				if dataIdx == len(items) {
					lbl.TextStyle = fyne.TextStyle{Bold: true}
					switch id.Col {
					case 0:
						lbl.SetText(a.bundle.T("opos.total"))
					case 3:
						lbl.Alignment = fyne.TextAlignTrailing
						lbl.SetText(fmtAmt(gesamt))
					default:
						lbl.SetText("")
					}
					return
				}

				item := items[dataIdx]
				switch id.Col {
				case 0:
					lbl.SetText(item.Belegnummer)
				case 1:
					lbl.SetText(item.Datum)
				case 2:
					lbl.SetText(item.Partner)
				case 3:
					lbl.Alignment = fyne.TextAlignTrailing
					lbl.SetText(fmtAmt(item.Betrag))
				case 4:
					lbl.Alignment = fyne.TextAlignTrailing
					lbl.SetText(fmt.Sprintf("%d", item.AgeDays))
				case 5:
					lbl.SetText(item.Bucket)
				}
			},
		)
		tbl.SetColumnWidth(0, 90)
		tbl.SetColumnWidth(1, 90)
		tbl.SetColumnWidth(2, 180)
		tbl.SetColumnWidth(3, 100)
		tbl.SetColumnWidth(4, 90)
		tbl.SetColumnWidth(5, 80)
		return tbl
	}

	debSection := container.NewVBox(
		widget.NewLabelWithStyle(a.bundle.T("opos.debitoren"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		buildTable(oi.Forderungen, oi.ForderungenGesamt),
	)

	kredSection := container.NewVBox(
		widget.NewLabelWithStyle(a.bundle.T("opos.kreditoren"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		buildTable(oi.Verbindlichkeiten, oi.VerbindlichkeitenGesamt),
	)

	scroll := container.NewVScroll(container.NewVBox(debSection, widget.NewSeparator(), kredSection))
	scroll.SetMinSize(fyne.NewSize(660, 400))

	pdfBtn := widget.NewButton(a.bundle.T("report.pdf"), func() {
		title := fmt.Sprintf("Offene Posten %d", year)
		data, err := core.BuildOpenItemsPDF(oi, title, a.profile)
		if err != nil {
			a.showError(a.bundle.T("error.processing.title"), err.Error())
			return
		}
		a.savePDF(fmt.Sprintf("OffenePosten_%d.pdf", year), data)
	})

	header := container.NewBorder(nil, nil, nil, pdfBtn,
		widget.NewLabelWithStyle(
			fmt.Sprintf(a.bundle.T("opos.title"), year),
			fyne.TextAlignLeading,
			fyne.TextStyle{Bold: true},
		),
	)

	content := container.NewBorder(header, nil, nil, nil, scroll)
	d := dialog.NewCustom(fmt.Sprintf(a.bundle.T("opos.title"), year), a.bundle.T("common.close"), content, a.window)
	d.Resize(fyne.NewSize(700, 520))
	d.Show()
}
