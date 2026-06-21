package ui

import (
	"math"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/bergx2/buchisy/internal/core"
)

// taxLineRow holds the widgets for a single VAT line:
// [Netto | Satz % | MwSt-Betrag | Brutto (read-only) | ✕].
type taxLineRow struct {
	netto     *widget.Entry
	satz      *widget.Entry
	mwst      *widget.Entry
	brutto    *widget.Label
	removeBtn *widget.Button
}

// taxLinesEditor is a reusable widget that lets the user edit a list of
// VAT lines (Netto / Satz % / MwSt-Betrag) plus a Trinkgeld field. Each line
// shows its own Brutto (Netto + MwSt) and the widget exposes the total Brutto
// as a read-only label that updates live.
type taxLinesEditor struct {
	app      *App
	onChange func()

	// seeding is true while rows are populated programmatically during
	// construction; auto-fill and onChange skip their work so stored values
	// are not overwritten.
	seeding bool

	rows        []taxLineRow
	trinkEntry  *widget.Entry
	bruttoLabel *widget.Label

	rowsBox   *fyne.Container // VBox holding the line rows
	container *fyne.Container // outer VBox (the full widget)
}

// newTaxLinesEditor creates a TaxLines editor pre-filled with lines and
// trinkgeld. If lines is empty, one empty row is provided. onChange is
// called (nil-safe) after every structural or value change.
func newTaxLinesEditor(a *App, lines []core.TaxLine, trinkgeld float64, onChange func()) *taxLinesEditor {
	e := &taxLinesEditor{
		app:      a,
		onChange: onChange,
	}

	sep := a.settings.DecimalSeparator
	e.rowsBox = container.NewVBox()

	// Trinkgeld entry
	e.trinkEntry = widget.NewEntry()
	e.trinkEntry.SetPlaceHolder("0" + sep + "00")
	if trinkgeld != 0 {
		e.trinkEntry.SetText(formatDecimal(trinkgeld, sep))
	}
	e.trinkEntry.OnChanged = func(string) {
		if e.seeding {
			return
		}
		e.refresh()
	}

	// Total Brutto label (read-only)
	e.bruttoLabel = widget.NewLabelWithStyle(
		formatDecimal(0, sep), fyne.TextAlignLeading, fyne.TextStyle{Bold: true},
	)

	// Add-line button
	addBtn := widget.NewButtonWithIcon("+ MwSt.", theme.ContentAddIcon(), func() {
		e.addLine()
	})
	addBtn.Importance = widget.LowImportance

	// Seed with provided lines (or one empty row).
	e.seeding = true
	if len(lines) == 0 {
		e.appendRow(core.TaxLine{})
	} else {
		for _, l := range lines {
			e.appendRow(l)
		}
	}
	e.seeding = false

	// Column header (above the rows).
	header := container.NewBorder(nil, nil, nil, rowRightSpacer(),
		container.NewGridWithColumns(4,
			columnHeader("Netto"), columnHeader("Satz %"),
			columnHeader("MwSt."), columnHeader("Brutto"),
		),
	)

	trinkRow := container.NewBorder(nil, nil,
		widget.NewLabelWithStyle("Trinkgeld", fyne.TextAlignLeading, fyne.TextStyle{}),
		nil, e.trinkEntry,
	)
	bruttoRow := container.NewBorder(nil, nil,
		widget.NewLabelWithStyle("Brutto", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		nil, e.bruttoLabel,
	)

	e.container = container.NewVBox(
		header,
		e.rowsBox,
		addBtn,
		widget.NewSeparator(),
		trinkRow,
		bruttoRow,
	)

	e.refresh()
	return e
}

// columnHeader makes a small muted column caption.
func columnHeader(s string) *widget.Label {
	return widget.NewLabelWithStyle(s, fyne.TextAlignLeading, fyne.TextStyle{Italic: true})
}

// rowRightSpacer reserves header space matching the row's ✕ button so the
// four header captions line up over the four entry columns.
func rowRightSpacer() fyne.CanvasObject {
	b := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {})
	b.Disable()
	b.Hidden = true
	return b
}

// appendRow adds a new taxLineRow for l to the editor (internal helper).
func (e *taxLinesEditor) appendRow(l core.TaxLine) {
	sep := e.app.settings.DecimalSeparator

	netE := widget.NewEntry()
	netE.SetPlaceHolder("Netto")
	satzE := widget.NewEntry()
	satzE.SetPlaceHolder("Satz %")
	mwstE := widget.NewEntry()
	mwstE.SetPlaceHolder("MwSt.")
	bruttoLbl := widget.NewLabel("")

	if l.Netto != 0 {
		netE.SetText(formatDecimal(l.Netto, sep))
	}
	if l.SatzProzent != 0 {
		satzE.SetText(formatDecimal(l.SatzProzent, sep))
	}
	if l.MwStBetrag != 0 {
		mwstE.SetText(formatDecimal(l.MwStBetrag, sep))
	}

	// Per-row state for auto-fill:
	//   mwstManual  — the MwSt was typed by the user (or loaded from storage),
	//                 so auto-fill must not overwrite it.
	//   autoFilling — guards our own SetText so it isn't mistaken for a manual edit.
	mwstManual := l.MwStBetrag != 0
	autoFilling := false

	// autoFill recomputes MwSt from Netto×Satz on EVERY net/satz change (so a
	// value entered mid-typing is corrected once the digits are complete),
	// unless the user has taken over the MwSt field.
	autoFill := func() {
		if e.seeding || mwstManual {
			return
		}
		n := parseFloat(netE.Text, sep)
		s := parseFloat(satzE.Text, sep)
		if n > 0 && s > 0 {
			autoFilling = true
			mwstE.SetText(formatDecimal(round2(n*s/100), sep))
			autoFilling = false
		}
	}

	netE.OnChanged = func(string) {
		if e.seeding {
			return
		}
		autoFill()
		e.refresh()
	}
	satzE.OnChanged = func(string) {
		if e.seeding {
			return
		}
		autoFill()
		e.refresh()
	}
	mwstE.OnChanged = func(string) {
		if e.seeding {
			return
		}
		if !autoFilling {
			// Direct user edit. Empty field re-enables auto-fill.
			mwstManual = strings.TrimSpace(mwstE.Text) != ""
		}
		e.refresh()
	}

	removeBtn := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
		for i, r := range e.rows {
			if r.netto == netE {
				e.rows = append(e.rows[:i], e.rows[i+1:]...)
				break
			}
		}
		e.rebuildRowsBox()
		e.refresh()
	})
	removeBtn.Importance = widget.DangerImportance

	row := taxLineRow{netto: netE, satz: satzE, mwst: mwstE, brutto: bruttoLbl, removeBtn: removeBtn}
	e.rows = append(e.rows, row)
	e.rowsBox.Add(rowContainer(row))
}

// rowContainer lays out a row: the four value cells share the width in a grid,
// the narrow ✕ button sits at the right at its natural (small) size.
func rowContainer(r taxLineRow) fyne.CanvasObject {
	return container.NewBorder(nil, nil, nil, r.removeBtn,
		container.NewGridWithColumns(4, r.netto, r.satz, r.mwst, r.brutto),
	)
}

// addLine appends an empty row and refreshes the container.
func (e *taxLinesEditor) addLine() {
	e.appendRow(core.TaxLine{})
	e.rowsBox.Refresh()
	e.refresh()
}

// rebuildRowsBox re-renders all rows into rowsBox after a removal.
func (e *taxLinesEditor) rebuildRowsBox() {
	e.rowsBox.RemoveAll()
	for _, r := range e.rows {
		e.rowsBox.Add(rowContainer(r))
	}
	e.rowsBox.Refresh()
}

// refresh updates each row's Brutto, the total Brutto, then calls onChange.
func (e *taxLinesEditor) refresh() {
	sep := e.app.settings.DecimalSeparator
	for _, r := range e.rows {
		rowBrutto := parseFloat(r.netto.Text, sep) + parseFloat(r.mwst.Text, sep)
		r.brutto.SetText(formatDecimal(rowBrutto, sep))
	}
	e.bruttoLabel.SetText(formatDecimal(core.ComputeBrutto(e.Lines(), e.Trinkgeld()), sep))
	if e.onChange != nil {
		e.onChange()
	}
}

// round2 rounds v to two decimal places.
func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

// Container returns the root Fyne object for embedding in dialogs.
func (e *taxLinesEditor) Container() fyne.CanvasObject {
	return e.container
}

// Lines reads the current entries back into a []core.TaxLine.
// Fully-empty rows (all three fields blank/zero) are skipped.
func (e *taxLinesEditor) Lines() []core.TaxLine {
	sep := e.app.settings.DecimalSeparator
	var out []core.TaxLine
	for _, r := range e.rows {
		n := parseFloat(r.netto.Text, sep)
		s := parseFloat(r.satz.Text, sep)
		m := parseFloat(r.mwst.Text, sep)
		if n == 0 && s == 0 && m == 0 {
			continue
		}
		out = append(out, core.TaxLine{Netto: n, SatzProzent: s, MwStBetrag: m})
	}
	return out
}

// Trinkgeld reads the tip field.
func (e *taxLinesEditor) Trinkgeld() float64 {
	return parseFloat(e.trinkEntry.Text, e.app.settings.DecimalSeparator)
}

// Brutto returns the current computed gross amount.
func (e *taxLinesEditor) Brutto() float64 {
	return core.ComputeBrutto(e.Lines(), e.Trinkgeld())
}
