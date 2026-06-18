# UI-Polish-Paket (Kassenbuch-Beleg, MwSt.-Labels, Zoom-Vorschau, Datumszeile) — Design

**Datum:** 2026-05-22
**Status:** Genehmigt (Design)

## Überblick

Vier UI-Verbesserungen an den Rechnungs-Dialogen, der Belegvorschau und
dem Kassenbuch:
1. Im Kassenbuch wird der Beleg-Dateiname angezeigt und ist anklickbar
   (öffnet „Rechnung bearbeiten"); dieser Dialog bekommt einen
   „Löschen"-Button.
2. „Steuersatz in %" / „Steuerbetrag" werden zu „MwSt. %" /
   „MwSt.-Betrag"; Betrag netto + MwSt. % + MwSt.-Betrag stehen in einer
   Zeile.
3. Die Belegvorschau wird zoombar.
4. Rechnungsdatum/Bezahldatum-Labels ohne „(DD.MM.YYYY)", beide in einer
   Zeile.

## Nicht-Ziele (YAGNI)

- Keine echte Pinch-Geste (vom UI-Framework nicht zuverlässig unterstützt).
- Keine Änderung der Rechnungsverarbeitung oder der CSV-Struktur.

## Komponenten & Datenfluss

### 1. Beleg im Kassenbuch sichtbar & klickbar (`internal/ui/kassenbuchview.go`, `internal/ui/tableedit.go`)

- In `showCashBookView` zeigt die Bar-Ausgaben-Liste je Ausgabe-Eintrag
  zusätzlich den **Dateinamen** (`CashEntry.Beleg`).
- Jede Bar-Ausgaben-Zeile wird anklickbar (Button mit `LowImportance`,
  wie die Monatszeilen der Jahresübersicht). Klick öffnet
  `showEditDialog` für die zugehörige Rechnung.
  - `showEditDialog` erhält dazu einen optionalen Abschluss-Callback
    `onClose func()`. Er wird in `editWin.SetOnClosed` ausgelöst. Der
    bestehende Aufruf aus der Tabelle (`table.go`) übergibt `nil`; der
    Kassenbuch-Aufruf übergibt einen Callback, der die Kassenbuch-Ansicht
    neu rendert (das vorhandene `rebuild`).
  - Die zugehörige `core.CSVRow` stammt aus den bereits geladenen
    `cashInvoicesFor(account)`-Daten; die Bar-Ausgaben-Zeilen werden direkt
    aus dieser Liste gebaut.
- `showEditDialog` bekommt einen **„Löschen"-Button** in der
  Schaltflächenleiste: ruft die bestehende
  `showDeleteConfirmation(row)`-Logik (`tabledelete.go`) auf; bei
  bestätigter Löschung schließt sich das Bearbeiten-Fenster.

### 2. MwSt.-Bezeichnungen + gemeinsame Zeile (`invoicemodal.go`, `tableedit.go`, i18n)

- i18n-Umbenennungen (de.json und en.json):
  - `field.vatPercent`: „Steuersatz in %" → „MwSt. %" (en: „VAT %").
  - `field.vatAmount`: „Steuerbetrag" → „MwSt.-Betrag" (en: „VAT Amount").
  - `table.col.vatPercent` / `table.col.vatAmount` analog — damit Dialog
    und Tabellenspalten konsistent sind.
- In `showConfirmationModal` und `showEditDialog` werden die drei
  bisher getrennten Formularzeilen Betrag netto, MwSt. %, MwSt.-Betrag zu
  **einer Zeile** zusammengefasst: ein Formular-Eintrag mit leerem Label
  und einem `container.NewGridWithColumns(3, …)`, dessen drei Zellen je
  ein Label + Eingabefeld enthalten.

### 3. Zoombare Belegvorschau (`internal/ui/documentpreview.go`)

- Die Vorschau-Komponente bekommt einen Zoomfaktor (Start 1.0). Der
  `pdfPreviewStrip` rendert die Seiten mit `Panelbreite × Zoom` statt nur
  Panelbreite.
- **Zoom-Buttons** im Vorschau-Bereich: „+", „−", „100%". Schrittweite
  z. B. 0,25; sinnvolle Grenzen (z. B. 0,5 bis 4,0).
- **Strg + Mausrad** (bzw. Strg + Zwei-Finger-Scroll am Touchpad) zoomt
  die Vorschau: das Vorschau-Fenster verfolgt den Strg-Status über die
  Tastatur-Events seines Canvas; ein Scroll-Ereignis bei gedrücktem Strg
  ändert den Zoom statt zu scrollen.
- Bei Zoom > 1.0 ist der Inhalt breiter/höher als das Panel — der
  Vorschau-Container scrollt dann horizontal **und** vertikal
  (`container.NewScroll` statt `NewVScroll`).
- `buildDocumentPreview` bleibt die öffentliche Funktion; sie liefert
  weiterhin ein `fyne.CanvasObject` (jetzt mit Zoom-Leiste + scrollbarem
  Bereich).

### 4. Datumsfelder (`invoicemodal.go`, `tableedit.go`, i18n)

- i18n-Umbenennungen (de.json und en.json):
  - `field.invoiceDate`: „Rechnungsdatum (DD.MM.YYYY)" → „Rechnungsdatum".
  - `field.paymentDate`: „Bezahldatum (DD.MM.YYYY)" → „Bezahldatum".
- In beiden Dialogen werden die bisher getrennten Zeilen Rechnungsdatum
  und Bezahldatum zu **einer Zeile** zusammengefasst: ein Formular-Eintrag
  mit leerem Label und einem `container.NewGridWithColumns(2, …)`, dessen
  zwei Zellen je Label + (Eingabefeld mit Kalender-Button) enthalten.

## Edge Cases

- Kassenbuch-Beleg ohne passende `CSVRow` (Datei extern entfernt) → kommt
  nicht vor, da die Zeilen direkt aus den geladenen CSV-Rechnungen gebaut
  werden.
- Löschen aus dem Bearbeiten-Dialog: bricht der Nutzer die Sicherheits-
  abfrage ab, bleibt das Bearbeiten-Fenster offen.
- Zoom an den Grenzen (Min/Max) → der jeweilige Button bewirkt nichts
  weiter; kein Fehler.
- Strg + Scroll außerhalb der Vorschau (im Formularbereich) → normales
  Scrollen, kein Zoom.

## Betroffene / neue Dateien

- `internal/ui/kassenbuchview.go` — Bar-Ausgaben-Zeilen mit Dateiname +
  klickbar; Kassenbuch-Refresh nach dem Bearbeiten.
- `internal/ui/tableedit.go` — `showEditDialog` erhält `onClose`-Callback
  und „Löschen"-Button; MwSt.-/Datums-Zeilen-Layout.
- `internal/ui/invoicemodal.go` — MwSt.-/Datums-Zeilen-Layout.
- `internal/ui/documentpreview.go` — Zoomfaktor, Zoom-Buttons,
  Strg+Scroll, beidseitiges Scrollen.
- `internal/ui/table.go` — der bestehende `showEditDialog`-Aufruf
  übergibt `nil` als `onClose`.
- `assets/i18n/de.json`, `assets/i18n/en.json` — Label-Umbenennungen.

## Tests

- `go build ./...`, `go vet ./...` fehlerfrei; bestehende `go test ./...`
  bleiben grün (reine UI-Änderungen, keine neuen Unit-Tests — die UI ist
  nicht headless testbar).
- Manuell:
  - Kassenbuch: Bar-Ausgaben zeigen den Dateinamen; Klick öffnet „Rechnung
    bearbeiten"; nach Speichern/Löschen/Verschieben ist das Kassenbuch
    aktualisiert.
  - „Löschen" im Bearbeiten-Dialog entfernt Rechnung + Datei nach
    Bestätigung.
  - Beide Dialoge: Betrag netto / MwSt. % / MwSt.-Betrag in einer Zeile;
    Rechnungsdatum / Bezahldatum in einer Zeile; Labels ohne
    „(DD.MM.YYYY)".
  - Belegvorschau: „+/−/100%" und Strg+Mausrad zoomen; bei Zoom > 100 %
    erscheinen Scrollbalken in beide Richtungen.
