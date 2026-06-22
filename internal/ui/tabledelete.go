package ui

import (
	"fmt"
	"os"

	"fyne.io/fyne/v2/dialog"

	"github.com/bergx2/buchisy/internal/core"
)

// unlinkInvoice clears the BuchungRef of an invoice after user confirmation.
func (a *App) unlinkInvoice(row core.CSVRow) {
	dialog.ShowConfirm(
		a.bundle.T("table.unlink"),
		a.bundle.T("table.unlinkConfirm"),
		func(confirm bool) {
			if !confirm {
				return
			}
			row.BuchungRef = ""
			if err := a.dbRepo.Update(row.Jahr, row.Monat, row.Dateiname, row); err != nil {
				a.logger.Error("unlinkInvoice Update %s: %v", row.Dateiname, err)
			}
			a.loadInvoices()
		},
		a.window,
	)
}

// showDeleteConfirmation shows a confirmation dialog before deleting an invoice.
func (a *App) showDeleteConfirmation(row core.CSVRow) {
	message := a.bundle.T(
		"table.delete.confirm.message",
		row.Dateiname,
		row.Auftraggeber,
		row.Bruttobetrag,
		row.Waehrung,
	)

	dialog.ShowConfirm(
		a.bundle.T("table.delete.confirm.title"),
		message,
		func(confirm bool) {
			if confirm {
				a.deleteInvoice(row)
			}
		},
		a.window,
	)
}

// deleteInvoice deletes an invoice from both the filesystem and CSV.
func (a *App) deleteInvoice(row core.CSVRow) {
	a.logger.Info("Deleting invoice: %s", row.Dateiname)

	// Build file path
	targetFolder := a.storageManager.GetMonthFolder(a.currentYear, a.currentMonth)
	filePath := core.InvoiceFilePath(targetFolder, row)

	// Delete PDF file. A missing file is fine (nothing to delete); any
	// other failure is reported to the user after the CSV entry is removed.
	fileRemoveFailed := false
	if err := os.Remove(filePath); err != nil {
		if !os.IsNotExist(err) {
			fileRemoveFailed = true
			a.logger.Error("Failed to delete PDF file: %v", err)
		}
	} else {
		a.logger.Info("Deleted PDF file: %s", filePath)
	}

	// Delete from SQLite database
	jahr := fmt.Sprintf("%04d", a.currentYear)
	monat := fmt.Sprintf("%02d", a.currentMonth)
	err := a.dbRepo.Delete(jahr, monat, row.Dateiname)
	if err != nil {
		a.showError(
			a.bundle.T("error.processing.title"),
			fmt.Sprintf("Failed to delete from database: %v", err),
		)
		return
	}

	a.logger.Info("Deleted invoice from database: %s", row.Dateiname)

	// Export updated data to CSV
	csvPath := a.storageManager.GetCSVPath(a.currentYear, a.currentMonth)
	if err := a.dbRepo.ExportToCSV(jahr, monat, csvPath, a.csvRepo); err != nil {
		a.logger.Warn("Failed to export to CSV after delete: %v", err)
		// Continue even if CSV export fails
	}

	// Reload table
	a.loadInvoices()

	if fileRemoveFailed {
		// Bigger problem — keep the modal so the user notices the
		// orphan file warning.
		a.showInfo(
			"Gelöscht",
			fmt.Sprintf("Der Eintrag wurde gelöscht, aber die Datei konnte nicht "+
				"entfernt werden — evtl. ist sie in einem anderen Programm geöffnet: %s",
				row.Dateiname),
		)
	} else {
		a.showToast(fmt.Sprintf("✓ Rechnung gelöscht: %s", row.Dateiname))
	}
}
