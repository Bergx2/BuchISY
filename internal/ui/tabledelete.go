package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

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
	if a.currentMonthLocked {
		a.showInfo(a.bundle.T("period.locked.title"), a.bundle.T("period.locked.msg"))
		return
	}
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

	// Buffer file bytes BEFORE removal so Undo can restore the file.
	data, _ := os.ReadFile(filePath) // nil if file doesn't exist — that's fine

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
		// Capture loop variables for the closure.
		capturedRow := row
		capturedJahr := jahr
		capturedMonat := monat
		capturedFilePath := filePath
		capturedData := data
		undoDone := false // one-shot: the 8s toast can be tapped only once
		a.showToastWithAction(
			"✓ Rechnung gelöscht: "+row.Dateiname,
			a.bundle.T("undo"),
			func() {
				if undoDone {
					return
				}
				undoDone = true
				a.undoDelete(capturedRow, capturedJahr, capturedMonat, capturedFilePath, capturedData)
			},
		)
	}
}

// undoDelete restores a deleted invoice: rewrites the PDF file (if data is
// available), re-inserts the row into the database, regenerates the CSV, and
// reloads the invoice table.
func (a *App) undoDelete(row core.CSVRow, jahr, monat, filePath string, data []byte) {
	// Restore the file if we have its contents.
	if len(data) > 0 {
		if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
			a.showError(a.bundle.T("error.processing.title"),
				fmt.Sprintf("Undo: Verzeichnis konnte nicht erstellt werden: %v", err))
			return
		}
		if err := os.WriteFile(filePath, data, 0644); err != nil {
			a.showError(a.bundle.T("error.processing.title"),
				fmt.Sprintf("Undo: Datei konnte nicht wiederhergestellt werden: %v", err))
			return
		}
		a.logger.Info("Undo: restored file %s", filePath)
	}

	// Re-insert the database row.
	if _, err := a.dbRepo.Insert(row); err != nil {
		a.showError(a.bundle.T("error.processing.title"),
			fmt.Sprintf("Undo: Datenbankeintrag konnte nicht wiederhergestellt werden: %v", err))
		return
	}
	a.logger.Info("Undo: re-inserted invoice %s", row.Dateiname)

	// Regenerate the CSV for the month the invoice BELONGED to (captured at
	// delete time) — the user may have navigated to another month during the
	// 8-second undo window, so a.currentYear/Month would be wrong.
	y, _ := strconv.Atoi(jahr)
	m, _ := strconv.Atoi(monat)
	csvPath := a.storageManager.GetCSVPath(y, time.Month(m))
	if err := a.dbRepo.ExportToCSV(jahr, monat, csvPath, a.csvRepo); err != nil {
		a.logger.Warn("Undo: Failed to export CSV: %v", err)
	}

	// Reload the table and confirm to the user.
	a.loadInvoices()
	a.showToast(a.bundle.T("undo.done"))
}
