# Scan-Eingang-Ordner Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** BuchISY überwacht einen konfigurierbaren Ordner und übernimmt neu eintreffende PDFs (z. B. NAPS2-Scans) automatisch in den bestehenden Verarbeitungsweg — ein Scan zur Zeit.

**Architecture:** Ein neuer Hintergrund-Watcher (`scanwatcher.go`) pollt den Ordner alle 5 Sekunden, erkennt vollständig geschriebene PDFs (stabile Dateigröße) und ruft den bestehenden `processSubmission` auf. Damit immer nur ein Scan gleichzeitig läuft, erhält `processSubmission` einen Abschluss-Callback, der über `showConfirmationModal`/`handleNoTextPDF` ausgelöst wird.

**Tech Stack:** Go 1.25, Fyne v2.6.3 (`fyne.Do` für Main-Thread-Marshalling), Standard-`testing`.

**Hinweis zu Commits:** Git-Commits sind in diesem Plan bewusst ausgelassen — Auslieferung per Build + Kopie der `.exe`. Jede Aufgabe endet mit `go build`/`go vet`/`go test` als Verifikation.

---

### Task 1: Einstellung „Scan-Eingang-Ordner"

**Files:**
- Modify: `internal/core/types.go`
- Modify: `internal/ui/settings.go`

- [ ] **Step 1: Add the setting field**

In `internal/core/types.go`, the `Settings` struct begins:

```go
type Settings struct {
	StorageRoot            string        `json:"storage_root"`
	UseMonthSubfolders     bool          `json:"use_month_subfolders"`
```

Insert a new field after `StorageRoot`:

```go
type Settings struct {
	StorageRoot            string        `json:"storage_root"`
	ScanInboxFolder        string        `json:"scan_inbox_folder"`
	UseMonthSubfolders     bool          `json:"use_month_subfolders"`
```

No change to `DefaultSettings()` is needed — the zero value (empty string) means the feature is off by default.

- [ ] **Step 2: Add the entry widget and browse button**

In `internal/ui/settings.go`, just after the `browseFolderBtn` definition (which ends with `}, a.window)\n\t})`), the next code is `useMonthFoldersCheck := widget.NewCheck(`. Insert the scan-inbox widgets between them. The current code:

```go
	browseFolderBtn := widget.NewButton("...", func() {
		dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
			if err != nil || uri == nil {
				return
			}
			storageRootEntry.SetText(uri.Path())
		}, a.window)
	})

	useMonthFoldersCheck := widget.NewCheck(
```

becomes:

```go
	browseFolderBtn := widget.NewButton("...", func() {
		dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
			if err != nil || uri == nil {
				return
			}
			storageRootEntry.SetText(uri.Path())
		}, a.window)
	})

	scanInboxEntry := widget.NewEntry()
	scanInboxEntry.SetText(a.settings.ScanInboxFolder)
	scanInboxEntry.SetPlaceHolder("leer = aus")

	browseScanInboxBtn := widget.NewButton("...", func() {
		dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
			if err != nil || uri == nil {
				return
			}
			scanInboxEntry.SetText(uri.Path())
		}, a.window)
	})

	useMonthFoldersCheck := widget.NewCheck(
```

- [ ] **Step 3: Add the form row in the storage tab**

In `internal/ui/settings.go`, the `generalTab` storage form currently is:

```go
		widget.NewForm(
			widget.NewFormItem(a.bundle.T("settings.targetFolder"),
				container.NewBorder(nil, nil, nil, browseFolderBtn, storageRootEntry)),
		),
```

Add a second form item:

```go
		widget.NewForm(
			widget.NewFormItem(a.bundle.T("settings.targetFolder"),
				container.NewBorder(nil, nil, nil, browseFolderBtn, storageRootEntry)),
			widget.NewFormItem("Scan-Eingang-Ordner",
				container.NewBorder(nil, nil, nil, browseScanInboxBtn, scanInboxEntry)),
		),
```

- [ ] **Step 4: Persist the setting on save**

In `internal/ui/settings.go`, the settings-save handler contains the line `newSettings.StorageRoot = storageRootEntry.Text`. Immediately after it, add:

```go
		newSettings.StorageRoot = storageRootEntry.Text
		newSettings.ScanInboxFolder = scanInboxEntry.Text
```

- [ ] **Step 5: Build and vet**

Run: `go build ./... && go vet ./...`
Expected: PASS — the new field round-trips through settings; nothing else changes behaviour yet.

---

### Task 2: Abschluss-Callback durch den Verarbeitungsweg

**Files:**
- Modify: `internal/ui/filepicker.go`
- Modify: `internal/ui/invoicemodal.go`
- Modify: `internal/ui/app.go`
- Modify: `internal/ui/custompicker.go`

- [ ] **Step 1: `showConfirmationModal` accepts and fires an `onClose` callback**

In `internal/ui/invoicemodal.go`, change the function signature:

```go
func (a *App) showConfirmationModal(originalPath string, attachments []string, meta core.Meta) {
```

to:

```go
func (a *App) showConfirmationModal(originalPath string, attachments []string, meta core.Meta, onClose func()) {
```

In the same function, the `confirmWin.SetOnClosed` handler currently is:

```go
	confirmWin.SetOnClosed(func() {
		a.settings.PreviewSplitOffset = split.Offset
		if err := a.settingsMgr.Save(a.settings); err != nil {
			a.logger.Warn("Failed to save preview split offset: %v", err)
		}
	})
```

Change it to also fire `onClose`:

```go
	confirmWin.SetOnClosed(func() {
		a.settings.PreviewSplitOffset = split.Offset
		if err := a.settingsMgr.Save(a.settings); err != nil {
			a.logger.Warn("Failed to save preview split offset: %v", err)
		}
		if onClose != nil {
			onClose()
		}
	})
```

- [ ] **Step 2: `handleNoTextPDF` accepts and forwards the callback**

In `internal/ui/app.go`, the function `handleNoTextPDF` currently is:

```go
func (a *App) handleNoTextPDF(path string, attachments []string) {
	dialog.ShowConfirm(
		a.bundle.T("error.noText"),
		a.bundle.T("error.manualEntry"),
		func(manual bool) {
			if manual {
				emptyMeta := core.Meta{
					Waehrung:   a.settings.CurrencyDefault,
					Gegenkonto: a.settings.DefaultAccount,
				}
				a.showConfirmationModal(path, attachments, emptyMeta)
			}
		},
		a.window,
	)
}
```

Replace it with:

```go
func (a *App) handleNoTextPDF(path string, attachments []string, onComplete func()) {
	dialog.ShowConfirm(
		a.bundle.T("error.noText"),
		a.bundle.T("error.manualEntry"),
		func(manual bool) {
			if manual {
				emptyMeta := core.Meta{
					Waehrung:   a.settings.CurrencyDefault,
					Gegenkonto: a.settings.DefaultAccount,
				}
				a.showConfirmationModal(path, attachments, emptyMeta, onComplete)
			} else if onComplete != nil {
				onComplete()
			}
		},
		a.window,
	)
}
```

- [ ] **Step 3: `processSubmission` accepts the callback and threads it through**

In `internal/ui/filepicker.go`, `processSubmission` currently is:

```go
func (a *App) processSubmission(mainPath string, attachments []string) {
	a.logger.Info("Processing submission: main=%s, attachments=%d", mainPath, len(attachments))

	if !core.IsPDF(mainPath) {
		emptyMeta := core.Meta{
			Waehrung:   a.settings.CurrencyDefault,
			Gegenkonto: a.settings.DefaultAccount,
		}
		a.showConfirmationModal(mainPath, attachments, emptyMeta)
		return
	}

	progress := dialog.NewProgressInfinite(
		a.bundle.T("processing.title"),
		a.bundle.T("processing.message"),
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
				a.handleNoTextPDF(mainPath, attachments)
			} else {
				a.showError(
					a.bundle.T("error.processing.title"),
					a.bundle.T("error.processing.message", err.Error()),
				)
			}
			return
		}

		a.logger.Info("Showing confirmation modal for: %s", filepath.Base(mainPath))
		a.showConfirmationModal(mainPath, attachments, meta)
	}()
}
```

Replace it with (signature gains `onComplete func()`; every terminal path triggers it):

```go
func (a *App) processSubmission(mainPath string, attachments []string, onComplete func()) {
	a.logger.Info("Processing submission: main=%s, attachments=%d", mainPath, len(attachments))

	if !core.IsPDF(mainPath) {
		emptyMeta := core.Meta{
			Waehrung:   a.settings.CurrencyDefault,
			Gegenkonto: a.settings.DefaultAccount,
		}
		a.showConfirmationModal(mainPath, attachments, emptyMeta, onComplete)
		return
	}

	progress := dialog.NewProgressInfinite(
		a.bundle.T("processing.title"),
		a.bundle.T("processing.message"),
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

		a.logger.Info("Showing confirmation modal for: %s", filepath.Base(mainPath))
		a.showConfirmationModal(mainPath, attachments, meta, onComplete)
	}()
}
```

- [ ] **Step 4: Update the existing `processSubmission` caller**

In `internal/ui/custompicker.go`, line 373 currently reads:

```go
		a.processSubmission(mainPath, attachments)
```

Change it to pass `nil` (manual picking needs no completion callback):

```go
		a.processSubmission(mainPath, attachments, nil)
```

- [ ] **Step 5: Build and vet**

Run: `go build ./... && go vet ./...`
Expected: PASS — all call sites of `processSubmission`, `showConfirmationModal`, and `handleNoTextPDF` now match the new signatures. Behaviour is unchanged for manual picking (callback is `nil`).

---

### Task 3: Der Scan-Watcher

**Files:**
- Create: `internal/ui/scanwatcher.go`
- Test: `internal/ui/scanwatcher_test.go`
- Modify: `internal/ui/app.go`

- [ ] **Step 1: Write the failing test**

Create `internal/ui/scanwatcher_test.go`:

```go
package ui

import "testing"

func TestScanFileReady(t *testing.T) {
	cases := []struct {
		name       string
		prevSize   int64
		seenBefore bool
		curSize    int64
		handled    bool
		want       bool
	}{
		{"never seen before", 0, false, 100, false, false},
		{"size still changing", 80, true, 100, false, false},
		{"stable and unhandled", 100, true, 100, false, true},
		{"stable but already handled", 100, true, 100, true, false},
	}
	for _, c := range cases {
		got := scanFileReady(c.prevSize, c.seenBefore, c.curSize, c.handled)
		if got != c.want {
			t.Errorf("%s: scanFileReady() = %v, want %v", c.name, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ui/ -run TestScanFileReady -v`
Expected: FAIL — `undefined: scanFileReady`.

- [ ] **Step 3: Create the watcher**

Create `internal/ui/scanwatcher.go`:

```go
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
	folder := strings.TrimSpace(w.app.settings.ScanInboxFolder)
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/ui/ -run TestScanFileReady -v`
Expected: PASS.

- [ ] **Step 5: Start the watcher on app startup**

In `internal/ui/app.go`, the `Run` method currently is:

```go
func (a *App) Run() {
	// Set up window close handler to save position/size
	a.window.SetOnClosed(func() {
		a.saveWindowState()
	})

	// ShowAndRun automatically brings window to front and starts event loop
	a.window.ShowAndRun()

	// Cleanup
	a.logger.Close()
}
```

Add the watcher start before `ShowAndRun`:

```go
func (a *App) Run() {
	// Set up window close handler to save position/size
	a.window.SetOnClosed(func() {
		a.saveWindowState()
	})

	// Start watching the scan-inbox folder for new PDFs.
	newScanWatcher(a).start()

	// ShowAndRun automatically brings window to front and starts event loop
	a.window.ShowAndRun()

	// Cleanup
	a.logger.Close()
}
```

- [ ] **Step 6: Build, vet, test**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS — build and vet clean; `internal/core` and `internal/ui` tests pass (incl. `TestScanFileReady`); other packages report `no test files`.

---

### Task 4: Build, Paketierung, Auslieferung

**Files:** none (build/deploy only)

- [ ] **Step 1: Final build + vet + tests**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: all succeed.

- [ ] **Step 2: Package the Windows executable**

Run (from `C:\Users\istok\Desktop\Dev\BuchISY`):
`fyne package -os windows -name BuchISY -src ./cmd/buchisy`
Expected: `cmd/buchisy/BuchISY.exe` produced.

- [ ] **Step 3: Stop the running app**

Terminate any running `BuchISY` process (PowerShell `wmic process where "ProcessId=<id>" call terminate` per running PID, as established in this session).

- [ ] **Step 4: Deploy and restart**

Copy `cmd/buchisy/BuchISY.exe` over `C:\Users\istok\Desktop\BuchISY.exe`, then launch
`C:\Users\istok\Desktop\BuchISY.exe` with working directory `C:\Users\istok\Desktop`.

- [ ] **Step 5: Manual smoke test**

1. Einstellungen → Tab „Ablage" → „Scan-Eingang-Ordner" auf
   `C:\Users\istok\Desktop\Löschen\NAPS2` setzen → Speichern.
2. Eine PDF in diesen Ordner legen → nach wenigen Sekunden öffnet BuchISY
   den Bestätigungsdialog für diese Datei.
3. Speichern → die PDF ist aus dem Eingang-Ordner in den Monatsordner
   verschoben.
4. Zwei PDFs gleichzeitig hineinlegen → sie werden nacheinander
   abgearbeitet, nicht gleichzeitig.
5. „Scan-Eingang-Ordner" wieder leeren (Feld leer) → Speichern → es wird
   nichts mehr automatisch übernommen.

---

## Self-Review

**Spec coverage:**
- `ScanInboxFolder`-Einstellung + UI-Feld im Tab „Ablage" + Speichern → Task 1.
- Watcher: Polling 5 s, Vollständigkeits-Prüfung (stabile Größe), `sizes`/`handled`-Zustände, Start vorhandener Dateien → Task 3 (`poll`, `scanFileReady`).
- Verarbeitung über bestehenden `processSubmission`; immer nur ein Scan (`busy`-Flag + Abschluss-Callback) → Task 2 (Callback) + Task 3 (`busy`, `onScanDone`).
- Threading: Polling im Goroutine, `processSubmission` über `fyne.Do` auf den Main-Thread → Task 3 (`poll`).
- Watcher-Start beim App-Start → Task 3 Step 5.
- Unit-Test der Vollständigkeits-Logik (`scanFileReady`) → Task 3 Steps 1-4.

**Placeholder scan:** Keine TBD/TODO; alle Code-Schritte enthalten vollständigen Code.

**Type consistency:** `processSubmission(string, []string, func())` (Task 2) wird in Task 2 Step 4 (custompicker, `nil`) und Task 3 (`w.onScanDone`) mit dieser Signatur aufgerufen. `showConfirmationModal(..., onClose func())` und `handleNoTextPDF(..., onComplete func())` (Task 2) werden ausschließlich aus dem in Task 2 angepassten `processSubmission`/`handleNoTextPDF` aufgerufen. `scanFileReady(int64, bool, int64, bool) bool` (Task 3) wird im Test (Step 1) und in `poll` mit dieser Signatur genutzt. `Settings.ScanInboxFolder` (Task 1) wird in Task 3 (`poll`) gelesen. `newScanWatcher(*App) *scanWatcher` / `(*scanWatcher).start()` (Task 3) werden in `Run` (Task 3 Step 5) genutzt.
