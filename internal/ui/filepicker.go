package ui

import (
	"context"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

// selectPDFFiles shows a custom file picker with search functionality.
func (a *App) selectPDFFiles() {
	// Use custom picker with search instead of standard dialog
	a.showCustomFilePicker()
}

// processPDFAsync processes a PDF file in the background.
func (a *App) processPDFAsync(path string) {
	a.logger.Info("Processing PDF: %s", path)

	// Show loading indicator
	progressBar := widget.NewProgressBarInfinite()
	progressContent := container.NewVBox(
		widget.NewLabel(a.bundle.T("processing.message")),
		progressBar,
	)
	progress := dialog.NewCustomWithoutButtons(
		a.bundle.T("processing.title"),
		progressContent,
		a.window,
	)
	progress.Show()

	// Process in background
	go func() {
		ctx := context.Background()

		// Extract and process (background work)
		meta, err := a.extractPDFData(ctx, path)

		// UI operations in Fyne v2 are thread-safe and can be called from goroutines
		progress.Hide()

		// Wait for progress dialog to fully hide before showing next dialog
		time.Sleep(150 * time.Millisecond)

		if err != nil {
			a.logger.Error("Failed to process PDF: %v", err)

			// Check if it's "no text" error
			if err.Error() == "no text found in PDF" {
				a.handleNoTextPDF(path)
			} else {
				a.showError(
					a.bundle.T("error.processing.title"),
					a.bundle.T("error.processing.message", err.Error()),
				)
			}
			return
		}

		// Show confirmation modal with extracted data
		a.logger.Info("Showing confirmation modal for: %s", filepath.Base(path))
		a.showConfirmationModal(path, meta)
	}()
}

// processPDFBatch processes multiple PDF files.
// TODO: Implement batch processing (currently processes one at a time)
