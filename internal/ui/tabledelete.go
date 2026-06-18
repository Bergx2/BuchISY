package ui

import (
	"fmt"
	"os"

	"fyne.io/fyne/v2/dialog"

	"github.com/bergx2/buchisy/internal/core"
)

// showDeleteConfirmation shows a confirmation dialog before deleting an invoice.
func (a *App) showDeleteConfirmation(row core.CSVRow) {
	message := a.bundle.T(
		"table.delete.confirm.message",
		row.Dateiname,
		row.Firmenname,
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

	// Reload CSV, remove this row, and save
	csvPath := a.storageManager.GetCSVPath(a.currentYear, a.currentMonth)
	existingRows, err := a.csvRepo.Load(csvPath)
	if err != nil {
		a.showError(
			a.bundle.T("error.processing.title"),
			fmt.Sprintf("Failed to load CSV: %v", err),
		)
		return
	}

	// Filter out the deleted row
	newRows := []core.CSVRow{}
	for _, r := range existingRows {
		if r.Dateiname != row.Dateiname {
			newRows = append(newRows, r)
		}
	}

	// Rewrite CSV file
	if err := a.rewriteCSV(csvPath, newRows); err != nil {
		a.showError(
			a.bundle.T("error.processing.title"),
			fmt.Sprintf("Failed to update CSV: %v", err),
		)
		return
	}

	a.logger.Info("Deleted invoice from CSV: %s", row.Dateiname)

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

// rewriteCSV rewrites a CSV file with new rows.
func (a *App) rewriteCSV(csvPath string, rows []core.CSVRow) error {
	// Delete old file
	if err := os.Remove(csvPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove old CSV: %w", err)
	}

	// Write new rows
	for _, row := range rows {
		if err := a.csvRepo.Append(csvPath, row); err != nil {
			return fmt.Errorf("failed to write row: %w", err)
		}
	}

	return nil
}
