# Kalender im „Datum wählen"-Fenster — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Das „Datum wählen"-Fenster zeigt einen anklickbaren Monatskalender statt drei Auswahllisten; ein Klick auf einen Tag wählt das Datum und schließt das Fenster.

**Architecture:** `showDatePicker` wird aus `invoicemodal.go` in eine neue Datei `datepicker.go` verlagert und als Monatskalender neu umgesetzt — mit Fyne-Bordmitteln (Raster aus Tag-Schaltflächen), keine zusätzliche Bibliothek. Signatur und Rückgabeformat (`TT.MM.JJJJ`) bleiben gleich.

**Tech Stack:** Go 1.25, Fyne v2.6.3.

**Hinweis zu Commits:** Git-Commits sind in diesem Plan bewusst ausgelassen. Jede Aufgabe endet mit `go build`/`go vet`/`go test`.

---

### Task 1: Monatskalender — neue Datei `datepicker.go`, alte `showDatePicker` entfernen

**Files:**
- Create: `internal/ui/datepicker.go`
- Modify: `internal/ui/invoicemodal.go`

- [ ] **Step 1: Remove the old `showDatePicker` from `invoicemodal.go`**

In `internal/ui/invoicemodal.go` gibt es die Funktion `showDatePicker` — sie beginnt mit der Kommentarzeile `// showDatePicker shows a date picker dialog on the given parent window.`, gefolgt von `func (a *App) showDatePicker(parent fyne.Window, initialDate string, onSelect func(string)) {`, und endet an der zugehörigen schließenden Klammer `}` (die Funktion baut die Tag/Monat/Jahr-`widget.NewSelect`-Listen, einen `dialog.NewCustomConfirm` und ruft `onSelect` mit einem `DD.MM.YYYY`-String auf).

Lösche die **gesamte** Funktion (Kommentarzeile + Funktionsrumpf) aus `invoicemodal.go`. Die neue Implementierung kommt in Step 2 in eine eigene Datei.

- [ ] **Step 2: Create `internal/ui/datepicker.go`**

Create `internal/ui/datepicker.go`:

```go
package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// showDatePicker shows a month-calendar date picker. Clicking a day selects
// that date and closes the window; the month arrows navigate without
// selecting. The selected date is delivered to onSelect as "DD.MM.YYYY".
func (a *App) showDatePicker(parent fyne.Window, initialDate string, onSelect func(string)) {
	// Selected date: the initial date if valid, otherwise today.
	now := time.Now()
	selDay, selMonth, selYear := now.Day(), int(now.Month()), now.Year()
	if parts := strings.Split(initialDate, "."); len(parts) == 3 {
		d, e1 := strconv.Atoi(strings.TrimSpace(parts[0]))
		m, e2 := strconv.Atoi(strings.TrimSpace(parts[1]))
		y, e3 := strconv.Atoi(strings.TrimSpace(parts[2]))
		if e1 == nil && e2 == nil && e3 == nil &&
			d >= 1 && d <= 31 && m >= 1 && m <= 12 && y > 0 {
			selDay, selMonth, selYear = d, m, y
		}
	}

	win := a.app.NewWindow("Datum wählen")
	viewYear, viewMonth := selYear, selMonth

	var render func()
	render = func() {
		prevBtn := widget.NewButton("‹", func() {
			viewMonth--
			if viewMonth < 1 {
				viewMonth = 12
				viewYear--
			}
			render()
		})
		nextBtn := widget.NewButton("›", func() {
			viewMonth++
			if viewMonth > 12 {
				viewMonth = 1
				viewYear++
			}
			render()
		})
		title := widget.NewLabelWithStyle(
			fmt.Sprintf("%s %d", a.bundle.T(fmt.Sprintf("month.%02d", viewMonth)), viewYear),
			fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
		header := container.NewBorder(nil, nil, prevBtn, nextBtn, title)

		weekdays := container.NewGridWithColumns(7)
		for _, wd := range []string{"Mo", "Di", "Mi", "Do", "Fr", "Sa", "So"} {
			weekdays.Add(widget.NewLabelWithStyle(wd, fyne.TextAlignCenter, fyne.TextStyle{Bold: true}))
		}

		grid := container.NewGridWithColumns(7)
		first := time.Date(viewYear, time.Month(viewMonth), 1, 0, 0, 0, 0, time.Local)
		lead := (int(first.Weekday()) + 6) % 7 // leading blanks, Monday-first
		for i := 0; i < lead; i++ {
			grid.Add(widget.NewLabel(""))
		}
		daysInMonth := time.Date(viewYear, time.Month(viewMonth)+1, 0, 0, 0, 0, 0, time.Local).Day()
		for d := 1; d <= daysInMonth; d++ {
			day := d
			btn := widget.NewButton(strconv.Itoa(d), func() {
				onSelect(fmt.Sprintf("%02d.%02d.%04d", day, viewMonth, viewYear))
				win.Close()
			})
			if day == selDay && viewMonth == selMonth && viewYear == selYear {
				btn.Importance = widget.HighImportance // current date — clearly marked
			} else if day == now.Day() && viewMonth == int(now.Month()) && viewYear == now.Year() {
				btn.Importance = widget.SuccessImportance // today — subtly marked
			}
			grid.Add(btn)
		}

		cancelBtn := widget.NewButton(a.bundle.T("btn.cancel"), func() { win.Close() })

		win.SetContent(container.NewVBox(header, weekdays, grid, widget.NewSeparator(), cancelBtn))
	}
	render()

	win.Resize(fyne.NewSize(340, 380))
	win.CenterOnScreen()
	win.Show()
}
```

- [ ] **Step 3: Build and vet**

Run: `go build ./... && go vet ./...`
Expected: PASS. If the build reports an import as now-unused in `invoicemodal.go` (the removed `showDatePicker` used `strconv` and `time` — they may or may not be used elsewhere in that file), remove only the import(s) the compiler actually flags.

- [ ] **Step 4: Run tests**

Run: `go test ./...`
Expected: PASS.

---

### Task 2: Build, Paketierung, Auslieferung

**Files:** none (build/deploy only)

- [ ] **Step 1: Final build + vet + tests**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: all succeed.

- [ ] **Step 2: Package the Windows executable**

Run (from `C:\Users\istok\Desktop\Dev\BuchISY`):
`fyne package -os windows -name BuchISY -src ./cmd/buchisy`
Expected: `cmd/buchisy/BuchISY.exe` produced.

- [ ] **Step 3: Stop the running app**

Terminate any running `BuchISY` process (PowerShell `wmic process where "ProcessId=<id>" call terminate` per running PID).

- [ ] **Step 4: Deploy and restart**

Copy `cmd/buchisy/BuchISY.exe` over `C:\Users\istok\Desktop\BuchISY.exe`, then launch
`C:\Users\istok\Desktop\BuchISY.exe` with working directory `C:\Users\istok\Desktop`.

- [ ] **Step 5: Manual smoke test**

1. In „Rechnungsdaten prüfen" oder „Rechnung bearbeiten" auf einen 📅-Knopf neben einem Datumsfeld klicken → ein Monatskalender erscheint.
2. Der heutige Tag ist markiert; ist im Feld bereits ein Datum, startet der Kalender in dessen Monat mit markiertem Tag.
3. Mit ‹ › durch die Monate blättern (auch über Jahresgrenzen) — dabei wird nichts ausgewählt.
4. Klick auf einen Tag → das Datum steht im Feld (`TT.MM.JJJJ`), das Kalenderfenster schließt sich.
5. „Abbrechen" schließt ohne Auswahl.

---

## Self-Review

**Spec coverage:**
- Monatskalender (Kopf mit ‹ › + Monat/Jahr, 7-Spalten-Raster Mo–So) → Task 1 Step 2 (`header`, `weekdays`, `grid`).
- Auswahl per Klick, Fenster schließt sofort → der Tag-Button-Handler ruft `onSelect` + `win.Close()`.
- Pfeile blättern ohne Auswahl, auch über Jahresgrenzen → `prevBtn`/`nextBtn` mit Über-/Unterlauf auf das Jahr.
- Heutiger Tag dezent, aktuelles Datum deutlich markiert → `SuccessImportance` für heute, `HighImportance` für das Ausgangsdatum.
- Hand-gebaut, keine zusätzliche Bibliothek → nur `fyne`, `container`, `widget`.
- Neue Datei `datepicker.go`; `showDatePicker` aus `invoicemodal.go` entfernt → Task 1 Steps 1+2.
- Rückgabe `TT.MM.JJJJ`, Signatur unverändert → `onSelect(fmt.Sprintf("%02d.%02d.%04d", …))`; Signatur `(parent fyne.Window, initialDate string, onSelect func(string))` unverändert.
- Edge: leeres/ungültiges Ausgangsdatum → heutiger Monat → die Parsing-Bedingung fällt sonst auf `now` zurück.
- Edge: 28–31 Tage → `daysInMonth` über „Tag 0 des Folgemonats".

**Placeholder scan:** Keine TBD/TODO; `datepicker.go` ist vollständig angegeben. Step 1 beschreibt die zu löschende Funktion eindeutig (Name, Signatur, Inhalt).

**Type consistency:** `showDatePicker(parent fyne.Window, initialDate string, onSelect func(string))` behält exakt die bisherige Signatur — die Aufrufstellen (die 📅-Knöpfe in `invoicemodal.go`/`tableedit.go`) bleiben unverändert. `a.app` (`fyne.App`) und `a.bundle.T` sind bestehende `App`-Member; die i18n-Schlüssel `month.01`–`month.12` und `btn.cancel` existieren bereits. `parent` bleibt ungenutzt (Signatur-Erhalt) — in Go zulässig für Funktionsparameter.
