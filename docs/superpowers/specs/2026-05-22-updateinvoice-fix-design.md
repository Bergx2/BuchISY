# Fix `updateInvoice` — Monatswechsel beim Bearbeiten — Design

**Datum:** 2026-05-22
**Status:** Genehmigt (Design)

## Überblick

Beim Bearbeiten einer Rechnung mit Monatswechsel schlägt das Speichern
fehl und hinterlässt inkonsistente Daten: `updateInvoice` aktualisiert
heute **zuerst** die CSV-Dateien und verschiebt **danach** die PDF mit
einem ungesicherten `os.Rename`. Schlägt das Verschieben fehl, sind die
CSV-Änderungen schon passiert; jeder Wiederholversuch hängt die Rechnung
**erneut** an die Ziel-CSV an (Doppeleinträge).

## Komponenten & Datenfluss

### 1. Sichere Reihenfolge: erst Datei, dann CSV

`updateInvoice` verschiebt künftig die Beleg-Datei **zuerst** und
aktualisiert **danach** die CSV-Dateien. Schlägt das Verschieben fehl,
kehrt die Funktion mit einem Fehler zurück, **bevor** irgendeine CSV
angefasst wurde — die Rechnung bleibt unverändert, ein Wiederholversuch
ist sauber.

### 2. Robustes Verschieben statt blankem `os.Rename`

Das Verschieben nutzt einen robusten Mechanismus mit Kopier-Fallback (wie
`MoveAndRename`/`copyFile` im Rest von BuchISY) statt eines blanken
`os.Rename`. Gilt für den Monatswechsel **und** das reine Umbenennen im
selben Monat (beide Zweige von `updateInvoice`).

### 3. Idempotenz — keine Doppeleinträge

- Beim Schreiben in die Ziel-CSV wird ein evtl. bereits vorhandener
  Eintrag mit **demselben Dateinamen** zuerst entfernt und dann neu
  geschrieben (Upsert). Ein wiederholter Speichervorgang erzeugt damit
  keinen Doppeleintrag.
- Ist die Quelldatei nicht mehr am Ursprungsort, aber bereits am Zielort
  (Folge eines vorherigen Teilversuchs), gilt das Verschieben als
  erledigt — kein Fehler.

### 4. Selbstheilung des bestehenden Datensalats

Mit dem reparierten Code heilt sich der aktuell inkonsistente Zustand
(PDF in `2026-04`, Eintrag in `2026-04` 1×, in `2026-05` 2×): Der Nutzer
führt den April→Mai-Wechsel einmal erneut aus → die PDF wandert nach
`2026-05`, der Upsert reduziert `2026-05` auf einen Eintrag, der Eintrag
in `2026-04` wird entfernt. Kein manuelles Datei-Hantieren nötig; das
Ergebnis wird nach der Umsetzung geprüft.

## Edge Cases

- Verschieben scheitert (Datei gesperrt, in anderem Programm offen) →
  klare Fehlermeldung, CSVs unverändert, retry-fähig.
- Quelldatei weg, Zieldatei schon vorhanden → als erledigt behandelt.
- Namenskollision am Zielort → `_2`/`_3`-Suffix wie bisher.

## Betroffene Dateien

- `internal/ui/tableedit.go` — `updateInvoice` (Reihenfolge, robustes
  Verschieben, Upsert).

## Tests

- `go build ./...`, `go vet ./...` fehlerfrei.
- Manuell: Rechnung von einem Monat in einen anderen verschieben →
  Datei + Eintrag landen im Zielmonat, Quellmonat sauber. Denselben
  Wechsel zweimal auslösen → kein Doppeleintrag. Bearbeiten mit
  gesperrter Datei → Fehlermeldung, Daten unverändert.
