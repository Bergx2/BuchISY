# Scan-Eingang-Ordner — Design

**Datum:** 2026-05-21
**Status:** Genehmigt (Design)

## Überblick

BuchISY überwacht einen vom Nutzer festgelegten Ordner. Legt ein externes
Scan-Programm (z. B. NAPS2) dort eine PDF ab, übernimmt BuchISY sie
automatisch in den bestehenden Verarbeitungsweg — Erkennung der
Rechnungsdaten und Bestätigungsdialog, genau wie bei „PDF auswählen".

## Ziele

- Ein konfigurierbarer „Scan-Eingang-Ordner" in den Einstellungen.
- Neu eintreffende PDFs werden automatisch erkannt und verarbeitet.
- Immer nur ein Scan gleichzeitig — keine überlappenden Dialoge.

## Nicht-Ziele (YAGNI)

- Keine Überwachung von Unterordnern.
- Keine OCR in BuchISY (übernimmt das Scan-Programm).
- Keine Änderung am Bestätigungsdialog selbst.
- Keine Persistenz, welche Dateien bereits gesehen wurden (nur im
  Speicher, pro Sitzung).

## Komponenten & Datenfluss

### 1. Einstellung (`internal/core/types.go`, `internal/ui/settings.go`)

- `Settings` bekommt das Feld `ScanInboxFolder string`
  (`json:"scan_inbox_folder"`). Leer = Funktion aus.
- Im Einstellungen-Tab „Ablage" ein neues Formularfeld
  „Scan-Eingang-Ordner" mit Eingabefeld + „Durchsuchen"-Button — analog
  zum bestehenden Zielordner-Feld (`browseFolderBtn`).
- `DefaultSettings()` setzt `ScanInboxFolder` auf den Leerstring.

### 2. Überwachung (`internal/ui/scanwatcher.go`, neu)

- Eine Hintergrund-Schleife (Goroutine mit `time.Ticker`, Intervall
  5 Sekunden) prüft den Ordner, solange die App läuft.
- Pro Durchlauf werden die `*.pdf`-Dateien des Ordners (nicht rekursiv)
  aufgelistet.
- **Vollständigkeits-Prüfung:** Eine PDF gilt erst als „fertig", wenn ihre
  Dateigröße seit dem vorigen Durchlauf unverändert ist. Beim ersten
  Sichten wird die Größe nur gemerkt; erst beim nächsten Durchlauf mit
  gleicher Größe gilt sie als bereit. So wird ein noch schreibendes
  Scan-Programm nicht zu früh erwischt.
- Zwei In-Memory-Zustände im Watcher:
  - `sizes map[string]int64` — zuletzt gesehene Dateigröße je Pfad
    (Vollständigkeits-Prüfung).
  - `handled map[string]bool` — bereits angestoßene Dateien; werden nicht
    erneut verarbeitet.
- Beim App-Start bereits vorhandene PDFs werden wie neue behandelt (nach
  der Vollständigkeits-Prüfung übernommen).
- Ist `ScanInboxFolder` leer oder existiert nicht, tut der Watcher nichts.

### 3. Verarbeitung

- Eine bereite, noch nicht behandelte PDF wird über den bestehenden
  Einstieg `processSubmission(path, nil)` verarbeitet (Erkennung →
  Bestätigungsdialog). Die Datei wird in `handled` eingetragen, bevor sie
  übergeben wird.
- **Serialisierung — immer nur ein Scan:** Der Watcher hält ein Flag
  „gerade beschäftigt". Es wird gesetzt, sobald ein Scan an
  `processSubmission` übergeben wird, und zurückgesetzt, wenn die
  Verarbeitung dieses Scans abgeschlossen ist (Bestätigungsfenster
  geschlossen oder die Verarbeitung endete mit Fehler). Solange das Flag
  gesetzt ist, übergibt der Watcher keinen weiteren Scan.
  - Dazu erhält `processSubmission` einen optionalen Abschluss-Callback
    (`onComplete func()`), den der Watcher übergibt; der bestehende Aufruf
    aus dem Datei-Picker übergibt `nil`. Der Callback wird auf jedem
    Abschlusspfad ausgelöst: nach Schließen des Bestätigungsfensters und
    auf den Fehlerpfaden von `processSubmission`.
- Beim Speichern verschiebt der bestehende Verarbeitungsweg die PDF in den
  Monatsordner; der Eingang-Ordner leert sich dadurch von selbst. Bricht
  der Nutzer ab, bleibt die Datei liegen, wird aber wegen `handled` in
  dieser Sitzung nicht erneut verarbeitet.

### 4. Threading

- Das Polling läuft in einer Goroutine. Datei-Auflistung und
  Größen-/`handled`-Prüfung erfolgen dort; der Aufruf von
  `processSubmission` (und damit alles UI-bezogene) wird über `fyne.Do`
  auf den UI-Hauptthread gegeben.
- Der Watcher wird beim App-Start gestartet (in `App.Run` bzw. nach
  `buildUI`).

## Edge Cases

- `ScanInboxFolder` leer / nicht vorhanden → Watcher inaktiv, kein Fehler.
- Eine abgebrochene (übersprungene) Datei bleibt im Ordner; sie wird in
  derselben Sitzung nicht erneut verarbeitet. Nach App-Neustart kann sie
  erneut erkannt werden — akzeptiert (die Datei ist ja unbearbeitet).
- Mehrere PDFs gleichzeitig bereit → nacheinander, je eine nach Abschluss
  der vorigen.
- Datei während des Schreibens → Vollständigkeits-Prüfung verzögert die
  Übernahme bis zur stabilen Größe.

## Betroffene / neue Dateien

- `internal/core/types.go` — Feld `ScanInboxFolder` in `Settings`.
- `internal/ui/settings.go` — Eingabefeld + „Durchsuchen" im Tab „Ablage".
- `internal/ui/scanwatcher.go` — neu: Polling-Schleife, Zustände,
  Anstoß der Verarbeitung.
- `internal/ui/filepicker.go` — `processSubmission` erhält den optionalen
  `onComplete`-Callback.
- `internal/ui/app.go` — Watcher beim Start starten.

## Tests

- `go build ./...`, `go vet ./...` fehlerfrei.
- Unit-Test der Vollständigkeits-Logik: eine reine, testbare Funktion
  entscheidet anhand von „vorige Größe" / „aktuelle Größe" / `handled`, ob
  eine Datei jetzt zu übernehmen ist (stabil + unbehandelt → ja; Größe
  geändert → nein; bereits behandelt → nein).
- Manuell: Einstellungen → Scan-Eingang-Ordner setzen; eine PDF in den
  Ordner legen → BuchISY öffnet nach wenigen Sekunden den
  Bestätigungsdialog; nach dem Speichern ist die PDF aus dem Ordner
  verschwunden; zwei PDFs gleichzeitig → nacheinander.
