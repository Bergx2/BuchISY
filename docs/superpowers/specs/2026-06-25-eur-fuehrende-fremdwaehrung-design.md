# EUR-führende Fremdwährungsbehandlung — Design / Spec

**Status:** Entwurf zur Review
**Datum:** 2026-06-25
**Kontext:** BuchISY führt Fremdwährungsbelege aktuell *in der Fremdwährung* (z. B. `BetragNetto = 200` USD); EUR ist nur ein Nebenfeld (`BetragNetto_EUR`). Auto-Buchung und mehrere Auswertungen rechnen mit den Fremdwährungs-Nennwerten. Deutsche Buchführung (HGB/GoBD) ist aber **EUR-führend** — der EUR-Betrag ist der Buchungswert, die Fremdwährung nur Dokumentation.

## Ziel
Für Fremdwährungsbelege ist der **EUR-Betrag der führende Buchungswert** in der gesamten App: Buchung, alle Auswertungen (UStVA, SuSa, GuV, OPOS, Controlling, Summen, Reports, CSV) und der Belegabgleich rechnen mit EUR. Der **Original-Fremdwährungsbetrag, die Währung und der Kurs bleiben als Dokumentation** erhalten und werden in der Tabelle weiterhin mit Code angezeigt (z. B. „USD 200,00").

## Grundsatzentscheidung (bestätigt)
- **Kursquelle: manuell.** Der Anwender trägt den Wechselkurs ein; EUR = Fremdbetrag ÷ Kurs. Kurs-Konvention: **Fremdwährung pro 1 EUR** (z. B. 1,1720 USD/EUR → EUR = USD ÷ 1,1720). Keine externe Kursquelle (BMF/EZB) in dieser Ausbaustufe — als spätere Option vorgemerkt.

## Datenmodell
Unverändert gespeichert (Dokumentation des Originalbelegs):
- `Waehrung` (z. B. "USD"), `BetragNetto`/`SteuersatzBetrag`/`Bruttobetrag` (in Fremdwährung), `Wechselkurs`, `Gebuehr`, `Trinkgeld`, `Rabatt`.

Neu / zentral:
- **Eine zentrale EUR-Umrechnung** `core.RowEUR(row) (nettoEUR, vatEUR, bruttoEUR, gebuehrEUR, trinkgeldEUR, rabattEUR float64)`:
  - EUR-Beleg (`Waehrung` leer oder "EUR"): Beträge unverändert.
  - Fremdwährung mit `Wechselkurs > 0`: jeder Betrag = `round2(betrag / Wechselkurs)`.
  - Fremdwährung **ohne** Kurs: Rückgabe der Nennwerte + ein Flag/Fehler „Kurs fehlt" (löst eine Warnung aus, siehe unten).
- `BetragNetto_EUR` bleibt als **abgeleitetes** Feld (== nettoEUR) erhalten — Anzeige + Abwärtskompatibilität; einzige Wahrheit ist der Kurs.

## Buchung
- `BuildBooking` (bzw. `computeInvoiceBooking`) arbeitet **auf den EUR-Werten** aus `RowEUR`: Aufwands-/Anlage-Soll, Vorsteuer, §13b (1577/1787), Zahlung-Haben — alles in EUR. Die USt-Basis ist der **EUR-Nettobetrag** (so wie bei 2026-0013/0014 manuell korrigiert).
- Die gebuchte Zahlung (Haben Bankkonto) ist der EUR-Wert nach manuellem Kurs. (Eine eventuelle kleine Differenz zum tatsächlichen Bankabbuchungs-EUR ist eine **Kursdifferenz**; in dieser Ausbaustufe nicht automatisch gebucht — als spätere Option vorgemerkt, ggf. Konto „Erträge/Aufwand Kursdifferenzen" 2660/…).

## Auswertungen (alle auf EUR)
Folgende Stellen müssen `RowEUR` statt der Fremdwährungs-Nennwerte verwenden:
- **Summenzeile** der Belegtabelle (teilweise schon umgesetzt).
- **UStVA** (`ComputeUStVAOfficial`), **SuSa** (`ComputeSuSa`), **GuV** (`ComputeGuV`), **OPOS** (`ComputeOpenItems`), **Controlling** (`AggregateControlling`).
- **Belegabgleich**: `InvoiceEURAmount` (rechnet bereits brutto/kurs — vereinheitlichen auf `RowEUR`).
- **PDF-Reports** (Buchungsjournal, Belegliste, Rechnungsausgangsbuch, UStVA/ZM/OPOS/SuSa/GuV/Controlling): Beträge in EUR; Original-Fremdwährung optional als Zusatzspalte/Hinweis.
- **CSV-Export**: EUR-Spalten führend; Original-Fremdwährung + Kurs als Zusatzspalten (Dokumentation).

## UI
- **Tabelle:** Betragsspalten zeigen weiterhin die **Original-Währung mit Code** (z. B. „USD 200,00"), plus die bestehende Spalte **„Betrag netto EUR"**. Summen sind EUR (umgerechnet). *(Status: bereits umgesetzt.)*
- **Erfassungs-/Bearbeiten-Modal:** Bei Fremdwährung zeigt die „Währungsumrechnung"-Sektion den **EUR-Buchungsbetrag** prominent (z. B. „Gebucht in EUR: 170,65"), abgeleitet aus dem manuell eingegebenen Kurs. Die Buchungs-Vorschau ist in EUR.
- **Warnung (neu):** Fremdwährungsbeleg **ohne Wechselkurs** → Plausibilitätswarnung „Fremdwährung ohne Kurs — EUR-Umrechnung fehlt" (es gibt bereits „Fremdwährung ohne Wechselkurs"; sicherstellen, dass sie auch die Buchung blockt/markiert).

## Edge Cases / Regeln
- **Rundung:** Jede EUR-Umrechnung `round2`. USt-Basis = EUR-Netto; USt = `round2(EUR-Netto × Satz)`. Buchung muss ausgeglichen bleiben (Σ Soll = Σ Haben), ggf. Rundungsausgleich auf der Zahlungszeile.
- **§13b:** EUR-Netto × 19 % → 1577/1787 (wie bereits manuell umgesetzt).
- **Rabatt/Gebühr/Trinkgeld:** ebenfalls über `RowEUR` in EUR.
- **EUR-Belege:** Verhalten unverändert (keine Umrechnung).

## Migration / Bestand
- Bestehende Fremdwährungsbelege: einmaliger Lauf, der die gespeicherte `buchung` auf EUR (via `RowEUR`) neu erzeugt — oder beim nächsten Öffnen/Speichern automatisch. Belege ohne Kurs werden markiert (Warnung), nicht stillschweigend umgerechnet.
- Reine EUR-Bestände bleiben unberührt.

## Nicht in dieser Ausbaustufe (spätere Optionen)
- Automatischer Kursabruf (BMF-Monatskurse §16 Abs. 6 UStG / EZB).
- Automatische Buchung von **Kursdifferenzen** (Bankabbuchungs-EUR vs. manueller Kurs).
- Mehrere Kurse pro Beleg (USt-Basis-Kurs vs. Zahlungskurs).

## Entscheidungen (bestätigt)
1. **PDF/CSV:** Original-Fremdwährung wird als **Zusatzspalte** mitgeführt (Dokumentation). ✅
2. **Migration:** **Einmaliger aktiver Neu-Buchungslauf** über bestehende Fremdwährungsbelege (mit Backup; kurslose Belege werden markiert, nicht stillschweigend umgerechnet). ✅

## Self-Review
Fokussiert auf ein Subsystem (Fremdwährung→EUR). Zentrale Umrechnung `RowEUR` als single source; Buchung + alle Auswertungen konsumieren sie. Manuelle Kursquelle (bestätigt). Bestehende EUR-Logik unberührt. Kursabruf/Kursdifferenz/Multi-Rate bewusst out-of-scope.
