# E19 — UX/UI-Politur (Design / Spec)

**Ziel:** Zehn additive UX/UI-Verbesserungen für BuchISY (Go 1.25 + Fyne), gebündelt in 5 für sich lauffähige Phasen. Keine Funktion wird entfernt — nur Lesbarkeit, Tempo, Orientierung, Sicherheit und Ergonomie verbessert.

## Globale Prinzipien
- Bestehenden Mustern folgen: Fyne-Widgets, Settings-Persistenz (`core.Settings` + `SettingsManager`), i18n über `a.bundle.T(...)` mit Keys in **beiden** `assets/i18n/{de,en}.json`.
- istoks Bestätigungs-Prinzip: destruktive/automatische Aktionen bleiben explizit bestätigt — kein stilles Automatik.
- Alles muss mit dem UI-Zoom (E19.1) und in der dichten Tabelle lesbar bleiben.
- Geschäftslogik (Such-Query, Aggregation, Undo-Puffer, Skalierungs-Theme) wird unit-getestet; reines Fyne-UI manuell.

---

## E19.1 — Darstellung & Einstieg

### Fluides UI-/Schrift-Zoom (#6)
- Neuer `Settings.UIScale float64` (default 1.0; geklemmt 0.6–2.5).
- Ein **Custom-`fyne.Theme`-Wrapper** um das aktuelle Theme, dessen `Size(name)` den Basiswert × `UIScale` zurückgibt (Farben/Icons unverändert). Live umschalten via `a.app.Settings().SetTheme(scaledTheme)` — kein Neustart.
- Steuerung global: **Strg+Mausrad** (Scroll-Event mit Strg-Modifier), **Strg++/Strg+−** (Schrittweite ~0,1), **Strg+0** = zurück auf 1.0.
- Feedback: kurze, selbst-ausblendende Overlay-Einblendung „125 %".
- `UIScale` wird wie die übrigen Settings persistiert und beim Start angewendet.
- **Kein Dark Mode** (bewusst gestrichen).

### Aussagekräftige Leerzustände + Onboarding (#5)
- **Leerer Monat:** statt leerer Tabelle ein zentrierter Hinweis + Primäraktion: „Noch keine Belege — PDFs hierher ziehen oder **+ Beleg hinzufügen**".
- **Fehlende Konfiguration:** dezente, ausblendbare Hinweis-Banner, wenn API-Key/Speicherpfad/Konten fehlen, mit Link in die Einstellungen.
- Hinweise einmalig/ausblendbar (`Settings`-Flag), nicht aufdringlich.

---

## E19.2 — Lesbarkeit

### Einheitliche Status-Sprache + Legende (#2)
- Eine schmale **Status-Spalte** mit **farbcodierten Icons** statt Wort-Mix; konsistentes Mapping aus dem Zeilenzustand: gebucht · abgeglichen · Warnung (aus `InvoiceWarnings`) · offen · Barkasse gedeckt/nicht gedeckt/bestätigt.
- **Hover-Tooltip** je Icon mit Klartext (z. B. „Abgeglichen — Auszug S.2 Z.7", „Bar nicht gedeckt — Einlage fehlt").
- Kleine **Legende** (ein „?"-Knopf/Popover), die alle Symbole + Farben erklärt.
- Farben so wählen, dass sie mit dem Zoom und auf üblichen Fyne-Hintergründen lesbar sind.

### Konto-Namen überall + angereicherte Tooltips (#12)
- Wo eine **nackte Kontonummer** erscheint (Tabelle, Export-Vorschau, Buchungsvorschau), den **Namen** mitzeigen oder per Tooltip einblenden.
- **Angereicherte Tooltips** mit mehr Daten: Konto → Nr + Name + (Typ/Kategorie, ggf. Nutzungszähler); Beleg-Zeile → Hover zeigt zusätzliche Felder (Verwendungszweck, USt-IdNr, Bezahldatum, Buchung kurz). Allgemein: Tooltips als Informations-Layer, nicht nur Namens-Auflösung.

---

## E19.3 — Navigation & Suche

### Schnellsuche über alle Belege (#3, Variante c)
- Immer sichtbares **Suchfeld** oben.
- **Tippen** filtert den **aktuellen Monat** live (clientseitig über die geladenen Zeilen).
- **Enter** löst eine **globale Suche** über alle Perioden aus: neue Repo-Methode `SearchInvoices(query) []CSVRow` (`SELECT ... WHERE LOWER(auftraggeber|verwendungszweck|rechnungsnummer|belegnummer) LIKE ? OR bruttobetrag ~ query`), Ergebnis als kompaktes Overlay; Klick **lädt den Monat** des Treffers und **markiert** die Zeile.

### Zeitnavigation + Übersichts-Panel (#10)
- **◀ ▶**-Schaltflächen neben der Monatsanzeige + Tastatur (Strg+←/→), plus „aktueller Monat".
- Kompaktes **Übersichts-Panel** (nutzt die vorhandene `jahresuebersicht`-Aggregation, ergänzt um Kennzahlen): pro Monat/Jahr Summe Netto/USt/Brutto, **Zahllast**, **# offene Abgleiche**, **# nicht gedeckte Barkasse**, **# Warnungen**. Klick auf eine Kennzahl springt gefiltert dorthin.

### Tastatur-Navigation in der Tabelle (#11)
- **↑/↓** Zeile wählen, **Enter** = bearbeiten/Vorschau, **Entf** = löschen (mit Rückfrage). Fokus-/Auswahl-Zustand sichtbar.

---

## E19.4 — Feedback & Sicherheit

### Einheitliche Toasts + „Rückgängig" (#9)
- Konsistente, nicht-blockierende **Toasts** für jede abgeschlossene Aktion (Speichern/Löschen/Export/Umbenennen/Belegnummern-Neuvergabe) über einen zentralen `a.showToast`-Pfad.
- **Zeitlich begrenztes „Rückgängig"** im Toast für destruktive Aktionen. **Start-Umfang: Löschen** — über eine kurze Wiederherstell-Puffer-Zone (Soft-Delete: gelöschte Zeile + Datei-Pfad gepuffert, echtes Entfernen erst nach Ablauf/Verdrängung). Später erweiterbar auf Umbenennen/Neuvergabe.

### „Alle ★ bestätigen" im Abgleich (#1)
- Button **„Alle eindeutigen (★) bestätigen"** im Beleg- **und** Erlös-Abgleich: verbucht in einem Klick alle `highConfidence`-Vorschläge (claim-once bleibt; Konflikte werden übersprungen), mit einer Sammel-Rückfrage. Mehrdeutige Vorschläge bleiben bewusst zur Einzelprüfung stehen. **Zusatzoption** neben dem Einzel-Bestätigen — kein Ersatz.

---

## E19.5 — Erfassungs-Ergonomie

### Prüf-Modal: Layout, Dichte, Tab-Reihenfolge (#8)
Sektionen mit dezenten Überschriften; gebündelte Info-Zeile oben; kompaktere Dichte; saubere Tab-Reihenfolge (Firma → Datum → Betrag → Konto → Speichern). Vorschlag (ASCII-Mockup, finanzielle Felder schematisch):

```
┌─ Rechnungsdaten prüfen ───────────────────────────────────────────────┐
│ Quelle: E-Rechnung   ⚠ 0% ohne USt-IdNr   ⚠ Mögliche Dublette 2026-0007│  ← eine ruhige Info-Zeile
├──────────────────────────── Vorschau (PDF) ───── │  Beleg-Nr. 2026-0008 │
│  ▸ Identifikation                                │  Neuer Name: …        │
│     Firma [______]  Datum [__.__.____]  Rechnungsnr [____]               │
│  ▸ Beträge & Steuer                                                      │
│     Netto […]  Satz […]  USt […]  Brutto […]   (Währung [EUR ▾])        │
│  ▸ Buchung & Konto                                                       │
│     Konto [4663 — Reisekosten ▾]   ☐ Ausgangsrechnung                    │
│     Buchungsvorschau: Soll 4663 55,24 / Soll 1576 10,50 / Haben 1200 …   │
│  ▸ Zahlung & Anhang                                                      │
│     Bezahlt am [__.__.____]   Anhänge: keine [+]                         │
├─────────────────────────────────────────────────────────────────────────┤
│                                            [Abbrechen]  [Speichern ⌘S]   │
└─────────────────────────────────────────────────────────────────────────┘
```
Reines Layout-Refactoring, kein neues Verhalten; größtes UI-File → sorgfältig + review-pflichtig. (Verbindliches Pixel-Layout wird im Plan finalisiert; ggf. visuelles Companion-Mockup.)

### Schnellerer Konto-Picker (#4)
- **Zuletzt benutzt** (5–8, pro Profil persistiert) ganz oben.
- **Fuzzy-Suche** gleichzeitig über **Nummer und Name**.
- **Favoriten/Anheften** (★) für Stammkonten.
- **Tastatur:** tippen → Pfeil → Enter.

---

## Sequencing
E19.1 (Zoom fundamental, früh testen) → E19.2 (Lesbarkeit) → E19.3 (Navigation/Suche) → E19.4 (Feedback/Sicherheit) → E19.5 (Modal — größtes/heikelstes, zuletzt). Jede Phase: Plan → subagent-getrieben (TDD für Core) → Review → merge, wie E15–E18.

## Out of Scope (bewusst)
Dark Mode (#6 gestrichen), Bulk-/Mehrfachauswahl (#7 abgelehnt), Inline-Tabellen-Bearbeitung (#13 vorerst zurückgestellt).
