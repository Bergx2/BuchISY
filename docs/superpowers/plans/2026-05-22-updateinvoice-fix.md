# Fix `updateInvoice` — Monatswechsel — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Beim Bearbeiten/Verschieben einer Rechnung wird die Datei zuerst robust verschoben und erst danach die CSVs aktualisiert (Upsert), sodass ein Fehlschlag weder Daten verfälscht noch Doppeleinträge erzeugt.

**Architecture:** `updateInvoice` wird umgebaut: Datei-Verschieben via `StorageManager.MoveAndRename` (robust, Kopier-Fallback) VOR den CSV-Schreibvorgängen; die CSV-Aktualisierung entfernt vorhandene Einträge gleichen Dateinamens vor dem Anhängen (Upsert).

**Tech Stack:** Go 1.25, Fyne v2.6.3.

**Hinweis zu Commits:** Git-Commits sind in diesem Plan bewusst ausgelassen. Jede Aufgabe endet mit `go build`/`go vet`/`go test`.

---

### Task 1: `updateInvoice` — Reihenfolge, robustes Verschieben, Upsert

**Files:**
- Modify: `internal/ui/tableedit.go`

- [ ] **Step 1: Replace the move/CSV section of `updateInvoice`**

In `internal/ui/tableedit.go`, `updateInvoice` currently has this block (it begins at the comment `// Target folder (may differ from the source month).` and ends with the `a.showInfo("Gespeichert", ...)` line):

```go
	// Target folder (may differ from the source month).
	targetFolder := a.storageManager.GetMonthFolder(targetYear, targetMonth)
	unterordner := a.invoiceSubfolder(bankAccount, ausgangsrechnung)
	if unterordner != "" {
		targetFolder = filepath.Join(targetFolder, unterordner)
	}
	if err := os.MkdirAll(targetFolder, 0755); err != nil {
		return fmt.Errorf("failed to create target folder: %w", err)
	}
	sameMonth := targetYear == a.currentYear && targetMonth == a.currentMonth

	// Determine the final path and resolve collisions WITHOUT moving the
	// file yet — the actual move happens after the CSV writes succeed.
	finalName := newFilename
	finalPath := filepath.Join(targetFolder, finalName)
	needsMove := finalPath != originalPath
	if needsMove {
		counter := 2
		for {
			if _, err := os.Stat(finalPath); os.IsNotExist(err) {
				break
			}
			ext := filepath.Ext(newFilename)
			base := strings.TrimSuffix(newFilename, ext)
			finalName = fmt.Sprintf("%s_%d%s", base, counter, ext)
			finalPath = filepath.Join(targetFolder, finalName)
			counter++
		}
	}

	newRow := newMeta.ToCSVRow()
	newRow.Dateiname = finalName
	newRow.HatAnhaenge = originalRow.HatAnhaenge
	newRow.AnzahlAnhaenge = originalRow.AnzahlAnhaenge
	newRow.Unterordner = unterordner

	sourceCSV := a.storageManager.GetCSVPath(a.currentYear, a.currentMonth)

	if sameMonth {
		// Update the row in place in the single CSV.
		rows, err := a.csvRepo.Load(sourceCSV)
		if err != nil {
			return fmt.Errorf("failed to load CSV: %w", err)
		}
		found := false
		for i := range rows {
			if rows[i].Dateiname == originalRow.Dateiname {
				rows[i] = newRow
				found = true
			}
		}
		if !found {
			return fmt.Errorf("original row not found in CSV")
		}
		if err := a.rewriteCSV(sourceCSV, rows); err != nil {
			return fmt.Errorf("failed to update CSV: %w", err)
		}
	} else {
		// Remove from the source CSV, add to the target CSV.
		srcRows, err := a.csvRepo.Load(sourceCSV)
		if err != nil {
			return fmt.Errorf("failed to load source CSV: %w", err)
		}
		kept := make([]core.CSVRow, 0, len(srcRows))
		for _, r := range srcRows {
			if r.Dateiname != originalRow.Dateiname {
				kept = append(kept, r)
			}
		}
		if err := a.rewriteCSV(sourceCSV, kept); err != nil {
			return fmt.Errorf("failed to update source CSV: %w", err)
		}
		targetCSV := a.storageManager.GetCSVPath(targetYear, targetMonth)
		tgtRows, err := a.csvRepo.Load(targetCSV)
		if err != nil {
			return fmt.Errorf("failed to load target CSV: %w", err)
		}
		tgtRows = append(tgtRows, newRow)
		if err := a.rewriteCSV(targetCSV, tgtRows); err != nil {
			return fmt.Errorf("failed to update target CSV: %w", err)
		}
	}

	// Move/rename the main file only after all CSV writes succeeded. A CSV
	// failure leaves the file untouched at originalPath so the user can retry.
	if needsMove {
		if err := os.Rename(originalPath, finalPath); err != nil {
			return fmt.Errorf("failed to move file: %w", err)
		}
	}

	a.logger.Info("Updated invoice: %s", finalName)
	a.showInfo("Gespeichert", fmt.Sprintf("Rechnung wurde aktualisiert: %s", finalName))
```

Replace that entire block with:

```go
	// Target folder (may differ from the source month).
	targetFolder := a.storageManager.GetMonthFolder(targetYear, targetMonth)
	unterordner := a.invoiceSubfolder(bankAccount, ausgangsrechnung)
	if unterordner != "" {
		targetFolder = filepath.Join(targetFolder, unterordner)
	}
	if err := os.MkdirAll(targetFolder, 0755); err != nil {
		return fmt.Errorf("failed to create target folder: %w", err)
	}
	sameMonth := targetYear == a.currentYear && targetMonth == a.currentMonth

	// Move the file FIRST — before any CSV write — so a move failure
	// leaves the CSVs and the invoice untouched and retryable. The move
	// is robust (copy fallback) via MoveAndRename, which also resolves
	// name collisions and returns the final name.
	finalName := newFilename
	intendedPath := filepath.Join(targetFolder, newFilename)
	if intendedPath != originalPath {
		if _, statErr := os.Stat(originalPath); statErr == nil {
			moved, mvErr := a.storageManager.MoveAndRename(originalPath, targetFolder, newFilename)
			if mvErr != nil {
				return fmt.Errorf("failed to move file: %w", mvErr)
			}
			finalName = moved
		} else if _, tgtErr := os.Stat(intendedPath); tgtErr != nil {
			// Source is gone and the file is not at the target either.
			return fmt.Errorf("Quelldatei nicht gefunden: %s", originalPath)
		}
		// else: source gone but the file is already at intendedPath — a
		// prior attempt moved it; treat the move as already done.
	}

	newRow := newMeta.ToCSVRow()
	newRow.Dateiname = finalName
	newRow.HatAnhaenge = originalRow.HatAnhaenge
	newRow.AnzahlAnhaenge = originalRow.AnzahlAnhaenge
	newRow.Unterordner = unterordner

	sourceCSV := a.storageManager.GetCSVPath(a.currentYear, a.currentMonth)

	if sameMonth {
		// Upsert in the single CSV: drop any row for the old or the new
		// filename, then append the updated row.
		rows, err := a.csvRepo.Load(sourceCSV)
		if err != nil {
			return fmt.Errorf("failed to load CSV: %w", err)
		}
		updated := make([]core.CSVRow, 0, len(rows)+1)
		for _, r := range rows {
			if r.Dateiname == originalRow.Dateiname || r.Dateiname == finalName {
				continue
			}
			updated = append(updated, r)
		}
		updated = append(updated, newRow)
		if err := a.rewriteCSV(sourceCSV, updated); err != nil {
			return fmt.Errorf("failed to update CSV: %w", err)
		}
	} else {
		// Remove the row from the source CSV.
		srcRows, err := a.csvRepo.Load(sourceCSV)
		if err != nil {
			return fmt.Errorf("failed to load source CSV: %w", err)
		}
		kept := make([]core.CSVRow, 0, len(srcRows))
		for _, r := range srcRows {
			if r.Dateiname != originalRow.Dateiname {
				kept = append(kept, r)
			}
		}
		if err := a.rewriteCSV(sourceCSV, kept); err != nil {
			return fmt.Errorf("failed to update source CSV: %w", err)
		}
		// Upsert into the target CSV: drop any existing row with the same
		// filename first, so a repeated save cannot create a duplicate.
		targetCSV := a.storageManager.GetCSVPath(targetYear, targetMonth)
		tgtRows, err := a.csvRepo.Load(targetCSV)
		if err != nil {
			return fmt.Errorf("failed to load target CSV: %w", err)
		}
		merged := make([]core.CSVRow, 0, len(tgtRows)+1)
		for _, r := range tgtRows {
			if r.Dateiname != finalName {
				merged = append(merged, r)
			}
		}
		merged = append(merged, newRow)
		if err := a.rewriteCSV(targetCSV, merged); err != nil {
			return fmt.Errorf("failed to update target CSV: %w", err)
		}
	}

	a.logger.Info("Updated invoice: %s", finalName)
	a.showInfo("Gespeichert", fmt.Sprintf("Rechnung wurde aktualisiert: %s", finalName))
```

Key changes: the file is moved (via the robust `MoveAndRename`, which has a copy fallback and resolves collisions) **before** any CSV write; a move failure returns early with the CSVs untouched; the CSV updates are now **upserts** (existing rows with the same filename are dropped before appending), so a repeated save cannot duplicate the entry; a source that is already gone but present at the target is treated as already-moved.

- [ ] **Step 2: Build and vet**

Run: `go build ./... && go vet ./...`
Expected: PASS. If the build reports `strings` as now-unused in `tableedit.go`, leave it — `strings` is used elsewhere in that file (`showEditDialog`); only remove an import the compiler actually flags.

- [ ] **Step 3: Run tests**

Run: `go test ./...`
Expected: PASS.

---

### Task 2: Build, Paketierung, Auslieferung + Selbstheilung prüfen

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

- [ ] **Step 5: Verify the self-heal of the existing inconsistency**

The euhost invoice is currently inconsistent (PDF in `…\2026\2026-04\`, entry in `2026-04\invoices.csv` 1×, in `2026-05\invoices.csv` 2×). After deployment, the user re-does the April→May Ablage change once. Then verify on disk:
- `…\2026\2026-05\` contains the PDF `2026-05-22_Boomstraat GmbH_EUR_17.850,00.pdf`.
- `…\2026\2026-05\invoices.csv` contains exactly **one** row for it.
- `…\2026\2026-04\invoices.csv` no longer contains a row for it.
Repeating the month change must not create duplicate rows.

---

## Self-Review

**Spec coverage:**
- „Erst Datei, dann CSV" → Task 1 Step 1 (the move block now precedes the CSV blocks).
- Robustes Verschieben statt blankem `os.Rename` → `a.storageManager.MoveAndRename` (rename + copy fallback).
- Upsert / keine Doppeleinträge → both CSV branches drop same-filename rows before appending.
- Quelle weg + Ziel vorhanden → behandelt im `else if`/`else`-Zweig der Move-Logik.
- Gilt für Monatswechsel UND gleichen Monat → `sameMonth`-Zweig nutzt ebenfalls den Upsert; der Move-Block läuft für beide Fälle (gesteuert über `intendedPath != originalPath`).
- Selbstheilung → Task 2 Step 5 (manuelle Prüfung).
- Edge „Verschieben scheitert" → `MoveAndRename`-Fehler → früher `return`, CSVs unangetastet.

**Placeholder scan:** Keine TBD/TODO; der zu ersetzende Block und der neue Block sind vollständig und verbatim angegeben.

**Type consistency:** `a.storageManager.MoveAndRename(string, string, string) (string, error)` ist die bestehende Signatur (genutzt auch in `saveInvoice`/`placeFile`); `moved` ist der zurückgegebene finale Name und wird `finalName` zugewiesen. `core.CSVRow` für die `make([]core.CSVRow, …)`-Slices ist im Paket `ui` bereits importiert. `a.rewriteCSV(string, []core.CSVRow) error` und `a.csvRepo.Load(string) ([]core.CSVRow, error)` sind bestehende Signaturen. `newRow`, `newMeta`, `originalRow`, `originalPath`, `newFilename`, `targetYear`, `targetMonth`, `bankAccount`, `ausgangsrechnung` sind alle bereits Parameter/Variablen in `updateInvoice` vor diesem Block.
