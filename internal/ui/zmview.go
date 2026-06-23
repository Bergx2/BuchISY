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

// showZMDialog shows the Zusammenfassende Meldung (EC Sales List) for a
// selectable period: current month, current calendar quarter, or whole year.
func (a *App) showZMDialog() {
	// period: 0 = month, 1 = quarter, 2 = year
	period := 1 // default: quarter (the official ZM filing period)

	body := container.NewVBox()
	scroll := container.NewVScroll(body)

	fmtAmt := func(v float64) string {
		return strings.Replace(fmt.Sprintf("%.2f", v), ".", ",", 1)
	}

	var zm core.ZM // current period's result, for the PDF export

	reload := func() {
		fromY, fromM, toY, toM := a.currentYear, int(a.currentMonth), a.currentYear, int(a.currentMonth)
		switch period {
		case 1: // quarter: calendar quarter containing currentMonth
			q := (int(a.currentMonth) - 1) / 3
			fromM = q*3 + 1
			toM = q*3 + 3
		case 2: // year
			fromM, toM = 1, 12
		}
		rows := a.collectInvoiceRows(fromY, fromM, toY, toM)
		zm = core.ComputeZM(rows)

		body.Objects = nil

		if len(zm.Zeilen) == 0 {
			body.Add(widget.NewLabel(a.bundle.T("zm.empty")))
		} else {
			sonstige := a.bundle.T("zm.art.sonstige")
			for _, z := range zm.Zeilen {
				body.Add(widget.NewLabel(fmt.Sprintf("    %s    %s €    %s", z.UStIdNr, fmtAmt(z.Netto), sonstige)))
			}
			body.Add(widget.NewSeparator())
			body.Add(widget.NewLabelWithStyle(
				a.bundle.T("zm.kontrollsumme", fmtAmt(zm.Kontrollsumme)),
				fyne.TextAlignLeading,
				fyne.TextStyle{Bold: true},
			))
		}
		body.Refresh()
	}

	toggleLabels := []string{
		a.bundle.T("export.month"),
		a.bundle.T("zm.quarter"),
		a.bundle.T("export.year"),
	}
	toggle := widget.NewRadioGroup(toggleLabels, func(sel string) {
		switch sel {
		case a.bundle.T("export.month"):
			period = 0
		case a.bundle.T("zm.quarter"):
			period = 1
		default:
			period = 2
		}
		reload()
	})
	toggle.Horizontal = true
	toggle.SetSelected(a.bundle.T("zm.quarter"))
	reload()

	pdfBtn := widget.NewButton(a.bundle.T("report.pdf"), func() {
		data, err := core.BuildZMPDF(zm, a.settings.OwnVATID, a.bundle.T("zm.title"))
		if err != nil {
			a.showError(a.bundle.T("error.processing.title"), err.Error())
			return
		}
		periodStr := fmt.Sprintf("%04d", a.currentYear)
		switch period {
		case 0:
			periodStr = fmt.Sprintf("%04d-%02d", a.currentYear, int(a.currentMonth))
		case 1:
			periodStr = fmt.Sprintf("%04d-Q%d", a.currentYear, (int(a.currentMonth)-1)/3+1)
		}
		a.savePDF("ZM_"+periodStr+".pdf", data)
	})

	headingLabel := widget.NewLabelWithStyle(a.bundle.T("zm.heading"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	headerItems := []fyne.CanvasObject{headingLabel}
	if a.settings.OwnVATID != "" {
		headerItems = append(headerItems, widget.NewLabel("USt-IdNr: "+a.settings.OwnVATID))
	}
	headerItems = append(headerItems, container.NewBorder(nil, nil, nil, pdfBtn, toggle))
	header := container.NewVBox(headerItems...)

	content := container.NewBorder(header, nil, nil, nil, scroll)
	d := dialog.NewCustom(a.bundle.T("zm.title"), a.bundle.T("common.close"), content, a.window)
	d.Resize(fyne.NewSize(520, 400))
	d.Show()
}
