package ui

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
)

// scanFileReady reports whether a watched file should be processed now: it
// must have been seen before with an identical size (fully written) and not
// yet have been handled.
func scanFileReady(prevSize int64, seenBefore bool, curSize int64, handled bool) bool {
	return seenBefore && !handled && prevSize == curSize
}

// scanWatcher polls the configured scan-inbox folder and feeds new, fully
// written PDFs into the normal processing flow, one at a time.
type scanWatcher struct {
	app     *App
	mu      sync.Mutex
	sizes   map[string]int64 // path -> last observed size
	handled map[string]bool  // paths already dispatched this session
	busy    bool             // a scan is currently being processed
}

// newScanWatcher creates a watcher bound to the given app.
func newScanWatcher(app *App) *scanWatcher {
	return &scanWatcher{
		app:     app,
		sizes:   make(map[string]int64),
		handled: make(map[string]bool),
	}
}

// start launches the polling loop in a background goroutine.
func (w *scanWatcher) start() {
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			w.poll()
		}
	}()
}

// onScanDone clears the busy flag so the next scan can be processed.
func (w *scanWatcher) onScanDone() {
	w.mu.Lock()
	w.busy = false
	w.mu.Unlock()
}

// poll checks the inbox folder once and dispatches at most one ready PDF.
func (w *scanWatcher) poll() {
	// Read the setting on the Fyne main thread — a.settings is otherwise
	// only ever touched there (incl. the settings-save), so this avoids a
	// data race with a concurrent settings save.
	var folder string
	fyne.DoAndWait(func() {
		folder = strings.TrimSpace(w.app.settings.ScanInboxFolder)
	})
	if folder == "" {
		return
	}
	info, err := os.Stat(folder)
	if err != nil || !info.IsDir() {
		return
	}
	entries, err := os.ReadDir(folder)
	if err != nil {
		return
	}

	w.mu.Lock()
	var candidate string
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".pdf") {
			continue
		}
		path := filepath.Join(folder, e.Name())
		fi, err := e.Info()
		if err != nil {
			continue
		}
		curSize := fi.Size()
		prevSize, seenBefore := w.sizes[path]
		if candidate == "" && scanFileReady(prevSize, seenBefore, curSize, w.handled[path]) {
			candidate = path
		}
		w.sizes[path] = curSize
	}
	dispatch := !w.busy && candidate != ""
	if dispatch {
		w.busy = true
		w.handled[candidate] = true
	}
	w.mu.Unlock()

	if dispatch {
		fyne.Do(func() {
			w.app.processSubmission(candidate, nil, w.onScanDone)
		})
	}
}
