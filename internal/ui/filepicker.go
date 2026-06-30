package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"
	"github.com/bergx2/buchisy/internal/core"
)

// processingTimeout aborts an upload/extraction that runs too long (slow API or
// a stuck render), so the user always gets a clean exit with retry options.
const processingTimeout = 180 * time.Second

// humanSize renders a byte count as "B / KB / MB".
func humanSize(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// newProcessingDialog builds the upload/extraction progress dialog: the file
// name + size, a live status line (what's happening right now), an infinite
// progress bar, and a Cancel button. Returns the dialog and a thread-safe
// status setter to call from the extraction goroutine.
func (a *App) newProcessingDialog(mainPath string) (dlg *dialog.CustomDialog, setStatus func(string), stop func()) {
	fileLbl := widget.NewLabelWithStyle(filepath.Base(mainPath), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	fileLbl.Wrapping = fyne.TextWrapWord
	sizeStr := "—"
	if fi, err := os.Stat(mainPath); err == nil {
		sizeStr = humanSize(fi.Size())
	}
	sizeLbl := widget.NewLabel(sizeStr)
	statusLbl := widget.NewLabel(a.bundle.T("processing.message"))
	statusLbl.Wrapping = fyne.TextWrapWord
	elapsedLbl := widget.NewLabel("läuft seit 0:00")
	elapsedLbl.Importance = widget.LowImportance
	hintLbl := widget.NewLabel("")
	hintLbl.Wrapping = fyne.TextWrapWord
	hintLbl.Importance = widget.WarningImportance
	hintLbl.Hide()
	content := container.NewVBox(
		fileLbl,
		sizeLbl,
		widget.NewSeparator(),
		statusLbl,
		widget.NewProgressBarInfinite(),
		elapsedLbl,
		hintLbl,
	)
	dlg = dialog.NewCustom(a.bundle.T("processing.title"), a.bundle.T("btn.cancel"), content, a.window)
	dlg.Resize(fyne.NewSize(600, 280))
	setStatus = func(s string) {
		fyne.Do(func() { statusLbl.SetText(s) })
	}

	// Live elapsed timer + a "taking longer than usual" hint after 30 s.
	stopCh := make(chan struct{})
	go func() {
		start := time.Now()
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-stopCh:
				return
			case <-t.C:
				el := time.Since(start)
				fyne.Do(func() {
					elapsedLbl.SetText(fmt.Sprintf("läuft seit %d:%02d", int(el.Minutes()), int(el.Seconds())%60))
					if el > 30*time.Second {
						hintLbl.SetText("Dauert länger als üblich – bei einem großen oder gescannten PDF kann das Rendern/Senden dauern. Du kannst jederzeit abbrechen.")
						hintLbl.Show()
					}
				})
			}
		}
	}()
	var once sync.Once
	stop = func() { once.Do(func() { close(stopCh) }) }
	return dlg, setStatus, stop
}

// showProcessingError reports an extraction failure (or timeout) with actionable
// choices: retry, enter manually (blank form), or close.
func (a *App) showProcessingError(mainPath string, attachments []string, onComplete func(), msg string) {
	var d *dialog.CustomDialog
	retryBtn := widget.NewButton(a.bundle.T("btn.retry"), func() {
		d.Hide()
		a.processSubmission(mainPath, attachments, onComplete)
	})
	retryBtn.Importance = widget.HighImportance
	manualBtn := widget.NewButton("Manuell erfassen", func() {
		d.Hide()
		a.showConfirmationModal(mainPath, attachments, core.Meta{
			Waehrung:   a.settings.CurrencyDefault,
			Gegenkonto: a.settings.DefaultAccount,
		}, onComplete)
	})
	closeBtn := widget.NewButton(a.bundle.T("btn.cancel"), func() {
		d.Hide()
		if onComplete != nil {
			onComplete()
		}
	})
	lbl := widget.NewLabel(msg)
	lbl.Wrapping = fyne.TextWrapWord
	content := container.NewVBox(lbl, container.NewHBox(retryBtn, manualBtn, closeBtn))
	d = dialog.NewCustomWithoutButtons(a.bundle.T("error.processing.title"), content, a.window)
	d.Resize(fyne.NewSize(540, 220))
	d.Show()
}

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
			ctx, cancel := context.WithTimeout(context.Background(), processingTimeout)
			var done, canceled atomic.Bool
			progress, setStatus, stopTimer := a.newProcessingDialog(mainPath)
			progress.SetOnClosed(func() {
				stopTimer()
				if done.Load() {
					return
				}
				canceled.Store(true)
				cancel()
				a.logger.Info("Bild-Verarbeitung abgebrochen: %s", mainPath)
				if onComplete != nil {
					onComplete()
				}
			})
			progress.Show()
			go func() {
				meta, err := a.extractImageData(ctx, mainPath, setStatus)
				if canceled.Load() {
					return
				}
				done.Store(true)
				stopTimer()
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

	// Show loading indicator WITH a cancel button: extraction can stall (slow
	// API, scanned-PDF rendering), so the user must be able to abort. The cancel
	// button cancels the context (aborts the Claude request) and dismisses the
	// dialog; the goroutine then discards any late result.
	ctx, cancel := context.WithTimeout(context.Background(), processingTimeout)
	var done, canceled atomic.Bool
	progress, setStatus, stopTimer := a.newProcessingDialog(mainPath)
	progress.SetOnClosed(func() {
		stopTimer()
		if done.Load() {
			return // dialog closed by us after a normal finish
		}
		canceled.Store(true)
		cancel()
		a.logger.Info("PDF-Verarbeitung abgebrochen: %s", mainPath)
		if onComplete != nil {
			onComplete()
		}
	})
	progress.Show()

	go func() {
		meta, err := a.extractPDFData(ctx, mainPath, setStatus)
		if canceled.Load() {
			return // user aborted; dialog + onComplete already handled
		}
		done.Store(true)
		stopTimer()
		progress.Hide()
		time.Sleep(150 * time.Millisecond)

		// Auto-timeout: the context's deadline fired before the user cancelled.
		if ctx.Err() == context.DeadlineExceeded {
			a.logger.Warn("PDF-Verarbeitung Zeitüberschreitung: %s", mainPath)
			a.showProcessingError(mainPath, attachments, onComplete,
				"Zeitüberschreitung bei der Verarbeitung (über 3 Minuten). Möglicherweise ist das PDF sehr groß/gescannt oder die Verbindung langsam.")
			return
		}

		if err != nil {
			a.logger.Error("Failed to process PDF: %v", err)
			if err.Error() == "no text found in PDF" {
				a.handleNoTextPDF(mainPath, attachments, onComplete)
			} else {
				a.showProcessingError(mainPath, attachments, onComplete,
					a.bundle.T("error.processing.message", err.Error()))
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
