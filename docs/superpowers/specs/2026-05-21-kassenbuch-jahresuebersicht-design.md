# Kassenbuch-Jahresübersicht — Design

**Datum:** 2026-05-21
**Status:** Genehmigt (Design)

## Überblick

Eine neue, rein lesende Ansicht zeigt für ein Barkassen-Konto und ein Jahr
alle zwölf Monate auf einen Blick: je Monat Anfangsbestand, Einnahmen,
Ausgaben und Endbestand. Der Saldo wird Januar → Dezember fortlaufend
gerollt — auch durch Monate, für die noch kein Kassenbuch erfasst wurde.

## Ziele

- Pro Barkasse und Jahr eine Tabelle aller zwölf Monate mit
  Anfangsbestand, Einnahmen, Ausgaben, Endbestand.
- Aus der Übersicht direkt in das Kassenbuch eines Monats springen.

## Nicht-Ziele (YAGNI)

- Rein lesend — die Übersicht speichert nichts.
- Keine Änderung an `ComputeCashReport` oder am Datenmodell.
- Kein Mehrjahres-Vergleich, keine Summen über das Jahr hinaus.

## Komponenten & Datenfluss

### 1. Berechnung (`internal/core/jahresuebersicht.go`, neu)

Zwei Eingabe-/Ausgabe-Typen und eine reine Funktion:

```go
type MonthInput struct {
    HasStoredBook bool        // ist für den Monat ein Kassenbuch gespeichert?
    Book          CashBook    // gültig nur wenn HasStoredBook
    Invoices      []CSVRow    // bar bezahlte Rechnungen des Monats (auf das Konto gefiltert)
}

type MonthSummary struct {
    Month          time.Month
    Anfangsbestand float64
    Einnahmen      float64
    Ausgaben       float64
    Endbestand     float64
}
```

- Funktion `ComputeYearOverview(carriedIn float64, months []MonthInput)
  []MonthSummary`:
  - `months` enthält die Monate des Jahres in Reihenfolge (Januar zuerst).
  - Je Monat:
    - Hat der Monat ein gespeichertes Kassenbuch → `Anfangsbestand =
      Book.Anfangsbestand`, gerechnet wird mit `Book` (inkl. dessen
      Einlagen).
    - Sonst → `Anfangsbestand =` Endbestand des Vormonats (für den ersten
      Monat: `carriedIn`); gerechnet wird mit einem leeren Kassenbuch, das
      mit diesem Anfangsbestand gestartet wird.
    - `entries, end := ComputeCashReport(book, months[i].Invoices)`;
      `Einnahmen =` Summe der `entries[].Einnahme`, `Ausgaben =` Summe der
      `entries[].Ausgabe`, `Endbestand = end`.
  - Reine Funktion ohne Datei-/UI-Zugriff — Unit-getestet.

### 2. Übertrag-Helfer (`internal/ui/kassenbuchview.go`, Refactoring)

Die vorhandene Rückwärts-Logik (in `showCashBookView` als Closure
`carryOver`) wird zu einer wiederverwendbaren Methode extrahiert:

```go
// cashCarryIn liefert den ins (year, month) übertragenen Anfangsbestand:
// rückwärts bis zum letzten gespeicherten Kassenbuch, dann vorwärts
// gerollt. ok ist false, wenn es davor kein gespeichertes Kassenbuch gibt.
func (a *App) cashCarryIn(account string, year int, month time.Month) (float64, bool)
```

- Der Funktionsrumpf ist die heutige Walk-Back-Logik, nur mit
  `year`/`month` als Parameter statt `a.currentYear`/`a.currentMonth`.
- `showCashBookView` nutzt künftig `a.cashCarryIn(account, a.currentYear,
  a.currentMonth)` — Verhalten unverändert.
- Die Jahresübersicht nutzt `a.cashCarryIn(account, year, time.January)`
  für den `carriedIn`-Wert des Januar.

### 3. Jahresübersicht-Ansicht (`internal/ui/kassenbuchview.go`)

- In `showCashBookView` kommt im Kopfbereich (neben „Speichern" /
  „Kassenbericht PDF") ein Button **„Jahresübersicht"**, der
  `showCashYearView(account, a.currentYear)` für das aktuell gewählte
  Konto und Jahr öffnet.
- `showCashYearView(account string, year int)` ersetzt den Fensterinhalt
  (Vollseiten-Ansicht wie das Kassenbuch):
  - **Kopf:** Titel „Jahresübersicht — <Jahr>", ein **Konto-Auswahlfeld**
    (alle Barkassen-Konten), ein **Jahr-Auswahlfeld**, ein „Zurück"-Button
    (führt zur Hauptansicht). Ändern von Konto oder Jahr rendert die
    Tabelle neu.
  - **Tabelle:** zwölf Zeilen (Januar–Dezember), Spalten `Monat |
    Anfangsbestand | Einnahmen | Ausgaben | Endbestand`. Beträge im
    Dezimalformat der Einstellungen.
  - Jede Monatszeile ist anklickbar und öffnet das Kassenbuch dieses
    Monats: setzt den Monat (synchron zur Monatsauswahl der oberen Leiste)
    und ruft `showCashBookView` auf.

### 4. Daten

`showCashYearView` lädt je Monat des Jahres:
- `kassenbuch.json` aus dem Monatsordner → das Kassenbuch des Kontos
  (gefunden → `HasStoredBook = true`).
- `invoices.csv` → die auf das Konto gebuchten Rechnungen
  (`cashInvoicesForMonth`).
Daraus baut sie die zwölf `MonthInput`, ermittelt `carriedIn` über
`cashCarryIn(account, year, time.January)` und ruft `ComputeYearOverview`.

## Edge Cases

- Kein Barkassen-Konto → der „Jahresübersicht"-Button erscheint nur im
  Normalzweig von `showCashBookView`, der ohnehin nur bei vorhandenen
  Barkassen gerendert wird.
- Monat ohne gespeichertes Kassenbuch und ohne Rechnungen → Anfangsbestand
  = Vormonats-Endbestand, Einnahmen/Ausgaben 0, Endbestand =
  Anfangsbestand.
- Monat mit gespeichertem Kassenbuch → dessen gespeicherter
  Anfangsbestand gilt (kann vom Vormonats-Endbestand abweichen — bewusst,
  der gespeicherte Wert ist maßgeblich).
- Gibt es vor Januar kein gespeichertes Kassenbuch → `carriedIn = 0`.

## Betroffene / neue Dateien

- `internal/core/jahresuebersicht.go` — neu: `MonthInput`,
  `MonthSummary`, `ComputeYearOverview`.
- `internal/core/jahresuebersicht_test.go` — neu: Test für
  `ComputeYearOverview`.
- `internal/ui/kassenbuchview.go` — geändert: `carryOver`-Closure zu
  Methode `cashCarryIn` extrahiert; „Jahresübersicht"-Button;
  neue Funktion `showCashYearView`.

## Tests

- `go build ./...`, `go vet ./...` fehlerfrei.
- Unit-Test `ComputeYearOverview`: ein Jahr mit `carriedIn`, einzelnen
  Monaten mit gespeichertem Kassenbuch (eigener Anfangsbestand + Einlagen)
  und Monaten ohne Kassenbuch aber mit Rechnungen → korrekter
  fortlaufender Saldo, korrekte Einnahmen/Ausgaben je Monat, der
  gespeicherte Anfangsbestand überschreibt den Vormonats-Übertrag.
- Manuell: Kassenbuch öffnen → „Jahresübersicht" → Tabelle aller zwölf
  Monate; Konto/Jahr wechseln aktualisiert die Tabelle; Klick auf eine
  Monatszeile öffnet das Kassenbuch des Monats.
