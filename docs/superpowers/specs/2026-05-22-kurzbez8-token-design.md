# Token „erste 8 Zeichen der Kurzbezeichnung" — Design

**Datum:** 2026-05-22
**Status:** Genehmigt (Design)

## Überblick

Die Dateinamen-Vorlage soll einen neuen Token bekommen, der die **ersten
8 Zeichen der Kurzbezeichnung** liefert. Außerdem wird die Vorlage für
beide vorhandenen Profile (Bergx2 GmbH, Boomstraat GmbH) auf ein
festgelegtes Muster gesetzt.

## Komponenten & Datenfluss

### 1. Neuer Token `${Kurzbez8}`

- In der Template-Engine (`internal/core/template.go`) wird ein neuer
  Token `${Kurzbez8}` ergänzt. Er liefert die **ersten 8 Zeichen** von
  `meta.Kurzbezeichnung` — zeichenweise (rune-basiert, umlaut-sicher).
  Ist die Kurzbezeichnung kürzer als 8 Zeichen, wird sie vollständig
  eingesetzt; ist sie leer, ergibt der Token nichts.
- Die bestehenden Tokens `${Kurzbezeichnung}` / `${Kurzbez}` (volle
  Kurzbezeichnung) bleiben unverändert.
- Der Gesamt-Dateiname wird wie bisher über `SanitizeFilename` bereinigt;
  Sonderzeichen in den 8 Zeichen sind damit abgedeckt.

### 2. „Verfügbare Tokens"-Liste in den Einstellungen

- Der Hilfetext in den Einstellungen (i18n-Schlüssel
  `settings.templateHelp` in `de.json` und `en.json`) wird um
  `${Kurzbez8}` erweitert, sodass der Token dort sichtbar angeboten wird.

### 3. Dateinamen-Vorlage

Die festgelegte Vorlage lautet:

```
${YYYY}-${MM}-${DD}_${Company}_${Kurzbez8}_${InvoiceNumber}_${Currency}_${GrossAmount}.pdf
```

(Mit Unterstrich zwischen `${Kurzbez8}` und `${InvoiceNumber}`.)

Sie wird gesetzt:
- als neue **Standard-Vorlage** in `core.DefaultSettings()` — künftige
  Profile bekommen sie automatisch.
- für die **beiden bestehenden Profile** direkt im jeweiligen
  `settings.json` (Feld `naming_template`):
  `%APPDATA%\BuchISY\profiles\Bergx2 GmbH\settings.json` und
  `%APPDATA%\BuchISY\profiles\Boomstraat GmbH\settings.json`. Das
  geschieht beim Deployment, während BuchISY gestoppt ist (kein
  Schreibkonflikt mit der laufenden App).

**Beispiel-Ergebnis:** Kurzbezeichnung „Software Projekt Entwicklung…" →
erste 8 Zeichen „Software" → Dateiname
`2026-05-22_Boomstraat GmbH_Software_17698_EUR_17.850,00.pdf`.

## Edge Cases

- Leere Kurzbezeichnung → `${Kurzbez8}` ergibt einen leeren String; im
  Dateinamen entstünde `…_Company__17698_…` — die Doppel-Trennzeichen
  bereinigt `SanitizeFilename` bzw. sie stören nicht weiter.
- Kurzbezeichnung mit weniger als 8 Zeichen → vollständig eingesetzt.
- Umlaute/mehrbyte-Zeichen → es werden 8 *Zeichen* (runes) genommen, kein
  abgeschnittenes Mehrbyte-Zeichen.

## Betroffene / neue Dateien

- `internal/core/template.go` — neuer Token `${Kurzbez8}` in der
  Ersetzungslogik.
- `internal/core/types.go` — `DefaultSettings().NamingTemplate` auf die
  neue Vorlage.
- `assets/i18n/de.json`, `assets/i18n/en.json` — `settings.templateHelp`
  um `${Kurzbez8}` erweitert.
- `%APPDATA%\BuchISY\profiles\Bergx2 GmbH\settings.json` und
  `…\Boomstraat GmbH\settings.json` — `naming_template` gesetzt (beim
  Deployment).

## Tests

- `go build ./...`, `go vet ./...` fehlerfrei.
- Unit-Test `ApplyTemplate` mit `${Kurzbez8}`: Kurzbezeichnung
  „Software Projekt" → der Token liefert „Software"; Kurzbezeichnung
  „Abc" → „Abc"; leere Kurzbezeichnung → "".
- Manuell: In den Einstellungen ist `${Kurzbez8}` in der Token-Liste
  sichtbar; eine Rechnung speichern → der erzeugte Dateiname enthält die
  ersten 8 Zeichen der Kurzbezeichnung; beide Profile nutzen die neue
  Vorlage.
