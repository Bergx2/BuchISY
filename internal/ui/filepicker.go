package ui

import (
	"context"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"
	"github.com/bergx2/buchisy/internal/core"
)

// selectPDFFiles shows a custom file picker with search functionality.
func (a *App) selectPDFFiles() {
	// Use custom picker with search instead of standard dialog
	a.showCustomFilePicker()
}

// pickerKind selects which per-profile "last used folder" the file
// picker reads its initial location from and saves to afterwards.
type pickerKind int

const (
	pickerBeleg pickerKind = iota
	pickerKontoauszug
)

// lastFolderFor returns the saved starting folder for the given picker
// kind (per-profile).
func (a *App) lastFolderFor(kind pickerKind) string {
	switch kind {
	case pickerKontoauszug:
		return a.settings.LastStatementFolder
	}
	return a.settings.LastUsedFolder
}

// rememberFolderFor persists the directory of `path` as the new
// "last used" folder for the given kind. No-op when no profile is
// active or when the value hasn't changed.
func (a *App) rememberFolderFor(kind pickerKind, path string) {
	if a.settingsMgr == nil || path == "" {
		return
	}
	dir := filepath.Dir(path)
	switch kind {
	case pickerKontoauszug:
		if a.settings.LastStatementFolder == dir {
			return
		}
		a.settings.LastStatementFolder = dir
	default:
		if a.settings.LastUsedFolder == dir {
			return
		}
		a.settings.LastUsedFolder = dir
	}
	if err := a.settingsMgr.Save(a.settings); err != nil && a.logger != nil {
		a.logger.Warn("Failed to persist last folder: %v", err)
	}
}

// showFilesPickerFor opens a multi-select file open dialog scoped to
// the given pickerKind: starts in that kind's remembered folder and
// remembers wherever the user navigates to. Native dialog on Windows;
// Fyne fallback on other platforms (single-file).
func (a *App) showFilesPickerFor(kind pickerKind, onPicked func(paths []string)) {
	initial := a.lastFolderFor(kind)
	if nativePickerAvailable() {
		paths, ok := pickFilesNative(initial, "Dateien auswählen")
		if ok && len(paths) > 0 {
			a.rememberFolderFor(kind, paths[0])
			onPicked(paths)
		}
		return
	}
	fd := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil || reader == nil {
			return
		}
		path := uriToNativePath(reader.URI())
		reader.Close()
		a.rememberFolderFor(kind, path)
		onPicked([]string{path})
	}, a.window)
	fd.Resize(fyne.NewSize(900, 700))
	if initial != "" {
		if uri := parseFolderURI(initial); uri != nil {
			fd.SetLocation(uri)
		}
	}
	fd.Show()
}

// showFilePickerFor opens a single-file picker scoped to the given
// pickerKind. Saves the folder the user picked from.
func (a *App) showFilePickerFor(kind pickerKind, onPicked func(path string)) {
	initial := a.lastFolderFor(kind)
	if nativePickerAvailable() {
		path, ok := pickFileNative(initial, "Datei auswählen")
		if ok && path != "" {
			a.rememberFolderFor(kind, path)
			onPicked(path)
		}
		return
	}
	fd := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil || reader == nil {
			return
		}
		path := uriToNativePath(reader.URI())
		reader.Close()
		a.rememberFolderFor(kind, path)
		onPicked(path)
	}, a.window)
	fd.Resize(fyne.NewSize(900, 700))
	if initial != "" {
		if uri := parseFolderURI(initial); uri != nil {
			fd.SetLocation(uri)
		}
	}
	fd.Show()
}

// showFilesPicker / showFilePicker are backward-compatible Belege-
// scoped wrappers — used by the invoice attachment "+ Anhang" flows.
func (a *App) showFilesPicker(onPicked func(paths []string)) {
	a.showFilesPickerFor(pickerBeleg, onPicked)
}

func (a *App) showFilePicker(onPicked func(path string)) {
	a.showFilePickerFor(pickerBeleg, onPicked)
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

// processSubmission processes a selected main file plus its attachments.
// A PDF main file is run through metadata extraction; a non-PDF main file
// skips extraction and opens the confirmation modal for manual entry.
func (a *App) processSubmission(mainPath string, attachments []string, onComplete func()) {
	a.logger.Info("Processing submission: main=%s, attachments=%d", mainPath, len(attachments))

	if !core.IsPDF(mainPath) {
		// Image files (jpg/png/gif/webp) can go through Claude's vision
		// extractor exactly like a PDF — pre-fills the form from a
		// screenshot. Fall back to a blank form on any failure.
		if core.ImageMediaType(mainPath) != "" && a.settings.ProcessingMode == "claude" {
			progress := dialog.NewProgressInfinite(
				a.bundle.T("processing.title"),
				a.bundle.T("processing.message"),
				a.window,
			)
			progress.Show()
			go func() {
				ctx := context.Background()
				meta, err := a.extractImageData(ctx, mainPath)
				progress.Hide()
				time.Sleep(150 * time.Millisecond)
				if err != nil {
					a.logger.Warn("Image vision extraction failed, opening blank form: %v", err)
					meta = core.Meta{
						Waehrung:   a.settings.CurrencyDefault,
						Gegenkonto: a.settings.DefaultAccount,
					}
				}
				a.showConfirmationModal(mainPath, attachments, meta, onComplete)
			}()
			return
		}

		emptyMeta := core.Meta{
			Waehrung:   a.settings.CurrencyDefault,
			Gegenkonto: a.settings.DefaultAccount,
		}
		a.showConfirmationModal(mainPath, attachments, emptyMeta, onComplete)
		return
	}

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

	go func() {
		ctx := context.Background()
		meta, err := a.extractPDFData(ctx, mainPath)
		progress.Hide()
		time.Sleep(150 * time.Millisecond)

		if err != nil {
			a.logger.Error("Failed to process PDF: %v", err)
			if err.Error() == "no text found in PDF" {
				a.handleNoTextPDF(mainPath, attachments, onComplete)
			} else {
				a.showError(
					a.bundle.T("error.processing.title"),
					a.bundle.T("error.processing.message", err.Error()),
				)
				if onComplete != nil {
					onComplete()
				}
			}
			return
		}

		// E20.5: attempt silent auto-booking when a matching rule is opt-in,
		// the receipt is plausible, and no duplicate exists.
		if tpl, ok := core.MatchAutobookRule(meta.Auftraggeber, a.bookingTemplates); ok && core.AutobookPlausible(meta) {
			// Quick duplicate pre-check using available fields (same logic saveInvoice will redo).
			dupRow := core.CSVRow{
				Auftraggeber:    meta.Auftraggeber,
				Rechnungsnummer: meta.Rechnungsnummer,
			}
			_, isDup, _ := a.dbRepo.FindDuplicate(dupRow)
			if !isDup {
				a.logger.Info("Auto-booking: %s (%s)", meta.Auftraggeber, filepath.Base(mainPath))
				if err := a.autoBookInvoice(mainPath, attachments, meta, tpl); err != nil {
					a.logger.Warn("Auto-book failed for %s: %v — opening modal", filepath.Base(mainPath), err)
					// Fall through to modal on error.
				} else {
					a.batchAutoBooked++
					a.loadInvoices()
					if onComplete != nil {
						onComplete()
					}
					return
				}
			}
		}

		a.logger.Info("Showing confirmation modal for: %s", filepath.Base(mainPath))
		a.showConfirmationModal(mainPath, attachments, meta, onComplete)
	}()
}

// enqueueSubmissions queues supported files for sequential entry. The first
// opens immediately; closing each review modal (save or cancel) opens the next.
func (a *App) enqueueSubmissions(paths []string) {
	var files []string
	for _, p := range paths {
		if core.IsSupportedFile(filepath.Base(p)) {
			files = append(files, p)
		}
	}
	if len(files) == 0 {
		return
	}
	// If a batch is already in flight (a modal is open or a file is being
	// extracted), append to the running queue instead of replacing it — that
	// avoids losing the remaining files and opening a second modal.
	if a.batchTotal > 0 {
		a.pendingFiles = append(a.pendingFiles, files...)
		a.batchTotal += len(files)
		return
	}
	a.pendingFiles = files
	a.batchTotal = len(files)
	a.batchDone = 0
	a.batchAutoBooked = 0
	a.processNextPending()
}

// processNextPending pops and processes the next queued file, chaining onComplete
// back to itself so the queue advances when each modal closes.
func (a *App) processNextPending() {
	if len(a.pendingFiles) == 0 {
		// Batch complete: show result toast when at least one file was processed.
		if a.batchTotal > 0 {
			autoN := a.batchAutoBooked
			reviewN := a.batchTotal - autoN
			if autoN > 0 {
				a.showToast(a.bundle.T("autobook.result", autoN, reviewN))
			}
		}
		a.batchTotal, a.batchDone, a.batchAutoBooked = 0, 0, 0
		return
	}
	path := a.pendingFiles[0]
	a.pendingFiles = a.pendingFiles[1:]
	a.batchDone++
	a.processSubmission(path, nil, func() { a.processNextPending() })
}
