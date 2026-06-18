# Multi-File-Auswahl & Anhänge — Design

**Datum:** 2026-05-21
**Status:** Genehmigt (Design)

## Überblick

Der PDF-Picker wählt aktuell genau eine Datei und legt eine Rechnung ab.
Dieses Feature erlaubt die Auswahl mehrerer Dateien in einem Vorgang, das
Markieren einer Datei als Hauptdatei (die eigentliche Rechnung) und das
Ablegen der übrigen Dateien als Anhänge neben dieser Rechnung. Zusätzlich
werden über PDF hinaus weitere Dateitypen akzeptiert.

## Ziele

- Mehrfachauswahl im Picker, persistent über Ordnerwechsel hinweg.
- Genau eine Datei als Hauptdatei markierbar.
- Anhänge werden mit derselben Rechnung im selben Monatsordner abgelegt.
- Akzeptierte Typen: PDF, MS Office, LibreOffice, Bilddateien.
- Ist die Hauptdatei kein PDF, wird die Extraktion übersprungen und der
  Bestätigungsdialog mit leeren Feldern zum manuellen Ausfüllen geöffnet.

## Nicht-Ziele (YAGNI)

- Keine typbezogenen Anhang-Labels (Lieferschein etc.) — generische
  Nummerierung `_Anhang1`, `_Anhang2`, …
- Keine OCR/Extraktion aus Nicht-PDF-Dateien.
- Kein nachträgliches Bearbeiten/Entfernen von Anhängen bereits abgelegter
  Rechnungen.
- Keine Unterordner pro Rechnung.

## Unterstützte Dateitypen

- PDF: `.pdf`
- MS Office: `.doc .docx .xls .xlsx .ppt .pptx`
- LibreOffice: `.odt .ods .odp`
- Bilder: `.jpg .jpeg .png .gif .bmp .tif .tiff .webp .heic .svg`

Helper `isSupportedFile(name string) bool` prüft die Endung
(case-insensitive). Ordner bleiben unabhängig vom Filter navigierbar.

## Komponenten & Datenfluss

### 1. Picker (`internal/ui/custompicker.go`)

- Dateiliste: jede Datei-Zeile erhält eine Checkbox. Ordner haben keine
  Checkbox; Klick navigiert wie bisher.
- Persistente `selected []string` (absolute Pfade). Beim (Neu-)Rendern
  einer Zeile spiegelt die Checkbox wider, ob der Pfad in `selected` ist.
- Auswahl-Ablage unterhalb der Liste: je gewählter Datei eine Zeile mit
  Radio-Markierung (Hauptdatei), Basisname und Entfernen-Button. Die zuerst
  gewählte Datei ist standardmäßig Hauptdatei.
- Der Dateifilter zeigt nur unterstützte Typen + Ordner.
- „Öffnen" validiert: ≥1 Datei gewählt und genau eine Hauptdatei markiert;
  sonst Fehlermeldung. Bei Erfolg: Dialog schließen, `processSubmission`
  aufrufen.

### 2. Verarbeitung (`internal/ui/filepicker.go`)

- Neuer Einstieg `processSubmission(mainPath string, attachments []string)`.
  `attachments` = gewählte Dateien ohne die Hauptdatei.
- Ist die Hauptdatei ein PDF: bestehende Extraktion (`extractPDFData`),
  Fortschrittsdialog wie bisher; bei Erfolg `showConfirmationModal` mit
  Anhängen; bei „kein Text" `handleNoTextPDF` (Anhänge bleiben erhalten).
- Ist die Hauptdatei kein PDF: kein Fortschrittsdialog, `meta` = leeres
  `core.Meta{}`, direkt `showConfirmationModal`.

### 3. Bestätigungsdialog (`internal/ui/invoicemodal.go`)

- `showConfirmationModal` erhält zusätzlich `attachments []string`.
- Zeigt read-only „Anhänge: N" samt Liste der Original-Basisnamen.
- Speicherlogik (`completeSave`):
  1. Echte Endung der Hauptdatei bestimmen.
  2. Dateinamen aus dem Namensschema erzeugen; die literale Endung des
     Schemas durch die echte Endung der Hauptdatei ersetzen.
  3. Hauptdatei via `MoveAndRename` ablegen → finaler Dateiname.
  4. `finalBase` = finaler Dateiname ohne Endung.
  5. Jeden Anhang N (1-basiert, N in der Reihenfolge der Auswahl-Ablage)
     als `<finalBase>_Anhang<N><Anhangendung>` in denselben Monatsordner
     verschieben (`MoveAndRename`, Kollisionen über bestehende `_2`-Logik).
  6. Schlägt ein Anhang fehl: Warnung anzeigen, fortfahren — die
     Hauptrechnung bleibt abgelegt.
  7. CSV-Zeile: `HatAnhaenge = len(attachments) > 0`,
     `AnzahlAnhaenge = len(attachments)`.

### 4. Dateinamen-Endung

Das Namensschema (`NamingTemplate`) endet literal auf `.pdf`. Ein Helper
ersetzt diese Endung durch die echte Endung der Hauptdatei. Für PDF-Haupt-
dateien ist das ein No-op.

### 5. CSV (`internal/core/types.go`, `csvrepo.go`, i18n)

- `CSVRow` erhält `HatAnhaenge bool` und `AnzahlAnhaenge int`.
- `csvrepo.go`: Lesen/Schreiben der neuen Spalten. Beim Lesen bestehender
  CSVs ohne diese Spalten → Default `false` / `0` (Spalten optional).
- Spaltendefinitionen: `DefaultCSVColumns`, `ColumnDisplayNames`,
  `ColumnTranslationKeys` um beide Spalten erweitern.
- i18n `de.json` / `en.json`: Header-Übersetzungen ergänzen.
- `ToCSVRow` setzt die Felder nicht (kein Meta-Feld); die Speicherlogik
  setzt sie.

### 6. Sortierung im Ordner

Die Hauptdatei (`name.pdf`) sortiert automatisch vor ihren Anhängen
(`name_Anhang1…`), da `.` (0x2E) < `_` (0x5F). „Hauptdatei zuoberst"
ergibt sich von selbst.

## Edge Cases

- 0 Dateien gewählt → Fehlermeldung.
- 0 oder >1 Hauptdatei markiert → Fehlermeldung.
- Anhang-Verschiebefehler → Hauptrechnung + CSV-Zeile bleiben; Warnung
  nennt die fehlgeschlagenen Anhänge.
- Duplikatprüfung unverändert (Rechnungsnummer/Datum/Betrag).
- Nicht-PDF-Hauptdatei ohne manuelle Daten → bestehende Modal-Validierung
  greift.
- Namenskollisionen → `_2`-Suffix für Haupt- und Anhangdateien.
- Wird dieselbe Datei als Haupt- und Anhang geführt: `attachments` =
  `selected` ohne die Hauptdatei.

## Betroffene Dateien

- `internal/ui/custompicker.go` — Mehrfachauswahl, Auswahl-Ablage, Filter
- `internal/ui/filepicker.go` — `processSubmission`
- `internal/ui/invoicemodal.go` — Modal-Signatur, Speicherlogik
- `internal/core/types.go` — `CSVRow`-Felder, Spaltendefinitionen
- `internal/core/csvrepo.go` — Lesen/Schreiben neuer Spalten
- `assets/i18n/de.json`, `assets/i18n/en.json` — Spaltenüberschriften

## Tests

- `go build ./...` und `go vet ./...` fehlerfrei.
- Unit-Tests: `isSupportedFile`, Endungs-Ersetzungs-Helper,
  Anhang-Namensschema, `csvrepo`-Round-Trip mit neuen Spalten.
- Manuell: nur 1 PDF (Regression); PDF + 2 Anhänge; Nicht-PDF-Hauptdatei +
  Anhänge — jeweils Monatsordner und `invoices.csv` prüfen.
