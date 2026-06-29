package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/bergx2/buchisy/internal/core"
)

// showControllingDialog shows per-account booked sums split into Einnahmen and
// Ausgaben for the current month or the whole current year, toggled by a
// segmented control.
func (a *App) showControllingDialog() {
	var c core.Controlling
	yearMode := false

	makeTable := func(sums []core.AccountSum) *widget.Table {
		t := widget.NewTable(
			func() (int, int) { return len(sums), 3 },
			func() fyne.CanvasObject { return newHoverLabel(nil, nil) },
			func(id widget.TableCellID, o fyne.CanvasObject) {
				hl := o.(*hoverLabel)
				// CRITICAL: cells are recycled — reset tooltip on every update.
				// The original callback set Alignment per column but not TextStyle,
				// so we only reset Alignment (no bold rows in this table).
				hl.tooltip = ""
				s := sums[id.Row]
				switch id.Col {
				case 0:
					hl.Alignment = fyne.TextAlignLeading
					hl.SetText(fmt.Sprintf("%d", s.Konto))
				case 1:
					hl.Alignment = fyne.TextAlignLeading
					hl.SetText(s.Name)
				default:
					hl.Alignment = fyne.TextAlignTrailing
					hl.SetText(formatMoney(s.Summe, "EUR", a.settings.DecimalSeparator))
				}
			},
		)
		t.SetColumnWidth(0, 70)
		t.SetColumnWidth(1, 280)
		t.SetColumnWidth(2, 100)
		return t
	}

	fmtAmount := func(v float64) string {
		return formatMoney(v, "EUR", a.settings.DecimalSeparator)
	}

	// Section labels and totals — created once, refreshed via reload.
	einnahmenLabel := widget.NewLabelWithStyle(a.bundle.T("controlling.einnahmen"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	einnahmenTotal := newCopyableLabel(a.bundle, "")
	ausgabenLabel := widget.NewLabelWithStyle(a.bundle.T("controlling.ausgaben"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	ausgabenTotal := newCopyableLabel(a.bundle, "")
	saldoLabel := widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

	// The two section tables; replaced on each reload.
	einnahmenTable := makeTable(nil)
	ausgabenTable := makeTable(nil)

	// scrollBox is rebuilt on each reload; we use a container.Scroll wrapping a VBox.
	scrollContent := container.NewVBox()
	scroll := container.NewVScroll(scrollContent)

	rebuildContent := func() {
		einnahmenTable = makeTable(c.Einnahmen)
		ausgabenTable = makeTable(c.Ausgaben)

		einnahmenTotal.SetText(a.bundle.T("controlling.total", fmtAmount(c.EinnahmenGesamt)))
		ausgabenTotal.SetText(a.bundle.T("controlling.total", fmtAmount(c.AusgabenGesamt)))
		saldoLabel.SetText(a.bundle.T("controlling.saldo", fmtAmount(c.Saldo)))

		scrollContent.Objects = []fyne.CanvasObject{
			einnahmenLabel,
			einnahmenTable,
			einnahmenTotal,
			widget.NewSeparator(),
			ausgabenLabel,
			ausgabenTable,
			ausgabenTotal,
			widget.NewSeparator(),
			saldoLabel,
		}
		scrollContent.Refresh()
	}

	reload := func() {
		fromY, fromM, toY, toM := a.currentYear, int(a.currentMonth), a.currentYear, int(a.currentMonth)
		if yearMode {
			fromM, toM = 1, 12
		}
		rows := a.collectInvoiceRows(fromY, fromM, toY, toM)

		paymentKonten := map[int]bool{}
		for _, ba := range a.settings.BankAccounts {
			if pay, ok := a.settings.PaymentAccountSKR04(ba.Name); ok {
				paymentKonten[pay] = true
			}
		}

		c = core.AggregateControlling(rows, a.bookingRules, paymentKonten, a.chart)
		rebuildContent()
	}

	toggle := widget.NewRadioGroup([]string{a.bundle.T("export.month"), a.bundle.T("export.year")}, func(sel string) {
		yearMode = sel == a.bundle.T("export.year")
		reload()
	})
	toggle.Horizontal = true
	toggle.SetSelected(a.bundle.T("export.month"))
	reload()

	pdfBtn := widget.NewButton(a.bundle.T("report.pdf"), func() {
		title := a.bundle.T("controlling.title")
		data, err := core.BuildControllingPDF(c, title, a.profile)
		if err != nil {
			a.showError(a.bundle.T("error.processing.title"), err.Error())
			return
		}
		period := fmt.Sprintf("%04d", a.currentYear)
		if !yearMode {
			period = fmt.Sprintf("%04d-%02d", a.currentYear, int(a.currentMonth))
		}
		a.savePDF("Controlling_"+period+".pdf", data)
	})

	header := widget.NewLabelWithStyle(a.bundle.T("controlling.heading"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	topBar := container.NewBorder(nil, nil, nil, pdfBtn, toggle)
	content := container.NewBorder(container.NewVBox(header, topBar), nil, nil, nil, scroll)
	d := dialog.NewCustom(a.bundle.T("controlling.title"), a.bundle.T("common.close"), content, a.window)
	d.Resize(fyne.NewSize(520, 540))
	d.Show()
}
