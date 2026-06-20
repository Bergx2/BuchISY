package ui

import (
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/bergx2/buchisy/internal/core"
)

// taxLineRow holds the three entry widgets for a single VAT line.
type taxLineRow struct {
	netto   *widget.Entry
	satz    *widget.Entry
	mwst    *widget.Entry
	removeBtn *widget.Button
}

// taxLinesEditor is a reusable widget that lets the user edit a list of
// VAT lines (Netto / Satz % / MwSt-Betrag) plus a Trinkgeld field. It
// exposes the computed Brutto as a read-only label that updates live.
type taxLinesEditor struct {
	app       *App
	onChange  func()

	rows       []taxLineRow
	trinkEntry *widget.Entry
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

	e.rowsBox = container.NewVBox()

	// Trinkgeld entry
	e.trinkEntry = widget.NewEntry()
	e.trinkEntry.SetPlaceHolder("0" + a.settings.DecimalSeparator + "00")
	if trinkgeld != 0 {
		e.trinkEntry.SetText(formatDecimal(trinkgeld, a.settings.DecimalSeparator))
	}
	e.trinkEntry.OnChanged = func(string) { e.refresh() }

	// Brutto label (read-only)
	e.bruttoLabel = widget.NewLabelWithStyle(
		formatDecimal(0, a.settings.DecimalSeparator),
		fyne.TextAlignLeading, fyne.TextStyle{Bold: true},
	)

	// Add-line button
	addBtn := widget.NewButtonWithIcon("+ MwSt.", theme.ContentAddIcon(), func() {
		e.addLine()
	})
	addBtn.Importance = widget.LowImportance

	// Seed with provided lines (or one empty row)
	if len(lines) == 0 {
		e.appendRow(core.TaxLine{})
	} else {
		for _, l := range lines {
			e.appendRow(l)
		}
	}

	// Trinkgeld row
	trinkRow := container.NewBorder(nil, nil,
		widget.NewLabelWithStyle("Trinkgeld", fyne.TextAlignLeading, fyne.TextStyle{}),
		nil,
		e.trinkEntry,
	)

	// Brutto summary row
	bruttoRow := container.NewBorder(nil, nil,
		widget.NewLabelWithStyle("Brutto", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		nil,
		e.bruttoLabel,
	)

	e.container = container.NewVBox(
		e.rowsBox,
		addBtn,
		widget.NewSeparator(),
		trinkRow,
		bruttoRow,
	)

	e.refresh()
	return e
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

	if l.Netto != 0 {
		netE.SetText(formatDecimal(l.Netto, sep))
	}
	if l.SatzProzent != 0 {
		satzE.SetText(formatDecimal(l.SatzProzent, sep))
	}
	if l.MwStBetrag != 0 {
		mwstE.SetText(formatDecimal(l.MwStBetrag, sep))
	}

	row := taxLineRow{netto: netE, satz: satzE, mwst: mwstE}

	// Auto-compute MwSt when Netto and Satz are set but MwSt is empty.
	autoFill := func() {
		n := parseFloat(netE.Text, sep)
		s := parseFloat(satzE.Text, sep)
		m := parseFloat(mwstE.Text, sep)
		if n > 0 && s > 0 && m == 0 {
			mwstE.SetText(formatDecimal(round2(n*s/100), sep))
		}
	}

	netE.OnChanged = func(string) { autoFill(); e.refresh() }
	satzE.OnChanged = func(string) { autoFill(); e.refresh() }
	mwstE.OnChanged = func(string) { e.refresh() }

	// Remove button — finds the row by identity (pointer equality on netto entry).
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

	row.removeBtn = removeBtn
	e.rows = append(e.rows, row)

	// Build the grid for this row: [Netto | Satz% | MwSt | ✕]
	rowWidget := container.NewGridWithColumns(4, netE, satzE, mwstE, removeBtn)
	e.rowsBox.Add(rowWidget)
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
		e.rowsBox.Add(container.NewGridWithColumns(4, r.netto, r.satz, r.mwst, r.removeBtn))
	}
	e.rowsBox.Refresh()
}

// refresh recomputes Brutto and calls onChange.
func (e *taxLinesEditor) refresh() {
	brutto := core.ComputeBrutto(e.Lines(), e.Trinkgeld())
	e.bruttoLabel.SetText(formatDecimal(brutto, e.app.settings.DecimalSeparator))
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
