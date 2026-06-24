package ui

import (
	"fmt"
	"os"
	"time"

	"github.com/bergx2/buchisy/internal/core"
)

// showExportPackage builds a GoBD/StB export ZIP for the current year
// (full year, months 1–12), identical period to the "Ganzes Jahr" booking
// export path. The ZIP contains the DATEV-EXTF Buchungsstapel, one PDF per
// Beleg (skipped when the file is missing/unreadable), manifest.csv and a
// GoBD-orientated index.xml.
func (a *App) showExportPackage() {
	fromY, fromM, toY, toM := a.currentYear, 1, a.currentYear, 12
	period := fmt.Sprintf("%04d", a.currentYear)

	rows := a.collectInvoiceRows(fromY, fromM, toY, toM)

	// Build DATEV header the same way writeBookingExport does.
	von, bis := datevPeriod(fromY, fromM, toY, toM)
	h := core.DATEVHeader{
		BeraterNr: a.settings.DatevBeraterNr,
		MandantNr: a.settings.DatevMandantNr,
		WJBeginn:  a.settings.DatevWJBeginn,
		ErzeugtAm: time.Now().Format("20060102150405") + "000",
		DatumVon:  von,
		DatumBis:  bis,
	}
	datev, _, _ := core.BuildDATEVStapel(h, rows)

	// Collect Belegbilder; skip rows whose PDF cannot be read.
	var belege []core.BelegFile
	for _, row := range rows {
		path := a.resolveInvoicePath(row)
		b, err := os.ReadFile(path)
		if err != nil {
			a.logger.Warn("exportpackage: skipping missing beleg %q: %v", row.Dateiname, err)
			continue
		}
		belege = append(belege, core.BelegFile{
			Belegnummer: row.Belegnummer,
			Dateiname:   row.Dateiname,
			Bytes:       b,
		})
	}

	zipBytes, err := core.BuildExportPackage(rows, datev, belege, period)
	if err != nil {
		a.showError(a.bundle.T("error.processing.title"), err.Error())
		return
	}

	a.savePDF("GoBD-Export_"+period+".zip", zipBytes)
	a.showInfo(a.bundle.T("exportpkg.menu"), a.bundle.T("exportpkg.done", len(belege), len(rows)))
}
