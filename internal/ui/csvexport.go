package ui

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/bergx2/buchisy/internal/core"
)

// showCSVExportDialog lets the user export the invoices of a month or a
// date range to a CSV file at a location of their choosing.
func (a *App) showCSVExportDialog() {
	const modeMonth = "Monat"
	const modeRange = "Zeitraum"

	yearSelect := widget.NewSelect(generateYearOptions(), nil)
	yearSelect.SetSelected(fmt.Sprintf("%d", a.currentYear))
	monthSelect := widget.NewSelect(generateMonthOptions(a.bundle), nil)
	monthSelect.SetSelected(fmt.Sprintf("%02d - %-12s", int(a.currentMonth),
		a.bundle.T(fmt.Sprintf("month.%02d", int(a.currentMonth)))))
	monthForm := container.NewGridWithColumns(2, yearSelect, monthSelect)

	fromEntry := widget.NewEntry()
	fromEntry.SetPlaceHolder("TT.MM.JJJJ")
	toEntry := widget.NewEntry()
	toEntry.SetPlaceHolder("TT.MM.JJJJ")
	rangeForm := container.NewVBox(
		container.NewBorder(nil, nil, widget.NewLabel("Von"), nil, fromEntry),
		container.NewBorder(nil, nil, widget.NewLabel("Bis"), nil, toEntry),
	)

	modeRadio := widget.NewRadioGroup([]string{modeMonth, modeRange}, func(sel string) {
		if sel == modeRange {
			monthForm.Hide()
			rangeForm.Show()
		} else {
			rangeForm.Hide()
			monthForm.Show()
		}
	})
	modeRadio.SetSelected(modeMonth)

	content := container.NewVBox(modeRadio, monthForm, rangeForm)

	dialog.ShowCustomConfirm("CSV-Export", "Exportieren", "Abbrechen", content,
		func(ok bool) {
			if !ok {
				return
			}
			fromY, fromM, toY, toM, err := csvExportRange(
				modeRadio.Selected == modeRange,
				yearSelect.Selected, monthSelect.Selected,
				fromEntry.Text, toEntry.Text)
			if err != nil {
				a.showError(a.bundle.T("error.processing.title"), err.Error())
				return
			}
			rows := a.collectInvoiceRows(fromY, fromM, toY, toM)
			if len(rows) == 0 {
				a.showInfo("CSV-Export", "Keine Rechnungen im gewählten Bereich.")
				return
			}
			a.saveExportCSV(rows)
		}, a.window)
}

// csvExportRange turns the dialog inputs into an inclusive month range
// (fromYear, fromMonth) .. (toYear, toMonth).
func csvExportRange(isRange bool, yearSel, monthSel, fromTxt, toTxt string) (int, int, int, int, error) {
	if isRange {
		fy, fm, ok1 := parseDateMonth(fromTxt)
		ty, tm, ok2 := parseDateMonth(toTxt)
		if !ok1 || !ok2 {
			return 0, 0, 0, 0, fmt.Errorf("Bitte gültige Datumsangaben (TT.MM.JJJJ) eingeben.")
		}
		if ty < fy || (ty == fy && tm < fm) {
			return 0, 0, 0, 0, fmt.Errorf("Das Bis-Datum liegt vor dem Von-Datum.")
		}
		return fy, fm, ty, tm, nil
	}
	y, _ := strconv.Atoi(strings.TrimSpace(yearSel))
	m := 0
	if len(monthSel) >= 2 {
		m, _ = strconv.Atoi(strings.TrimSpace(monthSel[:2]))
	}
	if y == 0 || m < 1 || m > 12 {
		return 0, 0, 0, 0, fmt.Errorf("Bitte Jahr und Monat wählen.")
	}
	return y, m, y, m, nil
}

// parseDateMonth parses a TT.MM.JJJJ string and returns its year and month.
func parseDateMonth(s string) (year, month int, ok bool) {
	parts := strings.Split(strings.TrimSpace(s), ".")
	if len(parts) != 3 {
		return 0, 0, false
	}
	m, e1 := strconv.Atoi(parts[1])
	y, e2 := strconv.Atoi(parts[2])
	if e1 != nil || e2 != nil || m < 1 || m > 12 || y < 1 {
		return 0, 0, false
	}
	return y, m, true
}

// collectInvoiceRows gathers all invoice rows from the monthly CSVs in the
// inclusive month range.
func (a *App) collectInvoiceRows(fromY, fromM, toY, toM int) []core.CSVRow {
	var rows []core.CSVRow
	y, m := fromY, fromM
	for y < toY || (y == toY && m <= toM) {
		monthRows, err := a.csvRepo.Load(a.storageManager.GetCSVPath(y, time.Month(m)))
		if err != nil {
			a.logger.Warn("CSV-Export: Monat %04d-%02d übersprungen: %v", y, m, err)
		} else {
			rows = append(rows, monthRows...)
		}
		m++
		if m > 12 {
			m = 1
			y++
		}
	}
	return rows
}

// saveExportCSV builds the CSV in memory, then asks for a target file and
// writes it there. Building first means a write failure cannot leave a
// half-written file behind.
func (a *App) saveExportCSV(rows []core.CSVRow) {
	var buf bytes.Buffer
	if err := a.csvRepo.WriteTo(&buf, rows); err != nil {
		a.showError(a.bundle.T("error.processing.title"), err.Error())
		return
	}
	d := dialog.NewFileSave(func(w fyne.URIWriteCloser, err error) {
		if w == nil {
			return // user cancelled
		}
		defer w.Close()
		if err != nil {
			a.showError(a.bundle.T("error.processing.title"), err.Error())
			return
		}
		if _, werr := w.Write(buf.Bytes()); werr != nil {
			a.showError(a.bundle.T("error.processing.title"), werr.Error())
			return
		}
		a.logger.Info("CSV-Export: %d Rechnungen exportiert", len(rows))
	}, a.window)
	d.SetFileName("Export.csv")
	d.Show()
}
