# Rechnung-bearbeiten überarbeiten + Dateiname-Verbesserungen — Design

**Datum:** 2026-05-21
**Status:** Genehmigt (Design)

## Überblick

Ein zusammenhängendes Bündel von Verbesserungen rund um die
Rechnungs-Dialoge und Dateinamen:
1. **Bugfix:** „Rechnung bearbeiten" zeigt Beträge mit Punkt statt mit dem
   eingestellten Dezimaltrennzeichen.
2. **Feature:** In „Rechnungsdaten prüfen" und „Rechnung bearbeiten" wird
   die Dateiname-Vorschau zu einem editierbaren Feld.
3. **Feature:** In „Rechnung bearbeiten" lässt sich der **Ablagemonat**
   (Jahr + Monat) ändern; die Rechnung wird in den gewählten Monatsordner
   verschoben.
4. **Bugfix:** Beträge im Dateinamen verlieren ihr Komma — `15,23` wird zu
   `1523`. Das Komma soll erhalten bleiben.
5. **Feature:** „Rechnung bearbeiten" wird ein größenänderbares Fenster mit
   Belegvorschau rechts — wie „Rechnungsdaten prüfen".

## Ziele

- „Rechnung bearbeiten" zeigt Beträge im konfigurierten Dezimalformat.
- Der finale Dateiname ist in beiden Dialogen direkt editierbar.
- Eine Rechnung kann in „Rechnung bearbeiten" einem anderen Monat
  zugeordnet werden.
- Ein Betrag wie `15,23` erscheint im Dateinamen als `15,23`.
- „Rechnung bearbeiten" ist in der Größe änderbar und zeigt rechts die
  Belegvorschau.

## Nicht-Ziele (YAGNI)

- Keine Änderung der Namensvorlage oder der Template-Logik.
- Anhang-Dateien werden beim Verschieben nicht mitverschoben (entspricht
  dem heutigen Verhalten beim Umbenennen) — siehe Edge Cases.

## Komponenten & Datenfluss

### 1. Dezimal-Fix (`internal/ui/tableedit.go`)

- In `showEditDialog` werden `netEntry`, `vatPercentEntry`,
  `vatAmountEntry`, `grossEntry` aktuell mit `fmt.Sprintf("%.2f", …)`
  vorbefüllt — immer mit Punkt.
- Stattdessen wird der bestehende paketweite Helfer
  `formatDecimal(v float64, sep string) string` (in `kassenbuchview.go`)
  verwendet. Reine Anzeige-Korrektur; das Einlesen (`parseFloat`) ist
  bereits komma-tolerant.

### 2. Editierbares Dateiname-Feld

Betrifft `showConfirmationModal` (`invoicemodal.go`) und `showEditDialog`
(`tableedit.go`) — beide haben heute denselben Aufbau:

- `filenamePreview` (`newCopyableLabel`, nur Anzeige) wird ein
  `widget.NewEntry()` (editierbar).
- Auto-Aktualisierung mit Vorrang der Nutzer-Eingabe:
  - Flag `filenameEdited bool` (Start `false`).
  - `updateFilenamePreview` schreibt den Vorlagen-Namen nur ins Feld, wenn
    `filenameEdited == false`.
  - Beim programmatischen Setzen wird ein Guard `suppressFilenameChange`
    gesetzt, damit der `OnChanged`-Handler dies nicht als Nutzer-Eingabe
    wertet.
  - Der `OnChanged`-Handler setzt `filenameEdited = true`, wenn der Guard
    nicht aktiv ist.
- Ergebnis: Das Feld folgt automatisch den Feldänderungen, bis der Nutzer
  es manuell ändert; danach bleibt seine Eingabe stehen.

### 3. Verwendung beim Speichern

- **`saveInvoice`** (`invoicemodal.go`) und **`updateInvoice`**
  (`tableedit.go`) erhalten den Dateinamen als Parameter, statt ihn per
  `core.ApplyTemplate` zu erzeugen.
- Beide verarbeiten den übergebenen Namen so:
  - `core.SanitizeFilename` darauf anwenden (Nutzer-Eingabe bereinigen;
    das Komma bleibt nach Fix 4 erhalten).
  - `core.ReplaceExtension`, um die Endung an die echte Datei anzupassen.
  - Der bestehende Kollisions-Zähler (`_2`, `_3`, …) bleibt.
- **Leerer Dateiname** (nach `TrimSpace`) → `saveInvoice`/`updateInvoice`
  geben einen Fehler zurück; der Dialog bleibt offen.

### 4. Komma im Dateinamen erhalten (`internal/core/sanitize.go`)

- `SanitizeFilename` entfernt aktuell das Komma (es steht in der
  Unsafe-Zeichen-Regex). Ein Betrag `15,23` wird dadurch zu `1523`.
- Das Komma wird aus der Unsafe-Zeichen-Regex entfernt — es ist auf
  Windows und macOS ein zulässiges Dateinamen-Zeichen. Die übrigen
  Unsafe-Zeichen (`< > : " | ? *`, Steuerzeichen, Schrägstriche) bleiben.
- Wirkung: alle Betrags-Token behalten ihr Dezimaltrennzeichen im
  Dateinamen. Die CSV-Speicherung bleibt korrekt (der CSV-Writer quotet
  Felder mit Komma automatisch).

### 5. Ablagemonat ändern (`internal/ui/tableedit.go`)

- In `showEditDialog` kommt **eine zusätzliche Formularzeile** „Ablage
  (Jahr/Monat)": zwei Auswahlfelder Jahr und Monat nebeneinander (über
  `generateYearOptions()` / `generateMonthOptions(a.bundle)`).
- Vorbelegt mit `a.currentYear` / `a.currentMonth` (dem Ordner, in dem die
  Rechnung gerade liegt).
- `updateInvoice` erhält zusätzlich `targetYear int` und
  `targetMonth time.Month`. Quelle bleibt `a.currentYear`/`a.currentMonth`.
- Verschiebe-Logik in `updateInvoice`:
  - **Zielordner** = `GetMonthFolder(targetYear, targetMonth)`; existiert
    er nicht, wird er angelegt (`os.MkdirAll`).
  - Die Hauptdatei wird in den Zielordner verschoben; der Kollisions-Zähler
    prüft gegen den Zielordner.
  - **CSV:** Ziel == Quelle → Zeile wie bisher in der einen CSV
    aktualisieren. Ziel != Quelle → Zeile aus der Quell-CSV entfernen
    (neu schreiben) und in die Ziel-CSV einfügen (neu schreiben).
  - `meta.Jahr` / `meta.Monat` (CSV-Spalten) bleiben aus dem
    Rechnungsdatum abgeleitet.
- Nach dem Speichern lädt `loadInvoices()` den aktuellen Monat neu; eine
  verschobene Rechnung verschwindet damit aus der aktuellen Ansicht.

### 6. „Rechnung bearbeiten" als größenänderbares Fenster mit Vorschau

- `showEditDialog` nutzt heute `dialog.NewCustomConfirm` — ein
  Fyne-Dialog, der nicht in der Größe änderbar ist.
- Stattdessen wird — wie `showConfirmationModal` — ein eigenes Fenster
  verwendet: `a.app.NewWindow("Rechnung bearbeiten")`, `Resize`,
  `CenterOnScreen`.
- Inhalt: ein `container.NewHSplit(scrollForm, preview)`, wobei
  `preview := buildDocumentPreview(originalPath, meta)` (dieselbe
  Vorschau-Komponente wie im Prüf-Dialog; zeigt die gerenderte
  Beleg-Datei, sofern vorhanden). Die Teiler-Position wird aus
  `a.settings.PreviewSplitOffset` gesetzt und beim Schließen dorthin
  zurückgespeichert (`SetOnClosed`) — gemeinsame Einstellung mit dem
  Prüf-Dialog.
- Eine Schaltflächenleiste unten mit „Speichern" / „Abbrechen":
  - „Speichern" ruft `updateInvoice(...)` auf; bei Erfolg `loadInvoices()`
    und Fenster schließen, bei Fehler bleibt das Fenster offen.
  - „Abbrechen" schließt das Fenster.
- Die Kalender-Buttons (`📅`) öffnen den Datums-Picker künftig über dem
  neuen Bearbeiten-Fenster statt über `a.window` (Fenster vorab als
  `var editWin fyne.Window` deklariert).

## Edge Cases

- Vorlagen-Fehler (`ApplyTemplate`) → „Fehler: …" im Feld; der Nutzer kann
  den Namen überschreiben.
- Name ohne/mismatched Endung → `ReplaceExtension` korrigiert die Endung.
- Dateiname-Feld leer → Fehler beim Speichern.
- „Rechnung bearbeiten" mit unverändertem Dateinamen *und* unverändertem
  Ablagemonat → kein Verschieben/Umbenennen.
- **Anhänge:** `updateInvoice` verschiebt/benennt nur die Hauptdatei —
  Anhang-Dateien bleiben unberührt (bewusste, bekannte Einschränkung,
  entspricht dem heutigen Umbenennen-Verhalten).
- Beleg-Datei nicht darstellbar (kein PDF/Bild) → `buildDocumentPreview`
  zeigt seinen vorhandenen Platzhalter; kein Sonderfall nötig.

## Betroffene Dateien

- `internal/core/sanitize.go` — Komma aus der Unsafe-Zeichen-Regex.
- `internal/ui/invoicemodal.go` — `filenamePreview` → Eingabefeld +
  `filenameEdited`-Logik; `saveInvoice` erhält den Dateinamen-Parameter.
- `internal/ui/tableedit.go` — Dezimal-Fix; `filenamePreview` →
  Eingabefeld + `filenameEdited`-Logik; Jahr/Monat-Auswahlzeile;
  `updateInvoice` erhält Dateinamen-/`targetYear`-/`targetMonth`-Parameter
  + Verschiebe-Logik; Umstellung von `dialog.NewCustomConfirm` auf ein
  eigenes Fenster mit `buildDocumentPreview`.

## Tests

- `go build ./...`, `go vet ./...` fehlerfrei.
- Unit-Test `SanitizeFilename`: ein Eingabewert mit Komma behält das
  Komma; weiterhin entfernte Zeichen (`<>:"|?*`) werden weiter entfernt.
- Bestehende `go test ./...` bleiben grün (UI-Änderungen sind nicht
  headless testbar — keine neuen UI-Tests).
- Manuell:
  - „Rechnung bearbeiten" zeigt Beträge mit Komma; das Fenster ist in der
    Größe änderbar; rechts erscheint die Belegvorschau.
  - Dateiname-Feld editierbar in beiden Dialogen; folgt den Feldänderungen
    bis zur manuellen Änderung, danach bleibt es stehen.
  - Speichern verwendet den angezeigten Namen (mit korrekter Endung); ein
    `${GrossAmount}` wie `15,23` bleibt im Dateinamen erhalten.
  - Leeres Feld → Fehlermeldung, Dialog bleibt offen.
  - Ablage-Monat in „Rechnung bearbeiten" ändern → Datei und CSV-Eintrag
    liegen danach im Zielmonat, nicht mehr im Quellmonat.
