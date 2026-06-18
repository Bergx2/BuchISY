# Kalender im „Datum wählen"-Fenster — Design

**Datum:** 2026-05-22
**Status:** Genehmigt (Design)

## Überblick

Das Fenster „Datum wählen" (aufgerufen über die 📅-Knöpfe neben
Datumsfeldern) hat heute drei Auswahllisten (Tag / Monat / Jahr). Künftig
zeigt es einen Monatskalender für schnellere Auswahl.

## Komponenten & Datenfluss

### 1. Monatskalender

Statt der drei Dropdowns: oben Monat + Jahr mit Blätter-Pfeilen **‹ ›**,
darunter ein 7-Spalten-Raster (Mo–So) mit den Tagen des Monats als
anklickbare Schaltflächen.

### 2. Auswahl per Klick

Ein Klick auf einen Tag wählt das Datum direkt aus und schließt das
Fenster — kein „OK" nötig. „Abbrechen" schließt ohne Auswahl. Die Pfeile
blättern Monate, ohne etwas auszuwählen (auch über Jahresgrenzen, z. B.
Dez → Jan des Folgejahres).

### 3. Hervorhebungen

Der **heutige Tag** ist dezent markiert. Wird das Fenster zum Ändern
eines bestehenden Datums geöffnet, startet der Kalender im Monat dieses
Datums und der betreffende Tag ist deutlich markiert.

### 4. Umsetzung

Der Kalender wird mit Fyne-Bordmitteln gebaut (Raster aus Tag-Buttons) —
**keine zusätzliche Programmbibliothek**. Da `invoicemodal.go` bereits
groß ist, kommt der Kalender-Code in eine neue Datei
`internal/ui/datepicker.go`; `showDatePicker` wird dorthin verlagert und
neu umgesetzt.

### 5. Rückgabe unverändert

Die Auswahl liefert wie bisher ein Datum im Format `TT.MM.JJJJ` an das
aufrufende Feld. Die Signatur von `showDatePicker` bleibt gleich, sodass
die 📅-Knöpfe unverändert aufrufen.

## Edge Cases

- Leeres/ungültiges Ausgangsdatum → Kalender startet im heutigen Monat.
- Monate mit 28–31 Tagen → das Raster zeigt genau die gültigen Tage.

## Betroffene / neue Dateien

- `internal/ui/invoicemodal.go` — `showDatePicker` wird entfernt.
- `internal/ui/datepicker.go` — neu: der Monatskalender.

## Tests

- `go build ./...`, `go vet ./...` fehlerfrei.
- Manuell: 📅-Knopf öffnet den Kalender; heutiger Tag dezent markiert;
  Klick auf einen Tag setzt das Datum und schließt; Blättern funktioniert
  über Jahresgrenzen; ein bestehendes Datum öffnet im richtigen Monat mit
  markiertem Tag.
