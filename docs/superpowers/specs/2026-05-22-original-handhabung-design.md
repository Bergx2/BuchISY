# Original-Handhabung beim Speichern — Design

**Datum:** 2026-05-22
**Status:** Genehmigt (Design)

## Überblick

Beim Ablegen einer Rechnung verschiebt BuchISY heute die Quelldatei
(kopieren + Original löschen). Das ist falsch für normale Datei-Uploads
— das Original des Nutzers soll erhalten bleiben — und es führt zu einem
Fehlschlag, wenn das Original in einem anderen Programm (z. B. Adobe
Acrobat) gesperrt ist.

Neue Regel: Die Herkunft der Datei bestimmt, was mit dem Original
geschieht. Ein Upload per Dateiauswahl wird **kopiert** (Original
bleibt); eine Datei aus dem Scan-Eingangsordner wird **verschoben**
(Original wird gelöscht, Eingang leeren).

## Komponenten & Datenfluss

### 1. Kopier-Variante in `internal/core/storage.go`

- `MoveAndRename(sourcePath, targetFolder, newName)` bleibt unverändert
  (kopieren + Original löschen) — weiterhin genutzt für das Verschieben
  bereits abgelegter Rechnungen beim Bearbeiten.
- Neue Funktion `CopyAndRename(sourcePath, targetFolder, newName) (string, error)`:
  identische Zielordner-Erstellung und Kollisionsbehandlung
  (`_2`, `_3`, …) wie `MoveAndRename`, aber sie **kopiert nur** — das
  Original bleibt liegen. Rückgabe ist der finale Dateiname.
- Die gemeinsame Logik (Zielordner anlegen, kollisionsfreien Zielnamen
  bestimmen) wird in einen internen Helfer ausgelagert, den beide
  Funktionen nutzen — keine duplizierte Kollisionsschleife.

### 2. Quellprüfung in `saveInvoice` (`internal/ui/invoicemodal.go`)

- `saveInvoice` bestimmt für die Hauptdatei und jeden Anhang, ob die
  Quelle aus dem Scan-Eingangsordner stammt:
  - Ist `settings.ScanInboxFolder` gesetzt **und** liegt der Quellpfad
    innerhalb dieses Ordners → **Scan-Fall** → `MoveAndRename`
    (Original wird gelöscht).
  - Sonst → **Upload-Fall** → `CopyAndRename` (Original bleibt).
- Die Zuordnung „liegt innerhalb von" erfolgt über einen Pfadvergleich
  (beide Pfade zu absoluten, bereinigten Pfaden normalisieren; prüfen, ob
  der Quellpfad ein Nachfahre des Scan-Ordners ist).
- Anhänge kommen ausschließlich per Upload und werden daher praktisch
  immer kopiert; die Prüfung wird der Einheitlichkeit halber trotzdem auf
  jede Datei angewandt.

### 3. Speichern schlägt bei gesperrtem Original nicht mehr fehl

- Upload-Fall: Es wird nichts gelöscht → der „Datei in Verwendung"-Fehler
  kann dort nicht mehr auftreten.
- Scan-Fall: Schlägt das Löschen des Scan-Originals fehl (z. B. Datei
  gesperrt), während das Kopieren erfolgreich war, gilt der Vorgang
  **trotzdem als erfolgreich**. `MoveAndRename` gibt in diesem Fall den
  finalen Dateinamen ohne Fehler zurück; das nicht gelöschte Original
  wird über den (bestehenden) Warn-Mechanismus protokolliert. Der
  Rechnungs-Speichervorgang läuft normal weiter (Datei abgelegt, CSV
  geschrieben, kein Fehlerdialog).
- Schlägt dagegen das **Kopieren** fehl, bleibt es ein echter Fehler —
  dann ist die Datei nicht abgelegt.

## Edge Cases

- `ScanInboxFolder` nicht gesetzt → jede Quelle gilt als Upload → Original
  bleibt immer erhalten.
- Quelldatei liegt zufällig im Scan-Ordner, obwohl sie per Dateiauswahl
  gewählt wurde → wird als Scan-Fall behandelt (Original gelöscht). Das
  ist akzeptiert: Der Scan-Ordner ist ein dedizierter Eingang.
- Kollision am Zielort → `_2`/`_3`-Suffix wie bisher, in beiden Varianten.
- Kopieren scheitert (Quelle nicht lesbar, Ziel nicht beschreibbar) →
  echter Fehler, Rechnung wird nicht abgelegt.

## Betroffene / neue Dateien

- `internal/core/storage.go` — neuer Helfer für Zielname/-ordner;
  `CopyAndRename`; `MoveAndRename` toleriert ein fehlgeschlagenes
  Löschen des Originals (Erfolg statt Fehler).
- `internal/ui/invoicemodal.go` — `saveInvoice` wählt je Quelle zwischen
  `CopyAndRename` und `MoveAndRename`.
- `internal/core/storage_test.go` — Tests (siehe unten).

## Tests

- `go build ./...`, `go vet ./...` fehlerfrei.
- Unit-Test `CopyAndRename`: Datei wird ins Ziel kopiert, Quelldatei
  existiert danach **weiterhin**; Rückgabe ist der finale Name;
  Kollision → `_2`-Suffix.
- Unit-Test `MoveAndRename`: Datei wird ins Ziel verschoben, Quelldatei
  ist danach weg (Normalfall unverändert).
- Manuell:
  - Datei-Upload einer PDF → Rechnung abgelegt, hochgeladene
    Original-Datei liegt unverändert am Ursprungsort.
  - Upload einer PDF, die in Adobe Acrobat geöffnet ist → Speichern
    funktioniert ohne Fehler.
  - PDF in den Scan-Ordner legen → nach Verarbeitung ist sie aus dem
    Scan-Ordner verschwunden und im Ablageordner abgelegt.
