package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"golang.org/x/text/encoding/charmap"

	"github.com/bergx2/buchisy/internal/core"
)

// showBookingExportDialog opens a dialog that lets the user export the
// double-entry bookings for the current month, the whole current year, or any
// chosen date range to a DATEV EXTF file (Windows-1252) and a Lexware CSV
// (UTF-8).
func (a *App) showBookingExportDialog() {
	monthLabel := a.bundle.T("export.month")
	yearLabel := a.bundle.T("export.year")
	rangeLabel := a.bundle.T("export.range")

	fromEntry := widget.NewEntry()
	fromEntry.SetPlaceHolder("TT.MM.JJJJ")
	toEntry := widget.NewEntry()
	toEntry.SetPlaceHolder("TT.MM.JJJJ")
	rangeForm := container.NewVBox(
		container.NewBorder(nil, nil, widget.NewLabel(a.bundle.T("export.from")), nil, fromEntry),
		container.NewBorder(nil, nil, widget.NewLabel(a.bundle.T("export.to")), nil, toEntry),
	)
	rangeForm.Hide()

	modeRadio := widget.NewRadioGroup([]string{monthLabel, yearLabel, rangeLabel}, func(sel string) {
		if sel == rangeLabel {
			rangeForm.Show()
		} else {
			rangeForm.Hide()
		}
	})
	modeRadio.SetSelected(monthLabel)

	content := container.NewVBox(modeRadio, rangeForm)
	dialog.ShowCustomConfirm(a.bundle.T("export.bookings"), a.bundle.T("export.do"), a.bundle.T("btn.cancel"), content,
		func(ok bool) {
			if !ok {
				return
			}
			var fromY, fromM, toY, toM int
			var period string
			switch modeRadio.Selected {
			case yearLabel:
				fromY, fromM, toY, toM = a.currentYear, 1, a.currentYear, 12
				period = fmt.Sprintf("%04d", a.currentYear)
			case rangeLabel:
				fy, fm, ty, tm, err := csvExportRange(true, "", "", fromEntry.Text, toEntry.Text)
				if err != nil {
					a.showError(a.bundle.T("error.processing.title"), err.Error())
					return
				}
				fromY, fromM, toY, toM = fy, fm, ty, tm
				period = fmt.Sprintf("%04d-%02d_bis_%04d-%02d", fy, fm, ty, tm)
			default: // current month
				fromY, fromM, toY, toM = a.currentYear, int(a.currentMonth), a.currentYear, int(a.currentMonth)
				period = fmt.Sprintf("%04d-%02d", a.currentYear, int(a.currentMonth))
			}
			a.runBookingExport(fromY, fromM, toY, toM, period)
		}, a.window)
}

// runBookingExport collects rows for the given month range, shows a preview
// dialog with the export classification, and on confirmation writes the files.
func (a *App) runBookingExport(fromY, fromM, toY, toM int, period string) {
	rows := a.collectInvoiceRows(fromY, fromM, toY, toM)

	previewLabel := widget.NewLabel("")
	updatePreview := func(include bool) {
		c := core.ClassifyForExport(rows, include)
		previewLabel.SetText(a.bundle.T("export.preview", len(c.Exportable), len(c.AlreadyExported), len(c.Skipped)))
	}
	includeCheck := widget.NewCheck(a.bundle.T("export.includeExported"), func(checked bool) { updatePreview(checked) })
	updatePreview(false)
	confirm := container.NewVBox(previewLabel, includeCheck)
	dialog.ShowCustomConfirm(a.bundle.T("export.bookings"), a.bundle.T("export.do"), a.bundle.T("btn.cancel"), confirm, func(ok bool) {
		if !ok {
			return
		}
		cls := core.ClassifyForExport(rows, includeCheck.Checked)
		a.writeBookingExport(cls.Exportable, fromY, fromM, toY, toM, period)
	}, a.window)
}

// writeBookingExport builds the DATEV and Lexware files from the given
// exportable rows, asks the user for a target folder, writes both files, and
// on success marks each row as exported and reloads the invoice table.
func (a *App) writeBookingExport(exportable []core.CSVRow, fromY, fromM, toY, toM int, period string) {
	von, bis := datevPeriod(fromY, fromM, toY, toM)
	h := core.DATEVHeader{
		BeraterNr: a.settings.DatevBeraterNr,
		MandantNr: a.settings.DatevMandantNr,
		WJBeginn:  a.settings.DatevWJBeginn,
		ErzeugtAm: time.Now().Format("20060102150405") + "000",
		DatumVon:  von,
		DatumBis:  bis,
	}
	datevBytes, dExp, _ := core.BuildDATEVStapel(h, exportable)
	lexBytes, _, _ := core.BuildLexwareCSV(exportable)

	datevName := "DATEV-EXTF_" + period + ".csv"
	lexName := "Lexware-Buchungen_" + period + ".csv"

	dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
		if uri == nil {
			return // user cancelled
		}
		if err != nil {
			a.showError(a.bundle.T("error.processing.title"), err.Error())
			return
		}

		// Re-encode DATEV file to Windows-1252.
		enc, encErr := charmap.Windows1252.NewEncoder().Bytes(datevBytes)
		if encErr != nil {
			a.logger.Warn("DATEV Windows-1252 encoding failed, falling back to UTF-8: %v", encErr)
			enc = datevBytes
		}

		if werr := os.WriteFile(filepath.Join(uri.Path(), datevName), enc, 0644); werr != nil {
			a.showError(a.bundle.T("error.processing.title"), werr.Error())
			return
		}
		if werr := os.WriteFile(filepath.Join(uri.Path(), lexName), lexBytes, 0644); werr != nil {
			a.showError(a.bundle.T("error.processing.title"), werr.Error())
			return
		}

		// Write the booking journal PDF alongside DATEV and Lexware files.
		journal, jerr := core.BuildBookingJournalPDF(exportable, a.chart, "Buchungsjournal "+period, a.profile)
		if jerr != nil {
			a.logger.Warn("journal PDF build failed: %v", jerr)
		} else if werr := os.WriteFile(filepath.Join(uri.Path(), "Buchungsjournal_"+period+".pdf"), journal, 0644); werr != nil {
			a.logger.Warn("journal PDF write failed: %v", werr)
		}

		// Mark each exported row in the database.
		for _, r := range exportable {
			if merr := a.dbRepo.MarkExported(r.Jahr, r.Monat, r.Dateiname); merr != nil {
				a.logger.Warn("MarkExported failed for %s: %v", r.Dateiname, merr)
			}
		}
		a.loadInvoices() // reload from DB to reflect the new Exportiert state

		a.logger.Info("Buchungsexport: %d Zeilen nach %s", dExp, uri.Path())
		a.showInfo(a.bundle.T("export.bookings"), a.bundle.T("export.done", dExp))
	}, a.window)
}

// datevPeriod returns the EXTF DatumVon/DatumBis (YYYYMMDD) for a from/to
// month range — DatumVon = the 1st of the from-month, DatumBis = the real
// last day of the to-month (handles 28/29/30/31-day months).
func datevPeriod(fromY, fromM, toY, toM int) (von, bis string) {
	von = fmt.Sprintf("%04d%02d01", fromY, fromM)
	lastDay := time.Date(toY, time.Month(toM)+1, 0, 0, 0, 0, 0, time.UTC).Day()
	bis = fmt.Sprintf("%04d%02d%02d", toY, toM, lastDay)
	return von, bis
}
