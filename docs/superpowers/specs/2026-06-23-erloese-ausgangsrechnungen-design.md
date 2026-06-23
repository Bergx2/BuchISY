# E15 — Erlöse / Ausgangsrechnungen verwalten

**Status:** Design (zur Review)
**Datum:** 2026-06-23
**Profil-Kontext:** SKR03 (Bergx2): Erlöse 19 % = 8400, Umsatzsteuer 19 % = 1776.

## Problem

BuchISY ist heute rein **aufwandsseitig**:

- `core.BuildBooking` erzeugt nur Eingangs-/Aufwandsbuchungen: Soll Aufwand + Vorsteuer, Haben Zahlungskonto.
- Das „Ausgangsrechnung"-Häkchen (`invoicemodal.go`) steuert **nur den Ablageordner** (`Unterordner = "Ausgangsrechnungen"`), nicht die Buchung.
- Der DATEV-/Lexware-Export nimmt an: **genau eine Haben-Zahlungszeile** (`Booking.PaymentEntry`) + N Soll-Aufwandszeilen. Eine Erlösbuchung (1 Soll Zahlung + N Haben) würde übersprungen.

**Ziel:** BuchISY soll Erlöse als gleichwertigen Belegtyp verwalten — Ausgangsrechnungen korrekt buchen, exportieren, in UStVA/Controlling/Reports führen und gegen Bank-Gutschriften abgleichen.

## Dekomposition (4 aufeinander aufbauende Sub-Phasen)

| Sub-Phase | Inhalt | Abhängigkeit |
|---|---|---|
| **E15.1 Fundament** | Erlösbuchung erzeugen + DATEV/Lexware beidseitig exportieren | — |
| E15.2 UStVA | Vereinnahmte Umsatzsteuer in der UStVA | E15.1 |
| E15.3 Controlling/Reports | Erlöse getrennt von Aufwand (Einnahmen vs. Ausgaben) in Controlling, Journal, PDF-Reports | E15.1 |
| E15.4 Erlös-Abgleich | Ausgangsrechnungen ↔ eingehende Bank-Gutschriften (Spiegel des Belegabgleichs) | E15.1 |

Dieses Dokument spezifiziert **E15.1** im Detail; E15.2–E15.4 bekommen eigene Specs, wenn sie dran sind.

## E15.1 — Fundament: Erlösbuchung + Export

### 1. Datenmodell

- Neues Feld `Ausgangsrechnung bool` auf `core.Meta` und `core.CSVRow` (+ beide Konvertierungen `ToCSVRow`/`ToMeta`).
- DB: neue Spalte `ausgangsrechnung INTEGER DEFAULT 0` (idempotente `ALTER TABLE`-Migration wie bei `belegnummer`), durch Insert/Update/List gefädelt (NULL-safe Scan).
- CSV: neue Spalte `Ausgangsrechnung` (Default-Spaltenliste + DisplayName + i18n-Key + Read/Write-Pfad).
- Begründung: Der Export braucht ein **verlässliches Richtungs-Signal** unabhängig vom Ablageordner. Beim Lesen von Altzeilen ohne das Feld wird es aus `Unterordner == "Ausgangsrechnungen"` abgeleitet (Rückwärtskompatibilität).

### 2. Konten-Konfiguration (`buchungsregeln.json`)

- Neues Feld `umsatzsteuer_konten map[string]int` (Spiegel zu `vorsteuer_konten`), z. B. `{"19": 1776, "7": 1771}`.
- Neue Methode `BookingRules.UmsatzsteuerKonto(satzProzent) (int, bool)` (analog `VorsteuerKonto`).
- `mergeBundledIntoSaved` füllt `umsatzsteuer_konten` lückenfüllend (wie `vorsteuer_konten`).
- Bundled-Default (SKR04, nur Seed): `{"19": 3806, "7": 3801}`.
- **Daten-Schritt beim Rollout:** Bergx2-Profil-`buchungsregeln.json` bekommt `umsatzsteuer_konten {"19": 1776}` (sonst würde es die SKR04-Bundled-Werte erben).
- Das **Erlöskonto** (z. B. 8400) wird **pro Rechnung** gewählt (genau wie heute das Gegenkonto), nicht in der Config.

### 3. Erlösbuchung (`core.BuildRevenueBooking`)

Spiegel von `BuildBooking`, neue Funktion in `buchung.go`:

```
BuildRevenueBooking(rules *BookingRules, lines []TaxLine, revenueAccount, paymentAccount int) (Booking, error)
```

- **Soll** `paymentAccount` = `round2(SumNetto + SumMwSt)` (brutto) — *Cash-Basis*: das Konto, auf dem das Geld eingeht.
- **Haben** `revenueAccount` = `round2(SumNetto)` (Erlös netto).
- pro Steuerzeile mit `MwStBetrag != 0`: **Haben** `UmsatzsteuerKonto(satz)` = `round2(MwStBetrag)`.
- Kein Trinkgeld (für Erlöse irrelevant).
- Bilanziert: Soll brutto = Haben (netto + Σ USt) = brutto.

*Beispiel Symeo (netto 6.500, USt 19 % = 1.235):* Soll 1200 Sparkasse 7.735 / Haben 8400 Erlöse 6.500 / Haben 1776 USt 1.235.

**Cash-Basis als bewusster Default:** keine offene-Forderung-Verwaltung. Spiegelt exakt die Aufwandsseite (die bucht Haben = Zahlungskonto = „bezahlt"). Der SKR03-Chart des Profils hat ohnehin kein Forderungskonto (1400/Debitoren). Offene Ausgangsrechnungen bucht der Nutzer ggf. in seiner Buchhaltungssoftware.

### 4. Export-Verallgemeinerung

Neue Methode auf `Booking`:

```
PaymentAndCounters(isRevenue bool) (base BookingEntry, counters []BookingEntry, ok bool)
```

- `isRevenue == false`: `base` = die einzige **Haben**-Zeile (Zahlung), `counters` = alle Soll-Zeilen.
- `isRevenue == true`: `base` = die einzige **Soll**-Zeile (Zahlung), `counters` = alle Haben-Zeilen.
- `ok == false`, wenn die Basisseite nicht genau eine Zeile hat (Buchung wird wie bisher übersprungen).

`BuildDATEVStapel` und `BuildLexwareCSV` nutzen das (mit `row.Ausgangsrechnung` als `isRevenue`):

- **DATEV:** pro `counter`: `Umsatz = counter.Betrag`, `S/H = counter.Soll ? "S" : "H"`, `Konto = counter.Konto`, `Gegenkonto = base.Konto`. (Eingang: alle „S" — wie bisher; Erlös: alle „H".)
- **Lexware:** pro `counter`: `Sollkonto/Habenkonto = counter.Soll ? (counter.Konto, base.Konto) : (base.Konto, counter.Konto)`.

`PaymentEntry()` bleibt für nicht betroffene Aufrufer erhalten.

### 5. UI (`invoicemodal.go`, `tableedit.go`)

- Das vorhandene „Ausgangsrechnung"-Häkchen setzt `meta.Ausgangsrechnung` (wird persistiert).
- Bei aktivem Häkchen: Buchungsvorschau rechnet über `computeRevenueBooking` (→ `BuildRevenueBooking`); Konten-Label „Gegenkonto" → „Erlöskonto"; Konto-Vorschlag bevorzugt Erlöskonten (8xxx). Umschalten des Häkchens rechnet die Vorschau neu.
- Keine Buchungskategorie-Auswahl für Erlöse (ein einziger Erlöspfad; Bewirtung/§13b/GWG sind aufwandsspezifisch).

### 6. Tests

- `BuildRevenueBooking`: Bilanz + korrekte Konten (Symeo-Beispiel).
- `PaymentAndCounters`: Eingang (1 Haben-Basis) und Erlös (1 Soll-Basis), inkl. `ok == false` bei mehrdeutiger Basis.
- DATEV/Lexware: je eine Erlöszeile mit korrektem S/H bzw. Soll/Haben.
- DB-Round-Trip des `Ausgangsrechnung`-Flags (Insert/List, Altzeile ohne Spalte).

## Bewusste Defaults / offene Punkte

- **Cash-Basis** (kein offenes Forderungs-Management) — siehe §3.
- **7 %-USt-Konto (SKR03):** im Bergx2-Chart noch zu identifizieren; wird gesetzt, sobald eine 7 %-Ausgangsrechnung auftritt. 19 % = 1776 ist gesetzt.
- **Eingabe** der Ausgangsrechnungen läuft über denselben PDF-/Claude-Weg; `Auftraggeber` = der Kunde.
- **Reconciliation** der Erlöse (gegen Bank-Gutschriften) ist E15.4, nicht Teil von E15.1.

## E15.2–E15.4 (Ausblick, eigene Specs später)

- **E15.2 UStVA:** `ustva.go` summiert zusätzlich die vereinnahmte USt aus Ausgangsrechnungen (Zahllast = USt − Vorsteuer).
- **E15.3 Controlling/Reports:** Erlöskonten getrennt ausweisen (Einnahmen vs. Ausgaben) in `controlling.go`, Buchungsjournal und PDF-Reports.
- **E15.4 Erlös-Abgleich:** Spiegel von `belegabgleich.go` gegen eingehende Gutschriften (`StatementBooking.IstGutschrift == true`, heute ausgeschlossen).
