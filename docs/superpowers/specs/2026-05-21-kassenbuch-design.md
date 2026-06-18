# Kassenbuch mit Kassenbericht-PDF — Design

**Datum:** 2026-05-21
**Status:** Genehmigt (Design)

## Überblick

Für Konten vom Typ „Barkasse" (`AccountType == cash`) führt BuchISY ein
Kassenbuch je Monat: Anfangsbestand, Bar-Einlagen und die bar bezahlten
Rechnungen ergeben einen fortlaufenden Saldo bis zum Endbestand. Eine
Ansicht erfasst Anfangsbestand und Einlagen; ein Button erzeugt je
Barkassen-Konto einen Kassenbericht als PDF.

## Ziele

- Pro Monat und Barkassen-Konto: Anfangsbestand + Bar-Einlagen erfassen und
  speichern.
- Den fortlaufenden Saldo aus Anfangsbestand + Einlagen + Bar-Rechnungen
  berechnen.
- Je Barkassen-Konto einen Kassenbericht als PDF im Monatsordner erzeugen.

## Nicht-Ziele (YAGNI)

- Kein automatischer Übertrag des Endbestands als Anfangsbestand des
  Folgemonats (Anfangsbestand wird manuell eingegeben).
- Keine Änderung an der Rechnungsverarbeitung oder am Bestätigungsdialog.
- Keine Bearbeitung der Bar-Ausgaben im Kassenbuch (diese kommen
  unverändert aus `invoices.csv`).

## Komponenten & Datenfluss

### 1. Datenmodell + Persistenz (`internal/core/kassenbuch.go`, neu)

- Typ `CashDeposit struct { Datum, Beschreibung string; Betrag float64 }` —
  eine Bar-Einlage (Datum im Format DD.MM.YYYY).
- Typ `CashBook struct { Konto string; Anfangsbestand float64; Einlagen
  []CashDeposit }` — das Kassenbuch eines Barkassen-Kontos für einen Monat.
- Persistenz: eine Datei `kassenbuch.json` je Monatsordner (neben
  `invoices.csv`), Inhalt ein Array von `CashBook` (ein Eintrag je
  Barkassen-Konto). Funktionen `LoadCashBooks(path string) ([]CashBook,
  error)` (fehlende Datei → leeres Ergebnis, kein Fehler) und
  `SaveCashBooks(path string, books []CashBook) error`.

### 2. Saldo-Berechnung (`internal/core/kassenbuch.go`)

- Typ `CashEntry struct { Datum, Beschreibung, Beleg string; Einnahme,
  Ausgabe, Saldo float64 }` — eine Zeile des Kassenberichts.
- Funktion `ComputeCashReport(book CashBook, invoices []CSVRow) (entries
  []CashEntry, endbestand float64)`:
  - Eingaben: das `CashBook` (Anfangsbestand + Einlagen) und die bar
    bezahlten Rechnungen (`CSVRow`, bereits auf das Konto gefiltert).
  - Jede Einlage wird zu einer `CashEntry` mit `Einnahme` gefüllt; jede
    Rechnung zu einer `CashEntry` mit `Ausgabe`.
  - Rechnungsdatum für die Sortierung: `Bezahldatum`, ersatzweise
    `Rechnungsdatum`. Einlagen verwenden ihr `Datum`.
  - Alle Einträge werden chronologisch sortiert; der Saldo startet beim
    Anfangsbestand und läuft je Eintrag mit (`Saldo += Einnahme - Ausgabe`).
  - Rückgabe: die Einträge mit gefülltem `Saldo` und der Endbestand.
  - Reine Funktion ohne Datei-/UI-Zugriff — Unit-getestet.

### 3. Kassenbuch-Ansicht (`internal/ui/kassenbuchview.go`, neu)

- Neuer Button „Kassenbuch" in der oberen Leiste (`app.go`,
  `buildTopBar`), neben „Zielordner öffnen" / „Einstellungen".
- Klick öffnet eine Ansicht für den aktuell gewählten Monat (analog zur
  Einstellungen-Vollseiten-Ansicht: Fensterinhalt wird ersetzt, „Zurück"
  führt zur Hauptansicht).
- Die Ansicht ermittelt die Barkassen-Konten (`a.settings.BankAccounts`
  mit `AccountType == core.AccountTypeCash`). Gibt es keine, zeigt sie
  einen Hinweis.
- Je Barkassen-Konto ein Abschnitt mit:
  - Eingabefeld **Anfangsbestand**.
  - **Einlagen-Liste** — je Einlage Datum/Beschreibung/Betrag editierbar,
    „Entfernen"; ein „+ Einlage"-Button fügt eine Zeile hinzu.
  - read-only Anzeige der **Bar-Ausgaben** des Monats (Rechnungen aus
    `invoices.csv`, deren `Bankkonto` dem Kontonamen entspricht).
  - der berechnete **Endbestand** (über `ComputeCashReport`).
- „Speichern" schreibt `kassenbuch.json`.
- „Kassenbericht PDF" erzeugt die PDF(s) — siehe 4.

### 4. Kassenbericht-PDF (`internal/core/kassenbericht.go`, neu)

- Neue Abhängigkeit `github.com/go-pdf/fpdf` (reines Go, kein CGO).
- Funktion `WriteCashReportPDF(path string, book CashBook, entries
  []CashEntry, endbestand float64, monthLabel string) error`.
- Layout: Kopf (Kontoname, Monat, Anfangsbestand), Tabelle mit Spalten
  Datum | Beschreibung | Beleg | Einnahme | Ausgabe | Saldo, je `CashEntry`
  eine Zeile, abschließend der Endbestand.
- Beträge im Dezimalformat der Einstellungen (`DecimalSeparator`).
- Dateiname: `Kassenbericht_<Konto>_<YYYY-MM>.pdf` im Monatsordner; der
  Kontoname wird über die bestehende `sanitize`-Logik dateinamentauglich
  gemacht.

## Edge Cases

- Kein Barkassen-Konto vorhanden → die Ansicht zeigt einen Hinweis, der
  Button bleibt nutzbar.
- `kassenbuch.json` fehlt → leeres Kassenbuch (Anfangsbestand 0, keine
  Einlagen).
- Rechnung ohne `Bezahldatum` → `Rechnungsdatum` für die Sortierung; fehlt
  auch das, wird der Eintrag ans Ende sortiert.
- Mehrere Barkassen-Konten → je Konto eine eigene PDF.
- Bestehende Monatsordner ohne Kassenbuch sind unberührt; `kassenbuch.json`
  entsteht erst beim ersten Speichern.

## Betroffene / neue Dateien

- `internal/core/kassenbuch.go` — neu: Modell, Laden/Speichern,
  `ComputeCashReport`.
- `internal/core/kassenbuch_test.go` — neu: Tests für `ComputeCashReport`
  und den Persistenz-Round-Trip.
- `internal/core/kassenbericht.go` — neu: PDF-Erzeugung.
- `internal/ui/kassenbuchview.go` — neu: die Kassenbuch-Ansicht.
- `internal/ui/app.go` — geändert: „Kassenbuch"-Button, Rückkehr zur
  Hauptansicht.
- `go.mod` / `go.sum` — `github.com/go-pdf/fpdf`.

## Tests

- `go build ./...`, `go vet ./...` fehlerfrei.
- Unit-Test `ComputeCashReport`: Anfangsbestand + zwei Einlagen + drei
  Rechnungen → korrekte chronologische Sortierung, fortlaufender Saldo,
  Endbestand; Rechnung ohne Bezahldatum fällt auf Rechnungsdatum zurück.
- Unit-Test Persistenz: `SaveCashBooks` → `LoadCashBooks` Round-Trip;
  fehlende Datei → leeres Ergebnis ohne Fehler.
- Manuell: Kassenbuch-Ansicht öffnen, Anfangsbestand + Einlage erfassen,
  speichern, neu öffnen (Werte erhalten); „Kassenbericht PDF" erzeugt eine
  lesbare PDF mit korrektem Saldo im Monatsordner.
