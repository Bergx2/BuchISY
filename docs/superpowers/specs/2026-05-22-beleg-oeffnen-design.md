# Beleg öffnen — Design

**Datum:** 2026-05-22
**Status:** Genehmigt (Design)

## Überblick

Es soll möglich sein, die Beleg-Datei einer Rechnung (PDF, JPG, …) direkt
zu öffnen — aus der Haupttabelle und aus dem Fenster „Rechnung
bearbeiten". Geöffnet wird im Standardprogramm des Betriebssystems.

## Komponenten & Datenfluss

### 1. Open-Helfer (`internal/ui/openfolder.go`)

- Neue Funktion `(a *App) openFileInOS(path string)` öffnet eine Datei im
  Standardprogramm. Sie nutzt denselben Mechanismus wie der bestehende
  „Original öffnen"-Knopf im Prüf-Dialog: eine Datei-URI über
  `storage.NewFileURI(path)`, `url.Parse`, dann `a.app.OpenURL(parsed)`.
  Schlägt das Parsen oder Öffnen fehl, wird `a.showError` mit der
  bestehenden i18n-Meldung `error.openOriginal` angezeigt.
- Der bestehende „Original öffnen"-Knopf in `showConfirmationModal`
  (`invoicemodal.go`) wird auf `a.openFileInOS(originalPath)` umgestellt —
  reine Aufräumung, kein Verhaltensunterschied.

### 2. Haupttabelle — Symbol-Spalte „👁" (`internal/ui/table.go`)

- Die Tabelle hat heute zwei Aktionsspalten — ✏️ (Bearbeiten) und 🗑
  (Löschen). Es kommt eine dritte Aktionsspalte mit dem Symbol **👁**
  hinzu, nach demselben Muster (Spaltenbreite, Header-Symbol, Zell-Button).
- Klick auf 👁 einer Zeile öffnet deren Beleg: der Pfad wird über
  `core.InvoiceFilePath(monthFolder, row)` aufgelöst (inkl.
  `Unterordner`), wobei `monthFolder = storageManager.GetMonthFolder(
  currentYear, currentMonth)`; dann `a.openFileInOS(pfad)`.

### 3. „Rechnung bearbeiten" — Button „Beleg öffnen" (`internal/ui/tableedit.go`)

- In `showEditDialog` kommt ein Button **„Beleg öffnen"** in die obere
  Zeile, direkt neben dem Label „Datei: …". Klick ruft
  `a.openFileInOS(originalPath)` auf (`originalPath` ist bereits über
  `core.InvoiceFilePath` aufgelöst).
- Die eingebettete Belegvorschau rechts bleibt unverändert; der Button
  dient dem Öffnen im externen Programm.

## Edge Cases

- Datei nicht vorhanden (extern verschoben/gelöscht) → `OpenURL` schlägt
  fehl → Fehlermeldung über `a.showError`.
- Nicht unterstütztes Dateiformat ohne zugeordnetes Standardprogramm →
  das Betriebssystem entscheidet; schlägt `OpenURL` fehl, Fehlermeldung.

## Betroffene Dateien

- `internal/ui/openfolder.go` — neue Funktion `openFileInOS`.
- `internal/ui/invoicemodal.go` — „Original öffnen" nutzt den neuen Helfer.
- `internal/ui/table.go` — dritte Aktionsspalte 👁 + Klick-Handler.
- `internal/ui/tableedit.go` — Button „Beleg öffnen" im Bearbeiten-Fenster.

## Tests

- `go build ./...`, `go vet ./...` fehlerfrei; bestehende `go test ./...`
  bleiben grün (reine UI-Änderungen, keine neuen Unit-Tests — die UI ist
  nicht headless testbar).
- Manuell:
  - In der Haupttabelle 👁 einer Zeile klicken → der Beleg öffnet sich im
    Standardprogramm; funktioniert auch für Belege in `Bar/` /
    `Ausgangsrechnungen/`.
  - In „Rechnung bearbeiten" „Beleg öffnen" klicken → die Datei öffnet
    sich.
  - Eine Zeile, deren Datei fehlt → Fehlermeldung statt Absturz.
