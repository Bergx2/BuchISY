# Rechtsklick „Kopieren" für Anzeige-Texte — Design

**Datum:** 2026-05-21
**Status:** Genehmigt (Design)

## Überblick

Anzeige-Texte in BuchISY (reine `widget.Label`) lassen sich aktuell nicht
kopieren. Fyne erlaubt kein Cursor-Markieren in Labels. Dieses Feature
ergänzt ein Rechtsklick-Kontextmenü „Kopieren" an den wertetragenden
Anzeige-Texten und an den Zellen der Rechnungstabelle.

## Ziele

- Wiederverwendbares Label-Widget mit Rechtsklick → „Kopieren".
- Im „Rechnungsdaten prüfen"-Fenster: Dateiname-Vorschau und Anhänge-Liste
  kopierbar.
- Auf den Einstellungs-Unterseiten: die Anzeige-/Hinweis-Labels kopierbar.
- In der Rechnungstabelle: Rechtsklick-Menü um „Zelle kopieren" und
  „Zeile kopieren" erweitert.

## Nicht-Ziele (YAGNI)

- Kein Cursor-Markieren von Text (in Fyne nicht möglich ohne Entry-Felder).
- Feld-Beschriftungen und Section-Überschriften werden NICHT kopierbar —
  sie tragen keinen kopierwürdigen Wert.
- Die Datenfelder im Fenster bleiben `Entry`-Widgets (schon markierbar).

## Komponenten

### 1. copyableLabel (`internal/ui/copyablelabel.go`, neu)

- Typ `copyableLabel` bettet `widget.Label` ein (gleiches Muster wie das
  bestehende `hoverLabel` in `table.go`) und implementiert
  `fyne.SecondaryTappable` über `TappedSecondary(*fyne.PointEvent)`.
- Rechtsklick öffnet via `widget.ShowPopUpMenuAtPosition` ein Menü mit
  einem Eintrag „Kopieren".
- „Kopieren" schreibt `label.Text` in die Zwischenablage:
  `fyne.CurrentApp().Clipboard().SetContent(text)`.
- Das Canvas für das Popup wird über
  `fyne.CurrentApp().Driver().CanvasForObject(widget)` ermittelt — das
  Widget braucht keine Fenster-Referenz.
- Konstruktor `newCopyableLabel(text string) *copyableLabel`. `Wrapping`
  und `Alignment` des eingebetteten Labels bleiben von außen setzbar.

### 2. „Rechnungsdaten prüfen"-Fenster (`internal/ui/invoicemodal.go`)

- Das Label der **Dateiname-Vorschau** (`filenamePreview`) wird ein
  `copyableLabel`.
- Das **Anhänge-Label** (`Anhänge (N): …`) wird ein `copyableLabel`.
- Die `Wrapping`-Einstellung dieser Labels bleibt erhalten.
- Übrige Labels (Beschriftungen, „Originaldatei", Section-Texte) bleiben
  unverändert; das Originaldatei-Feld ist bereits ein `Entry`.

### 3. Einstellungen (`internal/ui/settings.go`)

- Die Anzeige-/Hinweis-Labels auf den Unterseiten werden `copyableLabel`,
  insbesondere die Hinweistexte (z. B. der Token-Hinweis `templateHelp`,
  `columnHint`, `debugHint`, `accountsNote`).
- Section-Überschriften und Form-Beschriftungen bleiben einfache Labels.

### 4. Rechnungstabelle (`internal/ui/table.go`)

- `InvoiceTable` merkt sich statt nur `lastSelectedRow` künftig die volle
  zuletzt selektierte Zelle: zusätzliches Feld `lastSelectedCol int`
  (Spaltenindex der Datenspalten, oder -1 für Aktions-/keine Spalte).
  `OnSelected` setzt beide Werte.
- Das Menü in `rightClickTable.TappedSecondary` bekommt zwei neue Einträge
  zusätzlich zum bestehenden „Löschen":
  - **„Zelle kopieren"** — kopiert den Anzeigewert der selektierten Zelle
    (über die bestehende `valueForColumn`/`getCellValue`-Logik).
  - **„Zeile kopieren"** — kopiert alle Spaltenwerte der selektierten Zeile,
    durch Tab getrennt, in Spaltenreihenfolge `columnOrder`.
- Kopiert wird über `it.window.Clipboard().SetContent(...)`.
- Liegt keine gültige selektierte Zelle vor, tun die Einträge nichts.

## Edge Cases

- Leerer Labeltext → „Kopieren" legt leeren String ab (harmlos).
- Tabelle ohne selektierte Zelle (`lastSelectedRow < 0`) → neue Einträge
  sind wirkungslos, kein Absturz.
- Selektierte Zelle in einer Aktions-Spalte (Bearbeiten/Löschen, Col 0/1)
  → „Zelle kopieren" kopiert leeren String; „Zeile kopieren" funktioniert
  weiterhin über die Datenspalten.
- Keine neue Abhängigkeit.

## Betroffene Dateien

- `internal/ui/copyablelabel.go` — neu.
- `internal/ui/invoicemodal.go` — `filenamePreview` + Anhänge-Label.
- `internal/ui/settings.go` — Hinweis-/Anzeige-Labels.
- `internal/ui/table.go` — Zelle-merken + Menü-Erweiterung.

## Tests

- `go build ./...`, `go vet ./...` fehlerfrei.
- Unit-Test für die „Zeile kopieren"-Wertzusammensetzung ist nur sinnvoll,
  wenn die Logik als reine Funktion vorliegt; die Tab-Verkettung wird daher
  als kleine, testbare Hilfsfunktion `joinRowValues([]string) string`
  ausgelegt und mit einem Unit-Test abgedeckt.
- Manuell: Rechtsklick „Kopieren" im Fenster, in den Einstellungen und in
  der Tabelle („Zelle kopieren"/„Zeile kopieren") prüfen; Inhalt der
  Zwischenablage gegenprüfen.
