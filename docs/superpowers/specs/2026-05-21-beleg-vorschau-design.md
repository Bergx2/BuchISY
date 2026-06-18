# Beleg-Vorschau im „Rechnungsdaten prüfen"-Fenster — Design

**Datum:** 2026-05-21
**Status:** Genehmigt (Design)

## Überblick

Das „Rechnungsdaten prüfen"-Fenster (Bestätigung beim Ablegen einer
Rechnung) zeigt aktuell nur das Formular. Dieses Feature teilt das Fenster
zweispaltig: links die Eingabefelder (dadurch nur noch ~1/3 breit), rechts
eine Vorschau der Belegdatei. Bei PDFs werden die extrahierten Werte im
Vorschaubild gelb markiert.

## Ziele

- Fenster zweigeteilt: Formular links, Beleg-Vorschau rechts.
- Eingabefelder dadurch ~1/3 der bisherigen Breite.
- PDF: alle Seiten als Bild gerendert, vertikal scrollbar.
- Bilddateien: direkt angezeigt.
- Office/LibreOffice: Platzhalter (visuelles Rendern nicht möglich).
- PDF: extrahierte Werte im Bild gelb hinterlegt (Best-Effort).

## Nicht-Ziele (YAGNI)

- Kein visuelles Rendern von Office-/LibreOffice-Dokumenten (technisch nur
  mit mitgeliefertem LibreOffice möglich — außerhalb des Rahmens).
- Keine Markierung bei Bild- oder Office-Dateien (keine durchsuchbare
  Textebene).
- Keine Bearbeitung/Verschiebung der Markierungen durch den Nutzer.
- Kein markierbarer/kopierbarer Text — separates Folge-Feature.

## Komponenten & Datenfluss

### 1. Fenster-Layout (`internal/ui/invoicemodal.go`)

- Der Fensterinhalt wird ein `container.NewHSplit(formPanel, previewPanel)`.
- `formPanel` = der bisherige scrollbare Formularbereich.
- `previewPanel` = die neue Vorschau-Komponente.
- Split-Offset initial ~0,33 (Formular links 1/3, Vorschau rechts 2/3);
  vom Nutzer verschiebbar.
- Startgröße des Fensters ~1500×850.
- Die Button-Leiste (Speichern/Abbrechen) bleibt unten über die volle Breite.

### 2. PDF-Rendering (`internal/core/pdfrender.go`, neu)

- Wrapper um `github.com/gen2brain/go-fitz` (MuPDF, CGO).
- Funktion `RenderPDF(path string, dpi float64) ([]image.Image, error)`:
  rendert jede Seite zu einem `image.Image`. Die Pixelmaße jeder Seite
  ergeben sich aus `image.Bounds()`; die Markierungs-Komponente arbeitet
  mit demselben `dpi`-Wert (siehe Schritt 3).
- DPI-Standard 150.
- Schlägt das Rendern fehl (z. B. go-fitz nicht baubar/Datei kaputt), gibt
  die Funktion einen Fehler zurück; die UI zeigt dann den Platzhalter.

### 3. Markierungs-Berechnung (`internal/core/pdfhighlight.go`, neu)

- Typ `Rect struct { X, Y, W, H float32 }` — ein Rechteck in Bildpixeln.
- Funktion `HighlightRects(path string, values []string, dpi float64) ([][]Rect, error)`:
  liefert je PDF-Seite eine Liste gelber Rechtecke in **Bildpixeln**. Der
  `dpi`-Wert muss derselbe sein wie beim Rendern (Schritt 2), damit die
  Pixel-Koordinaten zum Seitenbild passen; der Skalierungsfaktor ist
  `dpi/72`. Die Seitenhöhe je Seite kommt aus `ledongthuc/pdf` selbst
  (MediaBox der Seite).
- Wortpositionen über `ledongthuc/pdf` `page.GetTextByRow()` — jedes
  Textfragment hat X, Y, Breite in PDF-Punkten.
- Für jeden gesuchten Wert: Wert normalisieren (trim) und im
  zusammengesetzten Seitentext suchen; die abgedeckten Textfragmente
  ergeben ein umschließendes Rechteck.
- Umrechnung PDF-Punkte → Bildpixel: `px = pt * scale`; Y-Achse spiegeln
  (`py = (seitenHoehe - pt_y) * scale`), da PDF-Ursprung unten-links,
  Bild-Ursprung oben-links liegt. `scale` und Seitenhöhe kommen aus
  `PageDims` von Schritt 2.
- Gesuchte Werte: Firmenname, Rechnungsnummer, BetragNetto,
  SteuersatzBetrag, Bruttobetrag, Währung. Leere Werte werden übersprungen.
- Werte, die nicht wörtlich gefunden werden, liefern kein Rechteck — keine
  Markierung, kein Fehler.

### 4. Vorschau-Komponente (`internal/ui/documentpreview.go`, neu)

- Funktion `buildDocumentPreview(mainPath string, meta core.Meta) fyne.CanvasObject`.
- Sofort: zeigt „Vorschau wird geladen…".
- Im Hintergrund-Goroutine, abhängig vom Dateityp der Hauptdatei:
  - **PDF** (`core.IsPDF`): `RenderPDF` + `HighlightRects`; pro Seite ein
    Stapel aus `canvas.Image` (Seitenbild) und halbtransparenten gelben
    `canvas.Rectangle` an den berechneten Pixelpositionen. Alle Seiten
    vertikal in einem `VScroll` untereinander.
  - **Bilddatei** (Endung jpg/jpeg/png/gif/bmp/tif/tiff/webp): `canvas.Image`
    aus der Datei, skaliert in einem `VScroll`.
  - **Sonst** (Office/LibreOffice): zentrierter Platzhalter-Text
    „Keine Bildvorschau für dieses Dateiformat verfügbar" + Dateiname.
- Markierungs-Rechtecke skalieren mit dem angezeigten Bild mit (eigener
  kleiner Layout-Container, der Bild + Rechtecke gemeinsam skaliert).

### 5. go.mod

- Neue Abhängigkeit `github.com/gen2brain/go-fitz`.
- CGO ist bereits aktiv (Fyne). go-fitz bringt vorkompilierte MuPDF-Libs
  für windows/amd64 mit.

## Edge Cases

- go-fitz baut/rendert nicht → PDF-Zweig zeigt den Platzhalter, restliche
  App unberührt. Wird beim Umsetzen als erster Schritt verifiziert.
- Mehrseitige PDFs → alle Seiten gerendert und scrollbar.
- PDF ohne Textebene (Scan) → Rendern klappt, Markierung liefert nichts.
- Nicht-PDF-Hauptdatei → keine Markierung (Manuell-Eingabe-Fall).
- Sehr große PDFs → Rendering im Hintergrund, UI blockiert nicht.

## Betroffene Dateien

- `internal/ui/invoicemodal.go` — Fenster wird HSplit, Vorschau eingehängt.
- `internal/core/pdfrender.go` — neu, go-fitz-Wrapper.
- `internal/core/pdfhighlight.go` — neu, Wert→Rechteck-Berechnung.
- `internal/ui/documentpreview.go` — neu, Vorschau-Komponente.
- `go.mod` / `go.sum` — go-fitz.

## Tests

- `go build ./...`, `go vet ./...` fehlerfrei.
- Unit-Test für die Koordinaten-Umrechnung in `pdfhighlight.go` (PDF-Punkt
  → Bildpixel, inklusive Y-Spiegelung) mit bekannten Eingaben.
- Unit-Test für die Wert-Suche (Wert wörtlich vorhanden → Rechteck; Wert
  nicht vorhanden → leer).
- Manuell: PDF-Rechnung (ein- und mehrseitig), Bilddatei, Office-Datei
  jeweils im Fenster prüfen; Markierungen sichtbar und plausibel platziert.
