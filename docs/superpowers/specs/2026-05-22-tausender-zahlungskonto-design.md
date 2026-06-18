# Tausender-Trennzeichen + Umbenennung „Bankkonto" → „Zahlungskonto" — Design

**Datum:** 2026-05-22
**Status:** Genehmigt (Design)

Zwei bestätigte UI-Änderungen, gemeinsam umgesetzt und ausgeliefert.

---

## Teil A — Tausender-Trennzeichen bei Beträgen

### Überblick

Beträge werden mit einem Tausender-Trennzeichen angezeigt und in
Dateinamen geschrieben. Das Tausender-Zeichen ist das jeweils *andere*
Zeichen als das eingestellte Dezimaltrennzeichen: Dezimal `,` →
Tausender `.` (`15.000,00`); Dezimal `.` → Tausender `,` (`15,000.00`).

### Komponenten & Datenfluss

#### A1. Gemeinsamer Formatier-Helfer (`internal/core`)

- Neue Funktion `FormatAmount(value float64, decimalSep string) string`:
  formatiert mit zwei Nachkommastellen, setzt das passende
  Dezimaltrennzeichen und gruppiert die Ganzzahl-Stelle in Dreierblöcken
  mit dem Tausender-Zeichen. Ein Minuszeichen bleibt vorangestellt.
  Beispiele (Dezimal `,`): `15000` → `15.000,00`, `1234567.5` →
  `1.234.567,50`, `-42` → `-42,00`, `19` → `19,00`.
- Liegt in `core`, damit sowohl die UI-Anzeige als auch die
  Dateinamen-Vorlage denselben Helfer nutzen.

#### A2. Anzeige am Bildschirm (`internal/ui`)

- `formatDecimal(v, sep)` (in `kassenbuchview.go`) wird auf
  `core.FormatAmount` umgestellt. Damit zeigen alle bestehenden
  Aufrufstellen Beträge gruppiert: Haupttabelle (Betrag netto, MwSt. %,
  MwSt.-Betrag, Bruttobetrag), Kassenbuch, und die vorbefüllten
  Betragsfelder in „Rechnungsdaten prüfen" und „Rechnung bearbeiten".

#### A3. Eingabe / Speichern (`internal/ui`)

- Die Betragsfelder sind editierbar und werden beim Öffnen des Dialogs
  mit Tausender-Trennzeichen vorbefüllt.
- `parseFloat(s)` (in `invoicemodal.go`) wird Tausender-tolerant: vor dem
  Parsen werden Tausender-Trennzeichen entfernt. Da das Tausender-Zeichen
  vom eingestellten Dezimaltrennzeichen abhängt, prüft die UI das anhand
  von `settings.DecimalSeparator`: ist der Dezimalseparator `,`, werden
  alle `.` entfernt und `,` → `.` ersetzt; ist er `.`, werden alle `,`
  entfernt. Anschließend normales Parsen.
- Während des Tippens wird **nicht** live umformatiert (keine
  Cursor-Sprünge). Ein vom Nutzer eingetippter Wert ohne Trennzeichen
  (`16000`) wird beim Speichern korrekt verarbeitet.

#### A4. Dateiname (`internal/core` Dateinamen-Vorlage)

- Die Dateinamen-Vorlage formatiert den Betrags-Platzhalter künftig über
  `core.FormatAmount` — der Betrag im erzeugten Dateinamen erhält das
  Tausender-Trennzeichen (`…_17.850,00_EUR.pdf`).

#### A5. Nicht betroffen

- `invoices.csv` speichert Beträge unverändert: Punkt als
  Dezimaltrennzeichen, **kein** Tausender-Zeichen (`core/csvrepo.go`
  `formatFloat` bleibt). Es ist das Datenformat, nicht die Anzeige.

### Edge Cases (Teil A)

- Prozentwerte („MwSt. %") sind < 1000 → kein sichtbarer Effekt, werden
  aber einheitlich über denselben Helfer formatiert.
- Negative Beträge (Kassenbuch) → führendes `-`, danach gruppierte
  Ziffern (`-1.234,56`).
- Vom Nutzer eingetippter Text mit gemischten/fehlenden Trennzeichen →
  die Tausender-tolerante Einlese-Logik entfernt Tausender-Zeichen
  bestmöglich; im Zweifel wird der konfigurierte Dezimalseparator als
  maßgeblich angenommen.

---

## Teil B — Umbenennung „Bankkonto" → „Zahlungskonto"

### Überblick

Alle **angezeigten** Beschriftungen „Bankkonto" / „Bankkonten" werden zu
„Zahlungskonto" / „Zahlungskonten". Rein kosmetisch — keine Daten- oder
Verhaltensänderung.

### Umfang

- **Geändert** (nur Anzeigetexte):
  - i18n-Werte in `assets/i18n/de.json` für die Schlüssel rund um
    `field.bankAccount`, `table.col.bankaccount` und die
    Konten-Verwaltung in den Einstellungen → „Zahlungskonto" /
    „Zahlungskonten" / „Zahlungskonto hinzufügen" usw.
  - die entsprechenden Werte in `assets/i18n/en.json` → „Payment account".
  - `ColumnDisplayNames["Bankkonto"]` in `internal/core/csvrepo.go` →
    `"Zahlungskonto"`.
  - etwaige fest verdrahtete deutsche „Bankkonto"/„Bankkonten"-Texte in
    der Einstellungs-UI.
- **Unverändert** (interne Bezeichner):
  - der CSV-Spalten-Bezeichner `Bankkonto` (in `DefaultCSVColumns`, im
    CSV-Header, im JSON-Feld) — sonst würden bestehende CSV-Dateien
    nicht mehr passen.
  - Go-Struktur-Feldnamen (`CSVRow.Bankkonto`, `BankAccount`).
  - i18n-Schlüssel (`field.bankAccount`, `table.col.bankaccount`) — nur
    deren Werte ändern sich.

---

## Betroffene / neue Dateien

- `internal/core/` — neuer `FormatAmount`-Helfer; die Dateinamen-Vorlage
  nutzt ihn; `ColumnDisplayNames` (Teil B).
- `internal/ui/kassenbuchview.go` — `formatDecimal` → `core.FormatAmount`.
- `internal/ui/invoicemodal.go` — `parseFloat` Tausender-tolerant.
- `assets/i18n/de.json`, `assets/i18n/en.json` — Umbenennung (Teil B).
- `internal/ui/settings.go` u. a. — etwaige „Bankkonto"-Anzeigetexte.

## Tests

- `go build ./...`, `go vet ./...` fehlerfrei.
- Unit-Test `core.FormatAmount` (Dezimal `,`): `15000` → `15.000,00`;
  `1234567.5` → `1.234.567,50`; `42` → `42,00`; `-1234` → `-1.234,00`;
  `999` → `999,00`. Mit Dezimal `.`: `15000` → `15,000.00`.
- Unit-Test der Tausender-toleranten Einlese-Logik: `15.000,00`
  (Dezimal `,`) → `15000`; `16000` → `16000`; `1.234.567,50` →
  `1234567.5`.
- Manuell: Beträge in Tabelle, Kassenbuch und beiden Dialogen zeigen das
  Tausender-Trennzeichen; eine Rechnung mit großem Betrag speichern und
  erneut öffnen → Wert unverändert; der erzeugte Dateiname enthält das
  Tausender-Trennzeichen; überall steht „Zahlungskonto" statt
  „Bankkonto".
