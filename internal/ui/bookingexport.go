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
// double-entry bookings for the current month or the whole current year
// to a DATEV EXTF file (Windows-1252) and a Lexware CSV (UTF-8).
func (a *App) showBookingExportDialog() {
	var d *dialog.CustomDialog

	monthBtn := widget.NewButton(a.bundle.T("export.month"), func() {
		if d != nil {
			d.Hide()
		}
		fromY, fromM := a.currentYear, int(a.currentMonth)
		toY, toM := a.currentYear, int(a.currentMonth)
		period := fmt.Sprintf("%04d-%02d", a.currentYear, int(a.currentMonth))
		a.runBookingExport(fromY, fromM, toY, toM, period)
	})
	monthBtn.Importance = widget.HighImportance

	yearBtn := widget.NewButton(a.bundle.T("export.year"), func() {
		if d != nil {
			d.Hide()
		}
		fromY, fromM := a.currentYear, 1
		toY, toM := a.currentYear, 12
		period := fmt.Sprintf("%04d", a.currentYear)
		a.runBookingExport(fromY, fromM, toY, toM, period)
	})

	content := container.NewHBox(monthBtn, yearBtn)
	d = dialog.NewCustom(a.bundle.T("export.bookings"), a.bundle.T("btn.cancel"), content, a.window)
	d.Show()
}

// runBookingExport collects rows for the given month range, builds the DATEV
// and Lexware files, and asks the user for a target folder before writing.
func (a *App) runBookingExport(fromY, fromM, toY, toM int, period string) {
	rows := a.collectInvoiceRows(fromY, fromM, toY, toM)

	von, bis := datevPeriod(fromY, fromM, toY, toM)
	h := core.DATEVHeader{
		BeraterNr: a.settings.DatevBeraterNr,
		MandantNr: a.settings.DatevMandantNr,
		WJBeginn:  a.settings.DatevWJBeginn,
		ErzeugtAm: time.Now().Format("20060102150405") + "000",
		DatumVon:  von,
		DatumBis:  bis,
	}
	datevBytes, dExp, dSkip := core.BuildDATEVStapel(h, rows)
	lexBytes, _, _ := core.BuildLexwareCSV(rows)

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

		a.logger.Info("Buchungsexport: %d Zeilen nach %s", dExp, uri.Path())
		a.showInfo(a.bundle.T("export.bookings"), a.bundle.T("export.done", dExp, dSkip))
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
