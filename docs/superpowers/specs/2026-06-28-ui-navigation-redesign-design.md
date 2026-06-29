# UI/UX-Redesign: Workflow-Navigation, Menüleiste & Bedienkomfort

**Datum:** 2026-06-28
**Status:** Entwurf zur Freigabe
**Betrifft:** `internal/ui/` (App-Shell, Navigation, Theme)

## Kontext

Eine Anwenderin, die täglich mit BuchISY arbeitet, hat konkretes Feedback gegeben:

> „UI / UX ist sehr unschön. Ich weiß wo was ist, aber ein Neuling findet das nie, weil
> teilweise versteckt. Es gibt SuSa, Offene Posten Listen und so weiter, aber alles ist
> versteckt (rechts oben bei den ‚drei Punkten‘ … vor Einstellungen). ‚Kontoauszüge‘ und
> ‚Belege‘ sind aktuell visuell sehr nah und vom UI schwer zu differenzieren.“
>
> „Markieren / greifen von Text geht auch fast nicht, obwohl immer wieder [gewünscht].“

Eine Code-Untersuchung bestätigt die Diagnose vollständig:

1. **Alles Wichtige ist hinter einem unbeschrifteten ⋮-Icon versteckt.** Der „drei Punkte"-
   Überlauf-Button (`app.go:917`, `ShowPopUpMenuAtPosition` `app.go:951`) enthält **25
   Einträge** — SuSa, GuV, OPOS, UStVA, ZM, Controlling, Kassenbuch, Anlagen, Belegabgleich,
   Erlös-Abgleich, Audit, sämtliche Exporte, Periodensperre — ungruppiert und durchmischt.
   Ein Neuling hat keine Chance, SuSa oder OPOS zu finden.
2. **Es gibt keine persistente Navigation.** Kein Menüband, keine Seitenleiste, keine Tabs
   (`SetMainMenu`/`AppTabs` existieren nirgends auf App-Ebene). Ziele öffnen zudem auf drei
   inkonsistente Arten (Dialog / neues Fenster / Vollfenster-Tausch).
3. **Belege vs. Konten sind kaum unterscheidbar.** Zwei gleich aussehende Icon-Buttons
   nebeneinander (`viewToggleButtons`, `kontenview.go:241`), nur durch einen subtilen
   Blau/Grün-Akzent und „Importance" getrennt.
4. **Text markieren/kopieren „geht fast nicht".** Kopieren funktioniert technisch in der
   Haupttabelle und in Modals (`hoverLabel`, `copyableLabel`, `rightClickTable`), aber **nur
   per Rechtsklick** — kein `Strg+C`, keine Markierung mit der Maus, kein sichtbarer Hinweis.
   In tabellenbasierten Reports (OPOS, Controlling, Audit) und im Belegabgleich (17 einfache
   `widget.NewLabel`) ist Kopieren gar nicht möglich. Commit `2307c58` hat bereits viele
   freistehende Labels kopierbar gemacht, die Tabellenzellen aber nicht erfasst.

**Ziel:** Die App soll sich wie professionelle deutsche Buchhaltungssoftware (Lexware, DATEV,
sevDesk) anfühlen — entlang des **monatlichen Buchhaltungs-Workflows** organisiert, sodass die
App den Ablauf selbst vermittelt und ein Neuling alles findet.

## Leitprinzip: Organisation nach Workflow, nicht nach Dokumenttyp

Die Marktführer organisieren **nicht** nach „Berichte / Aktionen / Sonstiges", sondern nach dem,
was die Anwenderin gerade **erledigen** will. Die Seitenleiste bildet das monatliche Ritual von
oben nach unten ab. Es gibt **keinen „Weitere/Sonstiges"-Sammeltopf** — jeder Eintrag bekommt
einen Platz in einer Workflow-Phase.

## Architektur (Round 1)

### 1. Workflow-Seitenleiste (linke Spalte, persistent)

Ersetzt die zwei Toggle-Buttons und entlädt das ⋮-Menü. Enthält **nur navigierbare Bildschirme**,
gruppiert nach Phase. Gruppenüberschriften sind nicht klickbar (immer ausgeklappt → maximale
Auffindbarkeit).

```
ERFASSEN
  📄 Belege
  🧾 Kassenbuch
BUCHEN
  🏦 Konten (Bank)
  🔗 Belegabgleich
  🔗 Erlös-Abgleich
  🏗 Anlagen
AUSWERTEN
  SuSa
  GuV
  Offene Posten (OPOS)
  Controlling
  Übersicht (Jahr)
FINANZAMT
  UStVA
  ZM
ABSCHLUSS / GoBD
  Zeitraum sperren
  Änderungsprotokoll
  Verfahrensdokumentation
  DATEV/GoBD-Export
```

**Phase-1-Verdrahtung (geringes Risiko):** Jeder Eintrag ruft die **bestehende** Funktion auf
(`a.showSuSa()`, `a.showOpenItems()`, `a.showCashBookView()`, …) — keine View-Dateien werden
umgeschrieben. Belege/Konten setzen weiterhin `a.viewMode` und rufen `buildUI()`. Reports öffnen
sich vorerst weiter als Dialoge; das Einbetten in den Inhaltsbereich ist eine spätere Phase
(siehe „Nicht in diesem Umfang").

**Aktiver Zustand:** Der aktuelle Bildschirm wird in der Leiste hervorgehoben (HighImportance /
Akzentfarbe). Die bestehende Akzentlogik (`accentBelege`/`accentKonten`, `applyAccentForMode`
`app.go:847`) wird auf die Seitenleisten-Phasen erweitert (Belege blau, Konten grün bleibt).

### 2. Native Menüleiste (`SetMainMenu`)

Für **einmalige Aktionen** (keine Navigationsziele). Vertraut für Windows-Anwender, kostet keinen
Inhaltsplatz, wirkt nativ.

```
Datei      Mehrere Belege importieren…
           Ziel-Ordner öffnen
           ─
           Backup erstellen
           ─
           Beenden

Bearbeiten Belegnummern neu vergeben
           Auto-Regeln…

Export     CSV-Export
           Buchungen exportieren
           Belegliste (PDF)
           Rechnungsausgangsbuch (PDF)
           ─
           DATEV/GoBD-Paket
           Verfahrensdokumentation (PDF)

Ansicht    Vergrößern / Verkleinern / Zurücksetzen   (bestehende Zoom-Shortcuts)
           ─
           Voriger Monat / Nächster Monat            (Strg+← / Strg+→)

Hilfe      Legende (Kennzahlen)
           Über BuchISY
```

> Hinweis: Mehrere Aktionen erscheinen **sowohl** in der Seitenleiste (als Workflow-Schritt) als
> auch im Menü (als Aktion) — z. B. „DATEV/GoBD-Export" und „Zeitraum sperren". Das ist
> beabsichtigt: in der ABSCHLUSS-Phase als Schritt sichtbar, im Menü für den schnellen Zugriff.

Das ⋮-Überlaufmenü und die zwei Toggle-Buttons **entfallen**. Das Zahnrad (Einstellungen) bleibt
oben rechts erhalten (zusätzlich evtl. unter „Datei").

> Fast alle Menüeinträge verdrahten bestehende Funktionen. **Neu** sind nur zwei triviale
> Standard-Einträge: „Datei → Beenden" (`a.app.Quit()`) und „Hilfe → Über BuchISY" (kleiner
> Info-Dialog mit Version/Bergx2). „Legende" nutzt den bestehenden `LegendButton`/„?".

### 3. Prominenter Buchungszeitraum

Der aktive Monat/Jahr ist die wichtigste Kontextinformation der Buchhaltung. Statt Auswahl oben
rechts + Periode in der Fußzeile wird der Zeitraum **groß und dauerhaft sichtbar** am Kopf des
Inhaltsbereichs platziert: `◀  Juni 2026  ▶` plus Jahr- und „Ganzes Jahr"-Auswahl. Die
bestehenden `yearSelect`/`monthSelect` (`app.go:868`) und Pfeil-/Tastatur-Navigation
(`Strg+←/→`, `app.go:389`) werden wiederverwendet, nur neu platziert und vergrößert. Die
Sperr-Anzeige (Festschreibung) erscheint direkt neben dem Zeitraum.

### 4. Bedienkomfort: Text markieren & kopieren

- **`Strg+C`-Shortcut:** kopiert die fokussierte Tabellenzelle bzw. die ausgewählte Zeile in die
  Zwischenablage. Registrierung analog zu den bestehenden `desktop.CustomShortcut`-Einträgen
  (`app.go:366–394`). In der Haupttabelle nutzt es die vorhandenen `getCellValue`/`joinRowValues`
  (`table.go:702–714`).
- **Restliche Zellen kopierbar machen:** Verbleibende einfache `widget.NewLabel` in
  tabellen-/zeilenbasierten Views konvertieren — `oposview.go`, `controllingview.go`,
  `auditview.go` (jeweils `widget.NewTable`-Zelltemplate auf kopierbare Zelle umstellen) und
  `belegabgleichview.go`/`erloesabgleichview.go` (Drop-in `newCopyableLabel`). Die Helfer
  existieren bereits (`copyablelabel.go`, `hoverLabel`).
- **Sichtbarer Hinweis:** dezenter Tooltip/Hinweis „Rechtsklick zum Kopieren · Strg+C" an
  Tabellen, damit das Feature auffindbar ist (löst das „geht fast nicht"-Gefühl).

### 5. Leichter Theme-Pass (professionelleres Aussehen)

Konzentriert in `internal/ui/theme.go` (`buchisyTheme`), keine Änderung an den ~60 Screen-Dateien.
Live-Refresh ist bereits verdrahtet (`SetTheme`, `app.go:600`).

- **Schriftart:** **Inter** als UI-Schrift einbinden (`fyne bundle` → `Font()`-Methode in
  `buchisyTheme`). Tabellenziffern (Beträge) mit Tabular-Figures/rechtsbündig. Größter Einzeleffekt
  gegen den „generischen Fyne"-Look.
- **Vollständige Farb-Token:** über `ColorNamePrimary` hinaus auch `Background`, `Foreground`,
  `InputBackground`, `Separator`, `Hover`, `Disabled`, `Shadow` für eine stimmige Palette in Hell
  und Dunkel definieren (erweitert die vorhandene `Color()`-Override).
- **Abstände/Größen:** `SizeNamePadding`, `SizeNameInnerPadding`, `SizeNameText`,
  `SizeNameInputBorder`, `SizeNameSeparatorThickness` bewusst setzen (erweitert die vorhandene
  `Size()`-Override). Wirkt sofort „designt" statt „default".

## Komponenten & Schnittstellen

| Einheit | Zweck | Abhängigkeiten |
|---|---|---|
| `sidebar.go` (neu) | Baut die Workflow-Seitenleiste; mappt Einträge → bestehende `a.show…`-Funktionen; verwaltet aktiven Zustand | `app.go` (Funktionen), `theme.go` (Akzent) |
| `mainmenu.go` (neu) | Baut `fyne.MainMenu`; mappt Aktionen → bestehende Funktionen | `app.go` |
| `app.go` (Shell) | Neues Shell-Layout: `Border(menubar implizit, period-header, sidebar links, content)`; entfernt ⋮-Button + Toggle-Buttons aus `buildTopBar` | sidebar, mainmenu |
| `theme.go` | Schrift, Farb-Token, Größen erweitern | `assets` (Inter-TTF, gebündelt) |
| Copy-Erweiterungen | `Strg+C`-Shortcut in `app.go`; Zell-Templates in oposview/controllingview/auditview; `newCopyableLabel` in den Abgleich-Views | `table.go`, `copyablelabel.go` |

**Shell-Layout (neu):** `MainMenu` via `window.SetMainMenu(...)` + Inhalt
`container.NewBorder(periodHeader, statusBar, sidebar, nil, content)`. `buildUI()` (`app.go:661`)
liefert weiterhin den Inhalt für den aktiven Bildschirm; die Seitenleiste ruft `SetContent` bzw.
die bestehenden Dialog-Funktionen.

## Nicht in diesem Umfang (spätere Runden)

- **Einbetten der Views** in den Inhaltsbereich (echte Ein-Fenster-Navigation ohne Dialoge) —
  bewusst Phase 2; betrifft ~12 View-Dateien.
- **Vollständiger Theme-Relaunch** (Iconographie-Set, individuelle Tabellen-Gestaltung,
  Hover-Zeilen) — eigener Folge-Pass.
- **Dashboard/Startseite.**

## Fehlerbehandlung & Kompatibilität

- Seitenleisten-/Menüeinträge rufen exakt die bestehenden Funktionen → kein neues Verhalten, kein
  neuer Fehlerpfad. Periodensperre, Mandanten-Isolation, Audit bleiben unangetastet.
- Inter-Schrift wird eingebettet (wie i18n) → keine externe Abhängigkeit, kein Packaging-Risiko.
- Fenstergröße/Persistenz unverändert (Default 1500×875). Seitenleiste fester Breite (~190px).

## Test / Verifikation

1. **Auffindbarkeit:** Jeder der bisher im ⋮-Menü versteckten 25 Punkte ist über Seitenleiste oder
   Menüleiste in ≤1 Klick erreichbar und korrekt beschriftet (DE/EN via i18n).
2. **Belege vs. Konten:** stehen in getrennten Phasen (ERFASSEN/BUCHEN), klar unterscheidbar.
3. **Funktionsgleichheit:** jeder Eintrag öffnet denselben Bildschirm/dieselbe Aktion wie zuvor
   (Regressionstest gegen die Tabelle in `app.go:919–952`).
4. **Kopieren:** `Strg+C` kopiert fokussierte Zelle/Zeile; OPOS/Controlling/Audit-Zellen und
   Belegabgleich-Zeilen per Rechtsklick kopierbar; Hinweis sichtbar.
5. **Theme:** App startet mit Inter-Schrift; Hell/Dunkel konsistent; Zoom (`Strg ±`) funktioniert
   weiter.
6. **Build/Run:** `make dev` (bzw. `go build`) baut; App startet auf macOS nativ; Windows-Build
   unverändert.
7. `go test ./...` grün.
```
