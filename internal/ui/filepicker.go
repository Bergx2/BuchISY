package ui

import (
	"context"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"

	"github.com/bergx2/buchisy/internal/core"
)

// selectPDFFiles shows a custom file picker with search functionality.
func (a *App) selectPDFFiles() {
	// Use custom picker with search instead of standard dialog
	a.showCustomFilePicker()
}

// parseFolderURI converts a folder path to a Fyne URI.
func parseFolderURI(path string) fyne.ListableURI {
	uri, err := storage.ParseURI("file://" + path)
	if err != nil {
		return nil
	}
	listable, ok := uri.(fyne.ListableURI)
	if !ok {
		return nil
	}
	return listable
}

// getDocumentsURI returns the URI for the Documents folder.
func getDocumentsURI() (fyne.ListableURI, error) {
	docsDir, err := core.GetDocumentsDir()
	if err != nil {
		return nil, err
	}
	return parseFolderURI(docsDir), nil
}

// processPDFAsync processes a PDF file in the background.
func (a *App) processPDFAsync(path string) {
	a.logger.Info("Processing PDF: %s", path)

	// Show loading indicator
	progress := dialog.NewProgressInfinite(
		a.bundle.T("processing.title"),
		a.bundle.T("processing.message"),
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
