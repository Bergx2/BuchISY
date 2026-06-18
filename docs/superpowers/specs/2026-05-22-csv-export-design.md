# CSV-Export — Design

**Datum:** 2026-05-22
**Status:** Genehmigt (Design)

## Überblick

Zusätzlich zur internen Speicherung soll BuchISY auf Knopfdruck eine CSV
erzeugen — für einen Monat oder einen frei gewählten Zeitraum — und sie
zum Abspeichern anbieten. Die interne Speicherung (die monatliche
`invoices.csv` als Datenspeicher) bleibt **unverändert**; der Export ist
rein zusätzlich und verändert/löscht keine Daten.

## Komponenten & Datenfluss

### 1. Export-Button

Ein neuer Knopf „CSV-Export" im Hauptfenster, in der bestehenden
Knopfleiste (neben „Kassenbuch" / „Zielordner öffnen").

### 2. Export-Dialog

Klick öffnet einen Dialog mit zwei Modi:
- **Monat** — Auswahl Jahr + Monat.
- **Zeitraum** — von-Datum und bis-Datum (`TT.MM.JJJJ`); kann mehrere
  Monate umfassen.

### 3. Erzeugung

BuchISY sammelt die Rechnungen des gewählten Monats bzw. Zeitraums aus
den vorhandenen monatlichen `invoices.csv`-Dateien (über
`ListAllCSVPaths` / die Monatsordner) und erzeugt **eine** CSV mit
denselben Spalten wie die interne CSV. Beim Zeitraum-Modus werden die
Zeilen aller betroffenen Monate zusammengeführt; gefiltert wird nach dem
Rechnungs-/Ablagemonat passend zum gewählten Bereich.

### 4. Abspeichern

Die erzeugte CSV wird über einen **Speichern-Dialog** angeboten — Ort und
Dateiname frei wählbar (Monatsordner oder beliebiger Ort, z. B. für den
Steuerberater). Einheitlich für beide Modi.

### 5. Interne Speicherung unverändert

Der Export liest nur; er schreibt nichts in die Monatsordner und ändert
die internen `invoices.csv`-Dateien nicht.

## Edge Cases

- Zeitraum/Monat ohne Rechnungen → Hinweis „keine Rechnungen im gewählten
  Bereich" statt einer leeren Datei.
- Ungültige Datumseingabe im Zeitraum-Modus → Hinweis, kein Export.
- Zeitraum über mehrere Jahre/Monate → Zusammenführung aus allen
  betroffenen Monats-CSVs.

## Betroffene / neue Dateien

- `internal/ui/csvexport.go` — neu: Export-Dialog + Auslöser.
- `internal/core/` — Helfer zum Sammeln der Zeilen eines Zeitraums und
  Schreiben der Export-CSV (nutzt `CSVRepository`).
- Hauptfenster-Knopfleiste (`internal/ui/`, wo Kassenbuch/Zielordner-
  Knöpfe gebaut werden) — der neue Knopf.

## Tests

- `go build ./...`, `go vet ./...` fehlerfrei.
- Unit-Test des Zeitraum-Sammelns: bei mehreren Monats-CSVs werden genau
  die Zeilen des gewählten Bereichs zusammengeführt.
- Manuell: „CSV-Export" → Monat wählen → Speichern-Dialog → CSV mit den
  Rechnungen des Monats. Zeitraum über zwei Monate → eine CSV mit den
  Zeilen beider Monate. Leerer Bereich → Hinweis.
