# Ordnerstruktur — Jahresordner + Kategorie-Unterordner — Design

**Datum:** 2026-05-22
**Status:** Genehmigt (Design)

## Überblick

Drei Änderungen an der Ablage-Ordnerstruktur:
1. **Jahresordner:** Monatsordner liegen künftig unter einem Jahresordner —
   `<Ablage>/<JJJJ>/<JJJJ-MM>` statt `<Ablage>/<JJJJ-MM>`.
2. **Kategorie-Unterordner:** Bar-Belege landen im Unterordner `Bar/`,
   Ausgangsrechnungen im Unterordner `Ausgangsrechnungen/` des
   Monatsordners.
3. Zwei einmalige, idempotente Startup-Migrationen bringen die vorhandenen
   Daten in die neue Struktur.

Die `invoices.csv` bleibt **eine pro Monat** (im Monatsordner-Wurzel) und
listet weiterhin alle Rechnungen — so erscheinen Bar- und
Ausgangsrechnungen weiter in der Monatstabelle.

## Nicht-Ziele (YAGNI)

- Keine Änderung an der Rechnungsverarbeitung/Extraktion.
- Bar-Einlagen im Kassenbuch haben keine Beleg-Datei — dort ist nichts
  abzulegen.
- Wenn `UseMonthSubfolders` deaktiviert ist, gibt es keine Monats- und
  damit keine Jahresordner — die Jahresordner-Logik greift nur bei
  aktiven Monatsunterordnern.

## Komponenten & Datenfluss

### 1. Jahresordner (`internal/core/storage.go`)

- `GetMonthFolder(year, month)` liefert künftig
  `filepath.Join(StorageRoot, "<JJJJ>", "<JJJJ-MM>")` (bei aktivem
  `UseMonthSubfolders`; sonst unverändert `StorageRoot`).
- `EnsureMonthFolder` legt über `os.MkdirAll` automatisch auch den
  Jahresordner mit an.
- `ListAllCSVPaths` bleibt unverändert (rekursiver Scan findet die CSVs
  in der tieferen Struktur weiterhin).

### 2. Kategorie-Unterordner + CSV-Spalte (`internal/core/types.go`, `internal/core/csvrepo.go`)

- `CSVRow` bekommt das Feld `Unterordner string` — Werte: `""` (normal),
  `"Bar"`, `"Ausgangsrechnungen"`.
- `Unterordner` wird als neue Spalte in `invoices.csv` geschrieben/gelesen
  (in `DefaultCSVColumns` aufgenommen; bestehende CSVs ohne die Spalte →
  Feld bleibt leer beim Laden).
- Die Beleg-Datei einer Rechnung liegt unter
  `<Monatsordner>/<Unterordner>/<Dateiname>` (bei leerem `Unterordner`
  direkt im Monatsordner). Die `invoices.csv` bleibt im Monatsordner.
- Ein Helfer `core.InvoiceFilePath(monthFolder string, row CSVRow) string`
  bildet diesen Pfad; alle Pfad-Bildungs-Stellen nutzen ihn.

### 3. Einsortierung beim Speichern (`saveInvoice`, `updateInvoice`)

- Der Unterordner einer Rechnung bestimmt sich beim Ablegen/Bearbeiten:
  - „Ausgangsrechnung"-Häkchen gesetzt → `"Ausgangsrechnungen"`.
  - sonst Bankkonto ist ein Konto vom Typ `AccountTypeCash` → `"Bar"`.
  - sonst → `""`.
- `saveInvoice` bekommt einen Parameter `ausgangsrechnung bool`; es
  bestimmt den Unterordner, legt die Haupt-PDF (und etwaige Anhänge) im
  passenden Unterordner ab (`MoveAndRename` mit Zielordner
  `<Monatsordner>/<Unterordner>`) und schreibt `Unterordner` in die
  CSV-Zeile.
- `updateInvoice` bekommt ebenfalls `ausgangsrechnung bool`; es berechnet
  den Unterordner neu und verschiebt die Datei zwischen Unterordnern (bzw.
  Monaten) wie nötig. Quelle ist der bisherige Pfad
  `<Quell-Monatsordner>/<alter Unterordner>/<alter Dateiname>`.

### 4. „Ausgangsrechnung"-Häkchen (`invoicemodal.go`, `tableedit.go`)

- Ein neues Häkchen „Ausgangsrechnung" in „Rechnungsdaten prüfen" und
  „Rechnung bearbeiten".
- Im Bearbeiten-Dialog vorbelegt aus `row.Unterordner ==
  "Ausgangsrechnungen"`. Im Prüf-Dialog standardmäßig nicht gesetzt.
- Der Häkchen-Wert wird an `saveInvoice` / `updateInvoice` übergeben.

### 5. Pfad-Auflösung (mehrere UI-Dateien)

Alle Stellen, die heute den Dateipfad einer Rechnung als
`filepath.Join(monthFolder, row.Dateiname)` bilden, nutzen künftig
`core.InvoiceFilePath(monthFolder, row)` (= inkl. `Unterordner`):
- `tableedit.go` (`showEditDialog` — `originalPath`),
- `tabledelete.go` (`deleteInvoice` — zu löschende Datei),
- `table.go` (Datei öffnen),
- Anhänge: Anhang-Dateien liegen im selben Unterordner wie die Hauptdatei.

### 6. Startup-Migrationen (`internal/core/storage.go`, `internal/ui/app.go`)

Beim App-Start, nach dem Laden der Einstellungen und vor dem ersten
Laden der Rechnungen, laufen — nur bei aktivem `UseMonthSubfolders` —
zwei idempotente Migrationen:

- **Jahresordner-Migration:** Ordner mit Namen `JJJJ-MM` direkt im
  Ablageordner werden nach `<Ablage>/<JJJJ>/<JJJJ-MM>` verschoben. Der
  Jahresordner wird bei Bedarf angelegt. Existiert das Ziel bereits, wird
  der Ordner übersprungen (und das geloggt). Nach erfolgter Migration
  liegen keine `JJJJ-MM`-Ordner mehr direkt im Ablageordner → ein
  erneuter Lauf tut nichts.
- **Bar-Migration:** Für jede `invoices.csv` (rekursiv unter dem
  Ablageordner) wird jede Zeile geprüft: ist `Unterordner` leer, das
  `Bankkonto` ein Konto vom Typ `AccountTypeCash`, und liegt die Datei im
  Monatsordner-Wurzel, so wird die Beleg-Datei nach `Bar/` verschoben und
  `Unterordner = "Bar"` gesetzt; die CSV wird neu geschrieben. Idempotent
  über das `Unterordner`-Feld (bereits gesetzte Zeilen werden
  übersprungen).
- Die Jahresordner-Migration läuft zuerst, danach die Bar-Migration (auf
  der bereits umgezogenen Struktur).

## Edge Cases

- `UseMonthSubfolders` deaktiviert → keine Jahres-/Monats-/Kategorie-
  Ordner; die Migrationen und die Unterordner-Logik greifen nicht.
- Jahresordner-Migration: Ziel `<Ablage>/<JJJJ>/<JJJJ-MM>` existiert
  bereits → diesen Ordner überspringen, Warnung loggen, übrige migrieren.
- Bar-Migration: Beleg-Datei einer Zeile nicht auffindbar → Zeile
  überspringen (`Unterordner` bleibt leer), Warnung loggen.
- Ausgangsrechnung, die bar bezahlt wurde → `Ausgangsrechnungen/` gewinnt
  (die Häkchen-Regel hat Vorrang vor der Bar-Regel).
- Eine bestehende Rechnung wird im Bearbeiten-Dialog mit geänderter
  Kategorie und/oder geändertem Monat gespeichert → `updateInvoice`
  verschiebt die Datei zwischen Unterordnern/Monaten und aktualisiert die
  CSV(s).

## Betroffene / neue Dateien

- `internal/core/storage.go` — `GetMonthFolder` mit Jahresebene;
  `InvoiceFilePath`-Helfer; die beiden Migrations-Funktionen.
- `internal/core/types.go` — `CSVRow.Unterordner`; `DefaultCSVColumns`.
- `internal/core/csvrepo.go` — `Unterordner`-Spalte lesen/schreiben.
- `internal/core/storage_test.go` / `csvrepo_test.go` — Tests (siehe unten).
- `internal/ui/invoicemodal.go` — „Ausgangsrechnung"-Häkchen; `saveInvoice`
  mit Unterordner-Logik.
- `internal/ui/tableedit.go` — „Ausgangsrechnung"-Häkchen; `updateInvoice`
  mit Unterordner-Logik; Pfad-Auflösung.
- `internal/ui/tabledelete.go`, `internal/ui/table.go` — Pfad-Auflösung.
- `internal/ui/app.go` — Aufruf der Migrationen beim Start.

## Tests

- `go build ./...`, `go vet ./...` fehlerfrei.
- Unit-Test `InvoiceFilePath`: leerer `Unterordner` → `<Monat>/<Datei>`;
  gesetzter Unterordner → `<Monat>/<Unterordner>/<Datei>`.
- Unit-Test `GetMonthFolder`: liefert `<Root>/<JJJJ>/<JJJJ-MM>` bei
  aktivem `UseMonthSubfolders`, `<Root>` sonst.
- Unit-Test Jahresordner-Migration: ein temporäres Verzeichnis mit
  `2026-04/` → nach der Migration liegt `2026/2026-04/`; erneuter Lauf
  ändert nichts.
- Unit-Test CSV-Round-Trip: `Unterordner` wird geschrieben und wieder
  gelesen; eine CSV ohne die Spalte lädt mit leerem `Unterordner`.
- Manuell: neue Bar-Rechnung → PDF landet in `<Jahr>/<Monat>/Bar/`;
  Rechnung mit „Ausgangsrechnung"-Häkchen → `…/Ausgangsrechnungen/`;
  bestehende Daten werden beim Start in die Jahres-/Bar-Struktur
  überführt; Öffnen/Bearbeiten/Löschen/Vorschau finden die Datei im
  Unterordner.
