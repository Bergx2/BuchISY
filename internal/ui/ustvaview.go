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

// showUStVADialog shows the VAT return for the current month, quarter, or whole
// year using the official ELSTER Kennzahlen (net bases + derived VAT + Zahllast).
func (a *App) showUStVADialog() {
	// period: 0 = month, 1 = quarter, 2 = year
	period := 0 // default: month (UStVA is filed monthly or quarterly)

	body := container.NewVBox()
	scroll := container.NewVScroll(body)

	fmtAmt := func(v float64) string {
		return strings.Replace(fmt.Sprintf("%.2f", v), ".", ",", 1)
	}

	addSection := func(headingKey string, lines []struct {
		kzKey  string
		kzNum  string
		val    float64
		ustKey string // optional: key for derived USt label
		ustVal float64
	}) {
		body.Add(widget.NewLabelWithStyle(
			a.bundle.T(headingKey),
			fyne.TextAlignLeading,
			fyne.TextStyle{Bold: true},
		))
		for _, l := range lines {
			if l.val == 0 {
				continue
			}
			label := a.bundle.T(l.kzKey)
			body.Add(widget.NewLabel(fmt.Sprintf("    %s  %s   %s €", l.kzNum, label, fmtAmt(l.val))))
			if l.ustKey != "" && l.ustVal != 0 {
				ustLabel := a.bundle.T("ustva.ust")
				body.Add(widget.NewLabel(fmt.Sprintf("        → %s: %s €", ustLabel, fmtAmt(l.ustVal))))
			}
		}
		body.Add(widget.NewSeparator())
	}

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
		u := core.ComputeUStVAOfficial(rows, a.bookingRules)

		body.Objects = nil

		// A. Umsätze
		addSection("ustva.sectionA", []struct {
			kzKey  string
			kzNum  string
			val    float64
			ustKey string
			ustVal float64
		}{
			{"ustva.kz81", "Kz 81", u.Kz81, "ustva.ust", u.USt81},
			{"ustva.kz86", "Kz 86", u.Kz86, "ustva.ust", u.USt86},
		})

		// E. Nicht steuerbare Umsätze
		addSection("ustva.sectionE", []struct {
			kzKey  string
			kzNum  string
			val    float64
			ustKey string
			ustVal float64
		}{
			{"ustva.kz21", "Kz 21", u.Kz21, "", 0},
			{"ustva.kz45", "Kz 45", u.Kz45, "", 0},
		})

		// D. § 13b
		addSection("ustva.sectionD", []struct {
			kzKey  string
			kzNum  string
			val    float64
			ustKey string
			ustVal float64
		}{
			{"ustva.kz84", "Kz 84", u.Kz84, "", 0},
			{"ustva.kz85", "Kz 85", u.Kz85, "", 0},
		})

		// F. Vorsteuer
		addSection("ustva.sectionF", []struct {
			kzKey  string
			kzNum  string
			val    float64
			ustKey string
			ustVal float64
		}{
			{"ustva.kz66", "Kz 66", u.Kz66, "", 0},
			{"ustva.kz67", "Kz 67", u.Kz67, "", 0},
		})

		// Kz83: Zahllast or Überschuss — always shown
		zKey, zVal := "ustva.zahllast", u.Kz83
		if u.Kz83 < 0 {
			zKey, zVal = "ustva.ueberschuss", -u.Kz83
		}
		kz83Line := fmt.Sprintf("Kz 83   %s", a.bundle.T(zKey, fmtAmt(zVal)))
		body.Add(widget.NewLabelWithStyle(kz83Line, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))

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
	toggle.SetSelected(a.bundle.T("export.month"))
	reload()

	header := widget.NewLabelWithStyle(a.bundle.T("ustva.heading"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	content := container.NewBorder(container.NewVBox(header, toggle), nil, nil, nil, scroll)
	d := dialog.NewCustom(a.bundle.T("ustva.title"), a.bundle.T("common.close"), content, a.window)
	d.Resize(fyne.NewSize(520, 520))
	d.Show()
}
