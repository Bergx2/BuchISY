# Original-Handhabung beim Speichern — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Beim Ablegen einer Rechnung bleibt die Original-Datei eines Datei-Uploads erhalten; nur Dateien aus dem Scan-Eingangsordner werden verschoben. Eine in einem anderen Programm gesperrte Datei lässt das Speichern nicht mehr scheitern.

**Architecture:** `core/storage.go` bekommt neben `MoveAndRename` (verschieben) ein `CopyAndRename` (kopieren, Original behalten); beide nutzen einen gemeinsamen Helfer für Zielordner und Kollisionsnamen. `saveInvoice` wählt je Quelle (Scan-Ordner ja/nein) zwischen beiden.

**Tech Stack:** Go 1.25, Fyne v2.6.3, Standard-`testing`.

**Hinweis zu Commits:** Git-Commits sind in diesem Plan bewusst ausgelassen. Jede Aufgabe endet mit `go build`/`go vet`/`go test`.

---

### Task 1: `CopyAndRename` + toleranter `MoveAndRename` (TDD)

**Files:**
- Modify: `internal/core/storage.go`
- Test: `internal/core/storage_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/core/storage_test.go` (die Datei existiert; `os`, `path/filepath`, `testing` sind im Import-Block):

```go
func TestCopyAndRename(t *testing.T) {
	src := filepath.Join(t.TempDir(), "quelle.pdf")
	if err := os.WriteFile(src, []byte("inhalt"), 0644); err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	sm := NewStorageManager(&Settings{})

	name, err := sm.CopyAndRename(src, target, "ziel.pdf")
	if err != nil {
		t.Fatalf("CopyAndRename: %v", err)
	}
	if name != "ziel.pdf" {
		t.Errorf("name = %q, want ziel.pdf", name)
	}
	if _, err := os.Stat(filepath.Join(target, "ziel.pdf")); err != nil {
		t.Errorf("target file missing: %v", err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Errorf("source file must still exist after copy: %v", err)
	}

	// Collision -> _2 suffix.
	name2, err := sm.CopyAndRename(src, target, "ziel.pdf")
	if err != nil {
		t.Fatalf("CopyAndRename collision: %v", err)
	}
	if name2 != "ziel_2.pdf" {
		t.Errorf("collision name = %q, want ziel_2.pdf", name2)
	}
}

func TestMoveAndRenameRemovesSource(t *testing.T) {
	src := filepath.Join(t.TempDir(), "quelle.pdf")
	if err := os.WriteFile(src, []byte("inhalt"), 0644); err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	sm := NewStorageManager(&Settings{})

	name, err := sm.MoveAndRename(src, target, "ziel.pdf")
	if err != nil {
		t.Fatalf("MoveAndRename: %v", err)
	}
	if name != "ziel.pdf" {
		t.Errorf("name = %q, want ziel.pdf", name)
	}
	if _, err := os.Stat(filepath.Join(target, "ziel.pdf")); err != nil {
		t.Errorf("target file missing: %v", err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Errorf("source file should be gone after move")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run "TestCopyAndRename|TestMoveAndRenameRemovesSource" -v`
Expected: FAIL — `CopyAndRename` ist undefiniert.

- [ ] **Step 3: Implement**

In `internal/core/storage.go` ist die aktuelle Funktion `MoveAndRename` genau dieser Block:

```go
// MoveAndRename moves a file to the target location with a new name.
// It handles collisions by appending _2, _3, etc.
func (sm *StorageManager) MoveAndRename(sourcePath, targetFolder, newName string) (string, error) {
	// Ensure target folder exists
	if err := os.MkdirAll(targetFolder, 0755); err != nil {
		return "", fmt.Errorf("failed to create target folder: %w", err)
	}

	// Handle filename collisions
	finalName := newName
	targetPath := filepath.Join(targetFolder, finalName)
	counter := 2

	for {
		if _, err := os.Stat(targetPath); os.IsNotExist(err) {
			break
		}

		// File exists, try with counter
		ext := filepath.Ext(newName)
		base := newName[:len(newName)-len(ext)]
		finalName = fmt.Sprintf("%s_%d%s", base, counter, ext)
		targetPath = filepath.Join(targetFolder, finalName)
		counter++
	}

	// Move the file
	if err := os.Rename(sourcePath, targetPath); err != nil {
		// If rename fails (e.g., cross-device), try copy + delete
		if err := copyFile(sourcePath, targetPath); err != nil {
			return "", fmt.Errorf("failed to copy file: %w", err)
		}
		if err := os.Remove(sourcePath); err != nil {
			return "", fmt.Errorf("failed to remove source file: %w", err)
		}
	}

	return finalName, nil
}
```

Ersetze diesen ganzen Block durch:

```go
// prepareTarget ensures targetFolder exists and returns a collision-free
// final name and full path within it (appending _2, _3, … as needed).
func prepareTarget(targetFolder, newName string) (finalName, targetPath string, err error) {
	if err := os.MkdirAll(targetFolder, 0755); err != nil {
		return "", "", fmt.Errorf("failed to create target folder: %w", err)
	}
	finalName = newName
	targetPath = filepath.Join(targetFolder, finalName)
	counter := 2
	for {
		if _, statErr := os.Stat(targetPath); os.IsNotExist(statErr) {
			break
		}
		ext := filepath.Ext(newName)
		base := newName[:len(newName)-len(ext)]
		finalName = fmt.Sprintf("%s_%d%s", base, counter, ext)
		targetPath = filepath.Join(targetFolder, finalName)
		counter++
	}
	return finalName, targetPath, nil
}

// MoveAndRename moves a file to the target location with a new name,
// handling collisions with _2, _3, … suffixes. If the source cannot be
// removed after a fallback copy (e.g. it is locked by another program),
// the operation still counts as successful — the file is already at its
// destination.
func (sm *StorageManager) MoveAndRename(sourcePath, targetFolder, newName string) (string, error) {
	finalName, targetPath, err := prepareTarget(targetFolder, newName)
	if err != nil {
		return "", err
	}
	if err := os.Rename(sourcePath, targetPath); err != nil {
		// Rename failed (cross-device, or source locked): copy instead.
		if err := copyFile(sourcePath, targetPath); err != nil {
			return "", fmt.Errorf("failed to copy file: %w", err)
		}
		// Best-effort source removal; a locked source must not fail the op.
		_ = os.Remove(sourcePath)
	}
	return finalName, nil
}

// CopyAndRename copies a file to the target location with a new name,
// leaving the source file untouched. Collisions get _2, _3, … suffixes.
func (sm *StorageManager) CopyAndRename(sourcePath, targetFolder, newName string) (string, error) {
	finalName, targetPath, err := prepareTarget(targetFolder, newName)
	if err != nil {
		return "", err
	}
	if err := copyFile(sourcePath, targetPath); err != nil {
		return "", fmt.Errorf("failed to copy file: %w", err)
	}
	return finalName, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run "TestCopyAndRename|TestMoveAndRenameRemovesSource" -v`
Expected: PASS.

- [ ] **Step 5: Build, vet, full core tests**

Run: `go build ./... && go vet ./internal/core/... && go test ./internal/core/...`
Expected: PASS.

---

### Task 2: `saveInvoice` wählt Kopieren vs. Verschieben je Quelle

**Files:**
- Modify: `internal/ui/invoicemodal.go`

- [ ] **Step 1: Add the scan-source detection and file-placement helpers**

In `internal/ui/invoicemodal.go`, am Dateiende, anfügen (`strings` und `path/filepath` sind bereits importiert):

```go
// isFromScanInbox reports whether path lies inside the configured scan
// inbox folder.
func (a *App) isFromScanInbox(path string) bool {
	inbox := strings.TrimSpace(a.settings.ScanInboxFolder)
	if inbox == "" {
		return false
	}
	absInbox, err1 := filepath.Abs(inbox)
	absPath, err2 := filepath.Abs(path)
	if err1 != nil || err2 != nil {
		return false
	}
	return strings.HasPrefix(absPath, absInbox+string(filepath.Separator))
}

// placeFile files a source file into targetFolder under newName. A file
// from the scan inbox is moved (original removed); any other file is
// copied (original kept). Returns the final, collision-free name.
func (a *App) placeFile(sourcePath, targetFolder, newName string) (string, error) {
	if a.isFromScanInbox(sourcePath) {
		return a.storageManager.MoveAndRename(sourcePath, targetFolder, newName)
	}
	return a.storageManager.CopyAndRename(sourcePath, targetFolder, newName)
}
```

- [ ] **Step 2: Use `placeFile` for the main invoice file**

In `internal/ui/invoicemodal.go`, in `saveInvoice`'s `completeSave` closure, the main file is currently filed like this:

```go
		// Move and rename the main invoice file
		finalFilename, err := a.storageManager.MoveAndRename(originalPath, targetFolder, filename)
		if err != nil {
			return fmt.Errorf("failed to move file: %w", err)
		}
```

Replace it with:

```go
		// File the main invoice file (copy for uploads, move for scans).
		finalFilename, err := a.placeFile(originalPath, targetFolder, filename)
		if err != nil {
			return fmt.Errorf("failed to save file: %w", err)
		}
```

- [ ] **Step 3: Use `placeFile` for attachments**

In the same `completeSave` closure, the attachment loop currently has:

```go
			if _, mvErr := a.storageManager.MoveAndRename(attPath, targetFolder, attName); mvErr != nil {
```

Replace that line with:

```go
			if _, mvErr := a.placeFile(attPath, targetFolder, attName); mvErr != nil {
```

- [ ] **Step 4: Build, vet, test**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS.

---

### Task 3: Build, Paketierung, Auslieferung

**Files:** none (build/deploy only)

- [ ] **Step 1: Final build + vet + tests**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: all succeed.

- [ ] **Step 2: Package the Windows executable**

Run (from `C:\Users\istok\Desktop\Dev\BuchISY`):
`fyne package -os windows -name BuchISY -src ./cmd/buchisy`
Expected: `cmd/buchisy/BuchISY.exe` produced.

- [ ] **Step 3: Stop the running app**

Terminate any running `BuchISY` process (PowerShell `wmic process where "ProcessId=<id>" call terminate` per running PID).

- [ ] **Step 4: Deploy and restart**

Copy `cmd/buchisy/BuchISY.exe` over `C:\Users\istok\Desktop\BuchISY.exe`, then launch
`C:\Users\istok\Desktop\BuchISY.exe` with working directory `C:\Users\istok\Desktop`.

- [ ] **Step 5: Manual smoke test**

1. Eine PDF per Dateiauswahl hochladen und speichern → Rechnung ist abgelegt; die hochgeladene Original-Datei liegt **unverändert** am Ursprungsort.
2. Eine PDF hochladen, die in Adobe Acrobat geöffnet ist → Speichern funktioniert **ohne Fehler**.
3. Eine PDF in den konfigurierten Scan-Ordner legen → nach der Verarbeitung ist sie **aus dem Scan-Ordner verschwunden** und im Ablageordner abgelegt.

---

## Self-Review

**Spec coverage:**
- Kopier-Variante `CopyAndRename` + gemeinsamer Helfer `prepareTarget` → Task 1.
- `MoveAndRename` toleriert fehlgeschlagenes Löschen des Originals → Task 1 (`_ = os.Remove(...)`).
- Quellprüfung in `saveInvoice` (Scan-Ordner → verschieben, sonst kopieren) → Task 2 (`isFromScanInbox`, `placeFile`), für Hauptdatei (Step 2) und Anhänge (Step 3).
- Upload-Fall kann nicht mehr an gesperrter Datei scheitern → folgt aus Task 2 (Upload → `CopyAndRename`, kein Löschen).
- Nicht betroffen: `updateInvoice` nutzt weiterhin `MoveAndRename` unverändert → in diesem Plan nicht angefasst.
- Tests `CopyAndRename` (Original bleibt, Kollision) und `MoveAndRename` (Original weg) → Task 1.

**Placeholder scan:** Keine TBD/TODO; alle Code-Schritte enthalten vollständigen Code. Die zu ersetzenden Blöcke sind verbatim angegeben.

**Type consistency:** `prepareTarget(string, string) (string, string, error)` wird von `MoveAndRename` und `CopyAndRename` genutzt. `CopyAndRename(string, string, string) (string, error)` und `MoveAndRename(string, string, string) (string, error)` haben dieselbe Signatur — `placeFile` (Task 2) gibt beide unverändert weiter. `isFromScanInbox(string) bool` und `placeFile(string, string, string) (string, error)` sind Methoden auf `*App` im Paket `ui`; `completeSave` ruft `a.placeFile(...)` für Haupt­datei und Anhänge auf. Das von `MoveAndRename`/`CopyAndRename` zurückgegebene `finalFilename` wird in `completeSave` wie bisher für `meta.Dateiname`, `newRow.Dateiname` und `core.AttachmentName` genutzt.
