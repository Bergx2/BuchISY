# E20 — Vollwertiges Buchhaltungs-Vorsystem (Design / Spec)

**Ziel:** BuchISY von „Beleg-Erfassung + USt" zu einem **GoBD-konformen Vorsystem** (mittelfristig Vollbuchhaltung) ausbauen — für einen Betrieb mit ~200 Belegen/Monat. Neun Bausteine, jeder als eigene, lauffähige Phase.

**Rolle (entschieden):** primär **Vorsystem** — sauber erfassen, revisionssicher festschreiben, USt, und einen sauberen DATEV-/Beleg-Stapel an den StB übergeben. SuSa/GuV als Auswertung (nicht voller Jahresabschluss). Vollbuchhaltung (Bilanz) bleibt mittelfristig.

## Globale Prinzipien
- Go 1.25 + Fyne; `internal/core` (testbare Logik, TDD), `internal/db` (Schema/Migration/Repo), `internal/ui`.
- Bestehende Muster: idempotente `ALTER TABLE`-Migrationen; Repo CRUD; i18n in beiden JSONs; SKR03 (Bergx2).
- **Default-Entscheidungen werden dokumentiert; echte Gabelungen → „Offene Entscheidungen" am Ende der Session, nicht blockierend.**
- Jede Phase: Plan → subagent-getrieben (TDD) → Review → merge → exe.

## Reihenfolge (Fundament zuerst)
E20.1 Audit-Trail → E20.2 Periodenabschluss/Festschreibung → E20.3 OPOS → E20.4 SuSa/GuV → E20.5 Regel-Engine/Auto-Buchung → E20.6 CAMT/MT940-Import → E20.7 Anlagen/AfA → E20.8 DATEV-Belegpaket/GoBD-Export → E20.9 Verfahrensdokumentation.

---

## E20.1 — Änderungsprotokoll (Audit-Trail) [#1a]
**Warum:** GoBD-Nachvollziehbarkeit — jede Änderung muss protokolliert sein.
**Datenmodell:** neue Tabelle `audit_log(id INTEGER PK, ts DATETIME, aktion TEXT[create|update|delete|lock|unlock|storno], entitaet TEXT, schluessel TEXT, details TEXT/JSON)`. 
**Logik:** `core.AuditEntry` + `db.Repository.LogAudit(entry)`; `Insert`/`Update`/`Delete` schreiben automatisch einen Eintrag (Update: geänderte Felder alt→neu als JSON). `AuditLog(filter) []AuditEntry`.
**UI:** Menü „Änderungsprotokoll" → Dialog/Tabelle (Zeit · Aktion · Beleg · Änderung), filterbar.
**Default:** Protokoll ist append-only, nie editierbar/löschbar aus der UI.

## E20.2 — Periodenabschluss / Festschreibung [#1b/#2]
**Warum:** GoBD-Unveränderbarkeit nach Abschluss.
**Datenmodell:** Tabelle `period_locks(jahr TEXT, monat TEXT, locked_at DATETIME, PRIMARY KEY(jahr,monat))`.
**Logik:** `Repository.LockPeriod/UnlockPeriod/IsPeriodLocked(jahr,monat)`. `Insert/Update/Delete` auf eine gesperrte Periode → Fehler `ErrPeriodLocked` (Ausnahme: explizite **Storno**-Buchung, die einen Gegeneintrag anlegt statt zu ändern). Lock/Unlock schreiben ins Audit-Log.
**UI:** „Monat abschließen" / „Monat öffnen" (mit Bestätigung + Warnhinweis); gesperrte Belege in der Tabelle als 🔒 markiert; Edit/Delete für gesperrte Belege deaktiviert mit Hinweis „Periode festgeschrieben — Storno nutzen".
**Default:** Unlock ist erlaubt (mit Audit-Eintrag), nicht hart verboten — Vorsystem-Pragmatik. *(Offene Entscheidung: ob Unlock später per Passwort/Recht eingeschränkt wird.)*

## E20.3 — Offene-Posten-Liste (OPOS) [#3]
**Warum:** Forderungen (Soll-Besteuerung 1400) + offene Verbindlichkeiten sichtbar machen.
**Logik (core, testbar):** `OpenItems(rows) struct{Forderungen, Verbindlichkeiten []OpenItem}` mit Aging-Buckets (0–30/31–60/61–90/>90 Tage ab Rechnungsdatum). Forderung = `Ausgangsrechnung && BuchungRef==""` (noch nicht als bezahlt verbucht). Verbindlichkeit = `!Ausgangsrechnung && Bankkonto!=cash && BuchungRef=="" && Bezahldatum==""`.
**UI:** Menü „Offene Posten" → zwei Tabellen (Debitoren/Kreditoren) mit Summe je Bucket; Klick springt zum Beleg; PDF-Export (`BuildOpenItemsPDF`).

## E20.4 — Summen-/Saldenliste (SuSa) + GuV/BWA [#8]
**Warum:** Grundauswertung der doppelten Buchführung.
**Logik (core, testbar):** `ComputeSuSa(rows, fromY,fromM,toY,toM) []AccountBalance{Konto,Name,SollSumme,HabenSumme,Saldo}` aus allen `Booking.Entries`. `GuV(susa) struct{Erloese,Aufwand,Ergebnis}` (Erlöskonten 8xxx − Aufwandskonten 4xxx via Chart-Typ). 
**UI:** Menü „Summen- & Saldenliste" (Monat/Jahr) + „GuV/BWA"; je PDF-Export (`BuildSuSaPDF`, `BuildGuVPDF`). Klassen-/Kontotyp aus dem Chart (`SKRAccount.Type`).

## E20.5 — Regel-Engine / Auto-Buchung [#6]
**Warum:** 200/Monat ohne jedes Mal Modal.
**Datenmodell:** erweitert `booking_templates`/companymap → `auto_rules.json`: pro Lieferant/Stichwort `{konto, kategorie, steuersatz, autobook bool}`.
**Logik:** beim Import: wenn eine Regel mit `autobook` matcht UND der Beleg plausibel (Brutto=Netto+USt, Konto gesetzt, keine Dublette/Warnung) → **direkt buchen ohne Modal**, sonst Modal wie bisher (mit vorbelegten Werten). Sammel-Ergebnis-Toast „N automatisch gebucht, M zur Prüfung".
**Default:** Autobook ist **opt-in pro Regel** (Schalter), Default aus — Sicherheit vor Tempo.

## E20.6 — Bank-Import CAMT.053 / MT940 [#5]
**Warum:** verlässlicher als PDF-Auszug-Parsing; Standard.
**Logik (core, testbar):** `ParseCAMT053(xml) []StatementBooking` (ISO 20022, `encoding/xml`) und `ParseMT940(text) []StatementBooking` (zeilenbasiert :61:/:86:). Erkennung am Dateiinhalt/Endung; in `fileStatement` neben PDF einklinken. **„Fehlende Belege"**: nach Import Bankzeilen ohne verknüpften Beleg auflisten.
**Default:** keine neue externe Lib — beide Formate in-house parsen (CAMT via encoding/xml, MT940 via Zeilen-Parser).

## E20.7 — Anlagenbuchhaltung (AfA/GWG) [#7]
**Warum:** Abschreibungen gehören zu fast jedem Betrieb.
**Datenmodell:** Tabelle `assets(id, bezeichnung, anschaffungsdatum, anschaffungswert, nutzungsdauer_jahre, methode TEXT[linear], konto INTEGER, afa_konto INTEGER)`.
**Logik (core, testbar):** `LinearAfA(asset, jahr) float64` (zeitanteilig im Anschaffungsjahr, Restbuchwert); GWG-Hinweis bei Netto ≤ 800 € (Sofortabschreibung). `Anlagenspiegel(assets, jahr)`.
**UI:** „Anlagen" (Liste + anlegen/bearbeiten) + „AfA-Buchungen erzeugen" (Soll AfA-Aufwand / Haben Anlage) je Jahr; PDF Anlagenspiegel.
**Default:** lineare AfA, Nutzungsdauer vom Nutzer pro Anlage (Vorschlag aus simpler Tabelle). *(Offene Entscheidung: degressive AfA, AfA-Tabellen-Automatik.)*

## E20.8 — DATEV-Belegpaket / GoBD-Datenexport [#9]
**Warum:** saubere StB-Übergabe + Betriebsprüfung.
**Logik:** `BuildExportPackage(rows, period) zip` = DATEV-EXTF-Buchungsstapel (vorhanden) + **verknüpfte Belegbilder** (PDFs, benannt nach Belegnummer) + `manifest.csv` (Beleg↔Buchung). Zusätzlich **GoBD-Datenexport**: `index.xml` (GDPdU/Z3-Beschreibung) + CSV-Tabellen (Buchungen, Belege, Konten).
**Default:** ZIP-Paket; DATEV-EXTF unverändert, nur gebündelt + Belege beigelegt.

## E20.9 — Verfahrensdokumentation [#4]
**Warum:** GoBD-Pflichtdokument.
**Logik:** `BuildVerfahrensdokumentation(settings, chart, rules) []byte` (Markdown→PDF) — beschreibt Erfassung (KI/lokal), Ablage/Speicherpfade, Belegnummernkreis, Festschreibung, Aufbewahrung (10 J.), Backup, Kontenrahmen, Regeln. 
**UI:** Menü „Verfahrensdokumentation (PDF)".

## Testing
Alle Kern-Logiken (Audit-Diff, Lock-Guard, OpenItems-Aging, SuSa/GuV-Summen, AfA-Berechnung, CAMT/MT940-Parser) unit-getestet; UI manuell.

## Out of Scope (mittelfristig / offene Entscheidungen — am Ende gesammelt)
Volle Bilanz/Jahresabschluss (HGB), degressive AfA-Automatik, Unlock-Rechteverwaltung/Passwort, Lohnbuchhaltung, echte DATEV-Online-Schnittstelle (API).
