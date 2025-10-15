package ui

import (
	"fmt"
	"os"
	"path/filepath"

	"fyne.io/fyne/v2/dialog"

	"github.com/bergx2/buchisy/internal/core"
)

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
	filePath := filepath.Join(targetFolder, row.Dateiname)

	// Delete PDF file
	if err := os.Remove(filePath); err != nil {
		a.logger.Error("Failed to delete PDF file: %v", err)
		// Continue anyway to remove from CSV
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

	a.showInfo(
		"Gelöscht",
		fmt.Sprintf("Rechnung wurde gelöscht: %s", row.Dateiname),
	)
}
