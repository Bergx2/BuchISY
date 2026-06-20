# Beleg-Verbuchung: Mehrere MwSt.-Sätze, SKR04, Buchungswissen & Auto-Verbuchung

Status: Design (genehmigt im Brainstorming am 2026-06-20)
Betrifft: `internal/core`, `internal/anthropic`, `internal/db`, `internal/ui`, `assets`

## Ziel

BuchISY entwickelt sich vom Beleg-Organizer zum **belegbasierten Vorbuchungs-Werkzeug**:
ein Beleg kann mehrere MwSt.-Sätze tragen, wird gegen den **SKR04**-Kontenrahmen
verbucht (mit Rückfrage), das Buchungswissen wird pro Beleg gespeichert, und die
Buchungen lassen sich als **DATEV-** und **Lexware-Buchungsstapel** exportieren sowie
fürs **interne Controlling** auswerten.

Treiber-Beispiel: `2026-06-06_Bewirtungsbeleg+Spesenabrechnung+Rechnung_Matche.Rina_…`
— mehrere MwSt.-Sätze (Seite 3), Trinkgeld-Hinweis (Seite 1), SKR04-Buchungshinweise (Seite 2).

## Nicht-Ziele

- Keine vollständige Finanzbuchhaltung/Doppik-Engine; BuchISY liefert **Vorschläge**.
- Keine Steuerberatung. Nichts wird ohne ausdrückliche Bestätigung festgeschrieben.
- Keine automatische Abgabe/Übermittlung an Behörden.

## Komponenten

Ein gemeinsamer Spec, Umsetzung in Phasen (A → B → C → D). A ist sofort testbar.

### Fundament: strukturierte Steuerzeilen

Pro Beleg eine Liste von Steuerzeilen statt Einzelwerten:

```
TaxLine { Netto float64, SatzProzent float64, MwStBetrag float64 }
```

- Ein Beleg = `[]TaxLine` + `Trinkgeld float64` (Sonderposten ohne MwSt.).
- Die bestehenden Einzelfelder bleiben als **Summen** erhalten (Rückwärtskompatibilität,
  Tabellenanzeige, alte CSVs):
  - `BetragNetto` = Σ `Netto`
  - `SteuersatzBetrag` = Σ `MwStBetrag`
  - `Bruttobetrag` = Σ(`Netto` + `MwStBetrag`) + `Trinkgeld`
  - `SteuersatzProzent` = Satz der ersten Zeile (Anzeige), bei mehreren Sätzen ist die
    Detailspalte maßgeblich.
- Die Detailzeilen werden zusätzlich strukturiert gespeichert: neue Spalte
  `Steuerzeilen` (JSON-Array) in CSV und entsprechendes Feld in der DB. `Trinkgeld`
  bekommt eine eigene Spalte/Feld.

Begründung: Eine flache CSV bildet „beliebig viele Sätze" nicht in festen Spalten ab.
Summen-Spalten + eine JSON-Detailspalte halten alte CSVs und den Steuerberater-Export
lesbar und tragen zugleich die volle Information für DATEV/Lexware und die Verbuchung.

### Baustein A — Mehrere MwSt.-Sätze (UI + Extraktion)

UI im Dialog **„Rechnungsdaten prüfen"** (`invoicemodal.go`) und **Bearbeiten**
(`tableedit.go`): der MwSt.-Block wird wiederholbar.

```
 Netto        MwSt. %     Betrag MwSt.
[  14,20 ]   [ 19 ]      [   2,70 ]   ✕
[  18,69 ]   [  7 ]      [   1,31 ]   ✕
[ + MwSt. ]
[ + Trinkgeld ]   [   2,00 ]
 ─────────────────────────────────
 Brutto (Summe):            38,90    (automatisch)
```

- „+ MwSt." fügt eine `TaxLine` hinzu, ✕ entfernt sie. Brutto wird live summiert
  (baut auf der bestehenden `amountcompute.go`-Logik auf).
- Trinkgeld als eigener Posten ohne Netto/MwSt.; fließt nur in die Brutto-Summe.
- **Extraktion (`anthropic/extractor.go`):** der Prompt liefert künftig ein Array
  `steuerzeilen: [{satz, netto, mwst}]` plus optional `trinkgeld`. Ein Beleg mit nur
  einem Satz ergibt eine Zeile — voll abwärtskompatibel. Lokale Heuristik
  (`localextract.go`) erzeugt mindestens eine Zeile aus den erkannten Beträgen.

Backward-Compatibility beim Laden: fehlt die `Steuerzeilen`-Spalte (alte CSVs), wird
aus den Summen-Feldern eine einzelne `TaxLine` rekonstruiert.

### Baustein B — SKR04-Kontenrahmen

- **Gebündelt:** `assets/skr04.json` (eingebettet) — je Konto
  `Nummer, Bezeichnung, Typ, StandardSteuerschlüssel`. Quelle: eine geläufige offene
  SKR04-Standardliste; zu verifizieren mit der Nutzerliste.
- **Import:** „SKR04 importieren…" (Settings) liest die Nutzerliste
  (Vorjahresabschluss / DATEV-/Lexware-Export als CSV/Excel). Nutzerkonten
  überschreiben/ergänzen die Standardliste und werden als „genutzt" markiert
  (erscheinen oben in der Auswahl).
- **Datenmodell:** `core.Account { Number int, Name string, Type string, TaxKey string,
  InUse bool }` und `ChartOfAccounts` (Standard + Overrides). Persistenz im
  Profil-Configdir (`chart_skr04.json`), Standard bleibt eingebettet.
- Das bestehende `Gegenkonto` (int) entspricht der SKR04-Kontonummer. Kontoauswahl
  überall durchsuchbar (nutzt vorhandenes `highlightedselect.go`).

### Baustein C — Buchungswissen pro Beleg

- Pro Beleg gespeichert: verwendete Konten + Kurzbegründung + ggf. auf dem Beleg
  gedruckte Buchungshinweise. Feld `BuchungInfo` (Text/JSON) in DB + CSV-Detailspalte.
- **Wiederverwendung:** Erweiterung der bestehenden Firma→Konto-Zuordnung
  (`companymap.go`) zu **Firma/Kategorie → vollständige Buchungsvorlage**
  (`BookingTemplate`). Wiederkehrende Lieferanten werden deterministisch vorbelegt.
- **Regel-/Wissensbasis (gebündelt):** kompakter Satz typischer Fälle (Bewirtung 70/30,
  Trinkgeld, Vorsteuer-Abzug) als nachschlagbare Regeln (`assets/buchungsregeln.json`),
  damit Engine und Claude den Kontext kennen.

### Baustein D — Auto-Verbuchung mit Rückfrage + Export

**Buchungsvorschlag** pro Beleg = `[]BookingEntry { Konto, Betrag, Soll/Haben,
Steuerschlüssel }`, abgeleitet aus den `TaxLine`s (A) + Belegart + SKR04 (B) +
Regeln/Memory (C).

Beispiel Bewirtung (brutto 38,90, davon Trinkgeld 2,00):
```
Soll  6640  Bewirtung abziehbar 70%     …
Soll  6644  Bewirtung nicht abz. 30%    …
Soll  1406  Abziehbare Vorsteuer 19%    …   (Vorsteuer 100%)
Soll  <Trinkgeld-Konto>                 2,00
Haben <Zahlungskonto: Bank/Kasse/KK>   38,90
```

- **Engine: Hybrid.** Bekannte Lieferanten → deterministisch aus `BookingTemplate`
  (keine KI). Neue/komplexe Belege → **Claude** schlägt vor (nutzt Kontenrahmen +
  Regeln + Belegtext/-bild).
- **Immer bestätigen:** ein Buchungs-Dialog zeigt den Vorschlag; der Nutzer
  prüft/korrigiert/bestätigt. Nichts wird ohne Bestätigung gespeichert oder exportiert.
  Nach Bestätigung wird der Buchungssatz am Beleg gespeichert und (optional) die
  `BookingTemplate` der Firma aktualisiert.
- **Export:**
  - **DATEV-Buchungsstapel (EXTF-CSV)** — benötigt Header (Berater-/Mandantennummer,
    Wirtschaftsjahr) aus den Profil-Einstellungen.
  - **Lexware-Buchungsimport** — Format gemäß Zielprodukt (zu bestätigen).
  - Beide lesen die gespeicherten `BookingEntry`s; Steuerschlüssel je SKR04-Konto.
- **Controlling:** gespeicherte Buchungen speisen Auswertungen; `jahresuebersicht.go`
  und `kassenbuch.go` werden um Konten-/Steuer-Auswertung erweitert.

## Datenfluss

1. Beleg-Extraktion → `Meta` mit `[]TaxLine` + `Trinkgeld` (A).
2. „Rechnungsdaten prüfen": Nutzer prüft Steuerzeilen, bestätigt Beleg.
3. Buchungsschritt: Engine erzeugt `[]BookingEntry` (D) aus TaxLines + SKR04 (B) +
   Template/Regeln (C); Claude bei neuen Belegen.
4. Buchungs-Dialog „Buchung ok?": Nutzer bestätigt/korrigiert.
5. Speichern: Beleg + Steuerzeilen + Buchungssatz + BuchungInfo in DB/CSV; ggf.
   Template lernen.
6. Export: DATEV-/Lexware-Stapel auf Knopfdruck; Controlling-Auswertungen.

## Sicherheit / Haftung

- UI-Hinweis: BuchISY liefert **Buchungsvorschläge**, keine Steuerberatung.
- Keine stille Festschreibung; jede Buchung wird bestätigt.
- Exporte sind klar als „Buchungsstapel zur Prüfung durch den Steuerberater" benannt.

## Tests

- `TaxLine`-Summenlogik (Netto/MwSt./Brutto inkl. Trinkgeld), Round-Trip CSV/DB,
  Backward-Compat (alte CSV ohne `Steuerzeilen` → eine Zeile).
- Extraktions-Parsing des `steuerzeilen`-Arrays (inkl. Einzelsatz-Fall).
- SKR04 Laden/Import/Override; Kontosuche.
- Buchungsregeln: Bewirtung 70/30, Vorsteuer 100%, Trinkgeld separat.
- Export-Format als Golden-Files (DATEV EXTF, Lexware); Steuerschlüssel-Mapping.
- Template-Lernen/-Anwenden je Firma.

## Offene Punkte (vom Nutzer nachzuliefern)

1. **DATEV-Header:** Berater-/Mandantennummer + Wirtschaftsjahr.
2. **Lexware:** konkretes Produkt/Importformat.
3. **Konten/Steuerschlüssel:** Abgleich mit der **Kontenliste des Vorjahresabschlusses**
   (Nutzer liefert nach); bis dahin Standard-SKR04 (z. B. 6640/6644/1406) als Annahme.

## Phasen (Umsetzungsreihenfolge)

1. **A** — Steuerzeilen-Datenmodell + UI „+ MwSt."/Trinkgeld + Extraktion. Sofort testbar.
2. **B** — SKR04 bündeln + Import + Kontenmodell/-auswahl.
3. **C** — BuchungInfo pro Beleg + BookingTemplate (Firma) + Regelbasis.
4. **D** — Buchungs-Engine (Hybrid) + Bestätigungs-Dialog + DATEV-/Lexware-Export +
   Controlling-Erweiterung.
