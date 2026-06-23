# E17.3 — Stapel-Eingabe von Belegen (#1)

> REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Steps use `- [ ]`.

**Goal:** Enter many receipts in one go: dropping/selecting N files queues them and opens the review modal for each in turn ("Übernehmen & nächste" = closing one opens the next), with an "X/N" progress hint.

**Tech:** Go 1.25, Fyne. `internal/ui`. Branch `feat/e17-3-stapel-eingabe`.

## Global Constraints

- `go build ./...`, `go test ./...`. Commit per task. Co-author trailer: `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`.
- Reuse existing infra: `processSubmission(mainPath, attachments, onComplete func())` already calls `onComplete` from the modal's `confirmWin.SetOnClosed` (fires on save OR cancel). `showFilesPicker(onPicked func(paths []string))` is a native multi-select dialog. `core.IsSupportedFile(base)` filters.
- Single-file flows (custom picker, scan watcher, settings/edit attachment pickers) must be unchanged.

---

### Task 1: Submission queue + drop fix + multi-select entry

**Files:** `internal/ui/app.go` (App struct + drop handler + a menu item), `internal/ui/filepicker.go` (queue helpers), `internal/ui/invoicemodal.go` (X/N title hint).

- [ ] **Step 1:** Add to the `App` struct: `pendingFiles []string`, `batchTotal int`, `batchDone int`.

- [ ] **Step 2:** Add to `filepicker.go`:

```go
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
	a.pendingFiles = files
	a.batchTotal = len(files)
	a.batchDone = 0
	a.processNextPending()
}

// processNextPending pops and processes the next queued file, chaining onComplete
// back to itself so the queue advances when each modal closes.
func (a *App) processNextPending() {
	if len(a.pendingFiles) == 0 {
		a.batchTotal, a.batchDone = 0, 0
		return
	}
	path := a.pendingFiles[0]
	a.pendingFiles = a.pendingFiles[1:]
	a.batchDone++
	a.processSubmission(path, nil, func() { a.processNextPending() })
}
```

Ensure `filepath` and `core` are imported in filepicker.go (they are used elsewhere in the package; add to this file's imports if missing).

- [ ] **Step 3:** Fix the drop handler in `app.go` (currently a `for` loop with an early `return` that processes only the FIRST file). Replace its body so ALL dropped supported files are handled: in `konten` view keep the single-statement behaviour (`a.fileStatement(uris[0].Path())` for the first supported file), otherwise collect every supported path and call `a.enqueueSubmissions(paths)`. Read the current handler (~app.go:568-582) and preserve the `viewMode == "konten"` branch.

- [ ] **Step 4:** Add a menu item near the existing invoice actions (the menu built around `app.go:730`), label `"Mehrere Belege importieren…"`, that calls:

```go
a.showFilesPicker(func(paths []string) { a.enqueueSubmissions(paths) })
```

- [ ] **Step 5: X/N hint.** In `invoicemodal.go` where the modal window title is set (`confirmWin = a.app.NewWindow(a.bundle.T("modal.title"))`, ~line 765), when `a.batchTotal > 1` set the title to `fmt.Sprintf("%s (%d/%d)", a.bundle.T("modal.title"), a.batchDone, a.batchTotal)`. (Read `a.batchDone/a.batchTotal` — they are set before the modal opens.)

- [ ] **Step 6:** `go build ./... && go test ./...`. Manually reason through: dropping 3 PDFs → modal opens for #1 titled "(1/3)"; closing it (save or cancel) opens #2 "(2/3)", then #3; after #3 the queue clears. Single-file drop → batchTotal stays >0 but equals 1, title unchanged. Commit `E17.3: batch receipt entry — queue, multi-drop fix, multi-select import, X/N hint`.

## Self-Review

Coverage: #1 → Task 1. The "Übernehmen & nächste" behaviour falls out of `onComplete` firing on modal close (no new button needed). Cancelling a file advances to the next (skip) — acceptable for a review-each flow. Single-file paths (scanwatcher, custom picker, attachment pickers) pass their own onComplete/nil and never touch the queue. No core/db changes.
