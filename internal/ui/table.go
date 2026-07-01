package ui

import (
	"fmt"
	"image/color"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/bergx2/buchisy/internal/core"
	"github.com/bergx2/buchisy/internal/i18n"
)

// headerBackgroundColor is the light blue used behind table header cells;
// headerSortBackgroundColor is the darker blue behind the active sort column.
var (
	headerBackgroundColor     = color.NRGBA{R: 214, G: 233, B: 248, A: 255}
	headerSortBackgroundColor = color.NRGBA{R: 95, G: 145, B: 205, A: 255}
)

// hoverLabel is a label that shows a tooltip on hover and optionally
// fires onTap when clicked (used to make the filename column open the
// edit dialog).
type hoverLabel struct {
	widget.Label
	onHover        func(string, fyne.Position)
	onExit         func()
	tooltip        string
	tooltipShown   bool
	lastTooltipPos fyne.Position
	onTap          func()
	alwaysTooltip  bool
	// bg is the cell's background rectangle (data cells only). The whole row is
	// filled by the table's UpdateCell based on the hovered row. Nil for headers.
	bg *canvas.Rectangle
	// rowIndex is the data row this (recycled) cell currently shows; onEnter /
	// onLeave notify the table so it can highlight/clear the ENTIRE row. All
	// three are set by UpdateCell; nil/0 for headers.
	rowIndex int
	onEnter  func(hl *hoverLabel) // notified with the cell so the table can highlight + frame the row
	onLeave  func()
}

// row hover styling: a light-blue fill spanning the whole hovered row.
var (
	cellHoverFill = color.NRGBA{R: 198, G: 222, B: 247, A: 255}
)

func newHoverLabel(onHover func(string, fyne.Position), onExit func()) *hoverLabel {
	hl := &hoverLabel{
		onHover: onHover,
		onExit:  onExit,
	}
	hl.ExtendBaseWidget(hl)
	hl.Truncation = fyne.TextTruncateEllipsis
	return hl
}

func (hl *hoverLabel) MouseIn(_ *desktop.MouseEvent) {
	// Highlight the whole row (the table refreshes every visible cell of this
	// row). Also cancels any pending row-clear scheduled by the cell we just
	// left, so moving within a row never flickers the highlight.
	if hl.onEnter != nil {
		hl.onEnter(hl)
	}
	if hl.onHover == nil {
		return
	}
	// Show the tooltip when the text is truncated, or when the cell opted in to
	// an always-on enriched tooltip (e.g. Gegenkonto shows account name+type).
	if hl.tooltip == "" || (!hl.alwaysTooltip && !hl.isTruncated()) {
		// This cell has nothing to show — clear any tooltip left over from the
		// previous cell so a stale popup doesn't linger on the wrong row.
		if hl.onExit != nil {
			hl.onExit()
		}
		hl.tooltipShown = false
		return
	}
	if hl.tooltipShown {
		return
	}
	// Anchor the tooltip just below the label rather than at the cursor:
	// the popup never overlaps the cursor, so MouseOut → re-MouseIn
	// flicker on hover stops.
	pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(hl)
	pos.Y += hl.Size().Height + 4
	hl.onHover(hl.tooltip, pos)
	hl.tooltipShown = true
	hl.lastTooltipPos = pos
}

// isTruncated reports whether the rendered text doesn't fit in the
// label's current width (i.e. ellipsis is showing).
func (hl *hoverLabel) isTruncated() bool {
	if hl.Text == "" {
		return false
	}
	natural := fyne.MeasureText(hl.Text, theme.TextSize(), hl.TextStyle)
	// Subtract the label's own inner padding so we compare against the
	// space actually available for glyphs.
	available := hl.Size().Width - 2*theme.InnerPadding()
	return natural.Width > available
}

func (hl *hoverLabel) MouseMoved(ev *desktop.MouseEvent) {
	// Don't recreate the tooltip on every move, but do let the table re-frame the
	// hovered row: if the content was scrolled by the wheel, the row under the
	// cursor changed position, and re-framing (a no-op unless it moved — see
	// frameRow) keeps the blue outline aligned.
	if hl.onEnter != nil {
		hl.onEnter(hl)
	}
}

func (hl *hoverLabel) MouseOut() {
	hl.tooltipShown = false
	// Data cells: defer clearing the row highlight + tooltip. If the mouse only
	// moved to a sibling cell in the same row, that cell's MouseIn cancels the
	// pending clear, so nothing flickers. The clear only fires when the mouse
	// truly leaves the table. Headers have no onLeave and fall back to onExit.
	if hl.onLeave != nil {
		hl.onLeave()
		return
	}
	if hl.onExit != nil {
		hl.onExit()
	}
}

func (hl *hoverLabel) Tapped(*fyne.PointEvent) {
	if hl.onTap != nil {
		hl.onTap()
	}
}

// TappedSecondary surfaces a "Kopieren"-Menü für jeden Zellen-/Header-
// Text — damit Tabellenwerte und Spaltenüberschriften per Rechtsklick
// in die Zwischenablage wandern, ohne den Umweg über die Edit-Felder.
func (hl *hoverLabel) TappedSecondary(ev *fyne.PointEvent) {
	if hl.Text == "" {
		return
	}
	canvas := fyne.CurrentApp().Driver().CanvasForObject(hl)
	if canvas == nil {
		return
	}
	menu := fyne.NewMenu("",
		fyne.NewMenuItem("Kopieren", func() {
			fyne.CurrentApp().Clipboard().SetContent(hl.Text)
		}),
	)
	widget.ShowPopUpMenuAtPosition(menu, canvas, ev.AbsolutePosition)
}

func (hl *hoverLabel) Cursor() desktop.Cursor {
	if hl.onTap != nil {
		return desktop.PointerCursor
	}
	return desktop.DefaultCursor
}

// InvoiceTable displays a table of invoices with filtering.
type InvoiceTable struct {
	bundle           *i18n.Bundle
	data             []core.CSVRow
	filtered         []core.CSVRow
	table            *widget.Table
	filterEntry      *widget.Entry
	container        *fyne.Container
	app              *App // Reference to main app for delete callback
	lastSelectedRow  int  // Track last selected row for context menu
	lastSelectedCol  int  // Track last selected data-column index (-1 = none)
	window           fyne.Window
	columnOrder      []string
	tooltipPopup     *widget.PopUp // Shared tooltip popup
	tooltipLabel     *widget.Label // Reused label inside tooltipPopup (avoids recreate-flicker)
	hoveredRow       int           // Row currently highlighted on hover (-1 = none)
	hoverGen         int           // Bumped on every cell-enter; guards deferred row-clear
	decimalSeparator string        // Decimal separator for display
	summaryLabel     *widget.Label // Sum bar under the table (Netto/MwSt/Brutto for filtered rows)
	activeChip       string        // active quick-filter chip ("", "anhang", "teilzahlung", "ausgang")
	chipRow          *fyne.Container

	// Sort state. sortColumn is a CSV column ID like "Auftraggeber" or
	// the special sentinel "__typ__" for the file-type column.
	sortColumn    string
	sortAscending bool

	// selectedRow is updated by OnSelected and used by keyboard navigation
	// (↑/↓/Enter/Del). -1 means nothing is selected.
	selectedRow int
}

const typSortKey = "__typ__"

// numericColumns lists columns whose cells should render right-aligned
// (so the comma/decimal point stacks visually) and sort numerically.
var numericColumns = map[string]bool{
	"BetragNetto":        true,
	"Steuersatz_Prozent": true,
	"Steuersatz_Betrag":  true,
	"Bruttobetrag":       true,
	"BetragNetto_EUR":    true,
	"Gebuehr":            true,
}

var columnWidthMap = map[string]float32{
	"Belegnummer":        110,
	"Dateiname":          250,
	"Rechnungsdatum":     120,
	"Jahr":               80,
	"Monat":              80,
	"Auftraggeber":       200,
	"Verwendungszweck":   220,
	"Rechnungsnummer":    160,
	"BetragNetto":        120,
	"Steuersatz_Prozent": 130,
	"Steuersatz_Betrag":  130,
	"Bruttobetrag":       120,
	"Waehrung":           80,
	"Gegenkonto":         110,
	"Kommentar":          200,
	"BetragNetto_EUR":    120,
	"Gebuehr":            100,
	"HatAnhaenge":        80,
	"Ausgangsrechnung":   110,
}

// NewInvoiceTable creates a new invoice table.
func NewInvoiceTable(bundle *i18n.Bundle, app *App) *InvoiceTable {
	it := &InvoiceTable{
		bundle:           bundle,
		data:             []core.CSVRow{},
		filtered:         []core.CSVRow{},
		app:              app,
		lastSelectedRow:  -1,
		lastSelectedCol:  -1,
		selectedRow:      -1,
		hoveredRow:       -1,
		columnOrder:      sanitizeColumnOrder(nil),
		decimalSeparator: ",", // Default
	}
	// Restore last-used sort from app preferences so the user sees the
	// same ordering as when they last closed the app.
	if app != nil && app.app != nil {
		prefs := app.app.Preferences()
		it.sortColumn = prefs.String("invoice_sort_col")
		it.sortAscending = prefs.BoolWithFallback("invoice_sort_asc", true)
	}

	it.filterEntry = widget.NewEntry()
	it.filterEntry.SetPlaceHolder(bundle.T("table.filter.placeholder"))
	it.filterEntry.OnChanged = func(query string) {
		it.applyFilter(query)
	}
	it.filterEntry.OnSubmitted = func(q string) {
		if strings.TrimSpace(q) != "" && it.app != nil {
			it.app.showGlobalSearch(q)
		}
	}

	// Tooltip show/hide callbacks
	showTooltip := func(text string, pos fyne.Position) {
		if it.window == nil {
			return
		}
		// Reuse the existing popup when one is already visible: just update the
		// text and reposition it. Recreating the popup on every cell caused the
		// tooltip to flicker as the mouse moved across a row.
		if it.tooltipPopup != nil && it.tooltipLabel != nil {
			it.tooltipLabel.SetText(text)
			// `pos` is already anchored below the label by MouseIn.
			it.tooltipPopup.ShowAtPosition(pos)
			return
		}

		// Create label with no wrapping
		label := widget.NewLabel(text)
		label.Wrapping = fyne.TextWrapOff
		it.tooltipLabel = label

		// Use a HBox container to keep text horizontal
		tooltipBox := container.NewHBox(label)

		it.tooltipPopup = widget.NewPopUp(
			container.NewPadded(tooltipBox),
			it.window.Canvas(),
		)
		// `pos` is already anchored below the label by MouseIn — no
		// extra offset, otherwise the tooltip drifts away from its row.
		it.tooltipPopup.ShowAtPosition(pos)
	}

	hideTooltip := func() {
		it.hideTooltip()
	}

	it.table = widget.NewTable(
		func() (int, int) {
			return len(it.filtered), len(it.columnOrder) + 2 // +2 for edit + filetype action columns
		},
		func() fyne.CanvasObject {
			// Stack: per-cell background rectangle + hoverLabel on top. The
			// hoverLabel recolours bg (light-blue fill + frame) while hovered.
			bg := canvas.NewRectangle(color.Transparent)
			hl := newHoverLabel(showTooltip, hideTooltip)
			hl.bg = bg
			hl.onEnter = it.onCellEnter // highlight the whole row on hover
			hl.onLeave = it.onCellLeave // deferred clear (no within-row flicker)
			return container.NewStack(bg, hl)
		},
		func(id widget.TableCellID, cell fyne.CanvasObject) {
			stack := cell.(*fyne.Container)
			bg := stack.Objects[0].(*canvas.Rectangle)
			hoverLabel := stack.Objects[1].(*hoverLabel)
			hoverLabel.TextStyle.Bold = false
			// Remember which data row this recycled cell now shows, so its
			// MouseIn can tell the table which row to highlight.
			hoverLabel.rowIndex = id.Row

			// Row background priority: hovered row → light blue (whole-row hover);
			// the row whose edit dialog is open → soft-amber band (matches the
			// active sidebar entry); otherwise transparent (no zebra striping).
			// This also clears leftover fill when a recycled cell moves rows.
			switch {
			case id.Row == it.hoveredRow:
				bg.FillColor = cellHoverFill
			case id.Row == it.selectedRow:
				bg.FillColor = sidebarActiveBG
			default:
				bg.FillColor = color.Transparent
			}
			bg.StrokeWidth = 0
			bg.Refresh()

			switch id.Col {
			case 0:
				// Edit: pencil icon, tooltip "Bearbeiten", opens edit dialog.
				hoverLabel.Alignment = fyne.TextAlignCenter
				hoverLabel.alwaysTooltip = false // recycled cell may carry a status flag
				hoverLabel.SetText("✏️")
				hoverLabel.tooltip = "Bearbeiten"
				dataRow := id.Row
				hoverLabel.onTap = func() {
					if dataRow >= 0 && dataRow < len(it.filtered) && it.app != nil {
						it.selectedRow = dataRow // mark the edited row (amber band)
						it.table.Refresh()
						it.app.showEditDialog(it.filtered[dataRow], nil)
					}
				}
			case 1:
				// Filetype column: shows the original file's extension
				// (PDF, JPG, …). Click OPENS THE ORIGINAL FILE in the OS
				// default app (outside BuchISY). Every other cell still
				// opens the edit/preview dialog.
				hoverLabel.Alignment = fyne.TextAlignCenter
				hoverLabel.alwaysTooltip = false // recycled cell may carry a status flag
				ext := strings.ToUpper(strings.TrimPrefix(
					filepath.Ext(it.filtered[id.Row].Dateiname), "."))
				hoverLabel.SetText(ext)
				hoverLabel.tooltip = "Datei öffnen"
				dataRow := id.Row
				hoverLabel.onTap = func() {
					if dataRow >= 0 && dataRow < len(it.filtered) && it.app != nil {
						r := it.filtered[dataRow]
						it.app.openFileInOS(it.app.resolveInvoicePath(r))
					}
				}
			default:
				// Regular text columns — id.Col is offset by the two
				// action columns above.
				colIndex := id.Col - 2
				if colIndex >= len(it.columnOrder) {
					hoverLabel.Alignment = fyne.TextAlignLeading
					hoverLabel.SetText("")
					hoverLabel.tooltip = ""
					hoverLabel.alwaysTooltip = false
					hoverLabel.onTap = nil
					return
				}
				colID := it.columnOrder[colIndex]

				// Reset the always-on flag so recycled cells don't leak it.
				hoverLabel.alwaysTooltip = false

				if numericColumns[colID] {
					hoverLabel.Alignment = fyne.TextAlignTrailing
				} else {
					hoverLabel.Alignment = fyne.TextAlignLeading
				}

				cellValue := it.getCellValue(id.Row, colID)
				hoverLabel.SetText(cellValue)

				// Tooltip for any non-empty cell — the hoverLabel
				// only shows it when the text is actually truncated
				// (see hoverLabel.MouseIn / isTruncated).
				hoverLabel.tooltip = cellValue

				// Gegenkonto: enrich tooltip with account name+type (always shown).
				// getCellValue already returns the "Nummer — Name" display text.
				if colID == "Gegenkonto" && it.app != nil && it.app.chart != nil {
					row := it.filtered[id.Row]
					if row.Gegenkonto != 0 {
						if acc, ok := it.app.chart.Find(row.Gegenkonto); ok {
							hoverLabel.tooltip = core.AccountTooltip(acc)
							hoverLabel.alwaysTooltip = true
						}
					}
				}

				// Status columns: Klartext tooltip always shown (alwaysTooltip).
				// Symbol set in use: ✓ linked/confirmed/yes · ⚠ uncovered · ○ open/unset.
				if id.Row < len(it.filtered) {
					row := it.filtered[id.Row]
					switch colID {
					case "BuchungRef":
						switch {
						case row.BuchungRef == core.CashConfirmedRef:
							hoverLabel.tooltip = it.bundle.T("status.cashConfirmed")
							hoverLabel.alwaysTooltip = true
						case row.BuchungRef != "":
							hoverLabel.tooltip = it.bundle.T("status.linked") + " — " + core.BuchungRefDisplay(row.BuchungRef)
							hoverLabel.alwaysTooltip = true
						case it.app != nil && it.app.isCashAccount(row.Bankkonto):
							if it.app.cashUncovered[row.Dateiname] {
								hoverLabel.tooltip = it.bundle.T("status.cashUncovered")
							} else {
								hoverLabel.tooltip = it.bundle.T("status.cashCovered")
							}
							hoverLabel.alwaysTooltip = true
						default:
							// "○" — open / not reconciled
							hoverLabel.tooltip = it.bundle.T("status.open")
							hoverLabel.alwaysTooltip = true
						}
					case "Teilzahlung":
						if row.Teilzahlung {
							hoverLabel.tooltip = it.bundle.T("status.partial")
							hoverLabel.alwaysTooltip = true
						}
					case "Ausgangsrechnung":
						if row.Ausgangsrechnung {
							hoverLabel.tooltip = it.bundle.T("status.outgoing")
							hoverLabel.alwaysTooltip = true
						}
					case "HatAnhaenge":
						if row.HatAnhaenge {
							hoverLabel.tooltip = it.bundle.T("status.attachment")
							hoverLabel.alwaysTooltip = true
						}
					}
				}

				// Every data cell is click-to-edit. Captured `dataRow` so
				// the closure stays valid when Fyne recycles the cell widget.
				dataRow := id.Row
				hoverLabel.onTap = func() {
					if dataRow >= 0 && dataRow < len(it.filtered) && it.app != nil {
						it.selectedRow = dataRow // mark the edited row (amber band)
						it.table.Refresh()
						it.app.showEditDialog(it.filtered[dataRow], nil)
					}
				}
			}
		},
	)

	// Native sticky header row. This also enables column resizing:
	// hovering a column divider in the header shows a resize cursor and
	// dragging it adjusts that column's width.
	it.table.ShowHeaderRow = true
	it.table.CreateHeader = func() fyne.CanvasObject {
		bg := canvas.NewRectangle(headerBackgroundColor)
		// hoverLabel is reused for headers because it already
		// implements Tappable + Cursor — needed for sort-on-click. The
		// tooltip stays disabled (tooltip="") so MouseIn early-returns.
		h := newHoverLabel(nil, nil)
		return container.NewStack(bg, h)
	}
	it.table.UpdateHeader = func(id widget.TableCellID, cell fyne.CanvasObject) {
		stack := cell.(*fyne.Container)
		bg := stack.Objects[0].(*canvas.Rectangle)
		h := stack.Objects[1].(*hoverLabel)

		// Defaults reset on every update because cell widgets are recycled.
		h.tooltip = ""
		h.onTap = nil
		h.TextStyle.Bold = true
		sorted := false

		switch id.Col {
		case 0:
			h.Alignment = fyne.TextAlignCenter
			h.SetText("✏️")
		case 1:
			h.Alignment = fyne.TextAlignCenter
			h.SetText(it.headerLabelFor("Typ", typSortKey))
			sorted = it.sortColumn == typSortKey
			h.TextStyle.Bold = sorted
			h.onTap = func() { it.toggleSort(typSortKey) }
		default:
			colIndex := id.Col - 2
			if colIndex < 0 || colIndex >= len(it.columnOrder) {
				h.Alignment = fyne.TextAlignLeading
				h.SetText("")
				bg.FillColor = headerBackgroundColor
				bg.Refresh()
				return
			}
			colID := it.columnOrder[colIndex]
			if numericColumns[colID] {
				h.Alignment = fyne.TextAlignTrailing
			} else {
				h.Alignment = fyne.TextAlignLeading
			}
			h.SetText(it.headerLabelFor(it.getColumnHeader(colID), colID))
			sorted = it.sortColumn == colID
			h.TextStyle.Bold = sorted
			h.onTap = func() { it.toggleSort(colID) }
		}
		// Dark-blue band behind the active sort column's header.
		if sorted {
			bg.FillColor = headerSortBackgroundColor
		} else {
			bg.FillColor = headerBackgroundColor
		}
		bg.Refresh()
		h.Refresh()
	}

	it.applyColumnWidths()

	// Track selected cell for the right-click context menu and keyboard nav.
	it.table.OnSelected = func(id widget.TableCellID) {
		if id.Row >= 0 && id.Row < len(it.filtered) {
			it.lastSelectedRow = id.Row
			it.selectedRow = id.Row
			colIdx := id.Col - 2 // cols 0 (edit) and 1 (filetype) are actions
			if colIdx >= 0 && colIdx < len(it.columnOrder) {
				it.lastSelectedCol = colIdx
			} else {
				it.lastSelectedCol = -1
			}
		}
	}

	// Create wrapper with right-click support
	tableWrapper := &rightClickTable{
		table:         it,
		wrappedWidget: it.table,
	}
	tableWrapper.ExtendBaseWidget(tableWrapper)

	// Sum bar — Netto / MwSt / Brutto totals across the currently
	// filtered rows. Updates on every applyFilter() / SetData() call.
	it.summaryLabel = widget.NewLabel("")
	it.summaryLabel.TextStyle = fyne.TextStyle{Bold: true}
	it.updateSummary()

	summaryBg := canvas.NewRectangle(cardBackgroundColor())
	summaryRow := container.NewStack(summaryBg,
		container.NewPadded(it.summaryLabel))

	// Quick-filter chips: instant boolean filters on top of the
	// free-text search. Click again to clear. Filter + chips live in
	// the Belege content header (App.buildBelegeContent) — the table
	// container itself only holds the table and the summary row.
	it.chipRow = container.NewHBox()
	it.refreshChips()

	it.container = container.NewBorder(
		nil,
		summaryRow,
		nil, nil,
		tableWrapper,
	)

	return it
}

// FilterEntry exposes the search input so the App can place it in the
// global top bar.
func (it *InvoiceTable) FilterEntry() *widget.Entry { return it.filterEntry }

// ChipRow exposes the chip strip for the same reason.
func (it *InvoiceTable) ChipRow() *fyne.Container { return it.chipRow }

// LegendButton returns a small "?"-button that opens a symbol-legend
// popup. Place it in the filter row (top bar) so users can always
// look up what ✓ / ⚠ / ○ mean.
func (it *InvoiceTable) LegendButton() *widget.Button {
	return widget.NewButton("?", func() {
		it.ShowLegend()
	})
}

// ShowLegend opens the symbol-legend popup explaining what ✓ / ⚠ / ○
// mean. Extracted from LegendButton so the global menu can reuse it.
func (it *InvoiceTable) ShowLegend() {
	if it.window == nil {
		return
	}
	title := widget.NewLabelWithStyle(
		it.bundle.T("legend.title"),
		fyne.TextAlignLeading,
		fyne.TextStyle{Bold: true},
	)
	// Each row: symbol label + meaning label side by side.
	row := func(sym, key string) *fyne.Container {
		symLbl := widget.NewLabelWithStyle(sym, fyne.TextAlignCenter, fyne.TextStyle{})
		symLbl.Wrapping = fyne.TextWrapOff
		meanLbl := widget.NewLabel(it.bundle.T(key))
		meanLbl.Wrapping = fyne.TextWrapWord
		return container.NewBorder(nil, nil, symLbl, nil, meanLbl)
	}
	rows := container.NewVBox(
		title,
		widget.NewSeparator(),
		row("✓", "legend.linked"),
		row("✓", "legend.cashConfirmed"),
		row("✓", "legend.cashCovered"),
		row("⚠", "legend.uncovered"),
		row("○", "legend.open"),
		row("✓", "legend.partial"),
		row("✓", "legend.outgoing"),
		row("✓", "legend.attachment"),
	)
	var popup *widget.PopUp
	closeBtn := widget.NewButton(it.bundle.T("common.close"), func() {
		if popup != nil {
			popup.Hide()
		}
	})
	content := container.NewVBox(rows, widget.NewSeparator(), container.NewCenter(closeBtn))
	popup = widget.NewModalPopUp(container.NewPadded(content), it.window.Canvas())
	popup.Show()
}

// refreshChips rebuilds the chip row so the active chip is shown
// HighImportance and the others LowImportance.
func (it *InvoiceTable) refreshChips() {
	if it.chipRow == nil {
		return
	}
	it.chipRow.Objects = it.chipRow.Objects[:0]
	type chipDef struct {
		key, label string
	}
	chips := []chipDef{
		{"", "Alle"},
		{"anhang", "Mit Anhängen"},
		{"teilzahlung", "Teilzahlung"},
		{"ausgang", "Ausgangsrechnungen"},
		{"obuchung", "Ohne Buchung"},
	}
	for _, c := range chips {
		c := c
		btn := widget.NewButton(c.label, func() {
			if it.activeChip == c.key {
				it.activeChip = "" // toggle off
			} else {
				it.activeChip = c.key
			}
			it.refreshChips()
			it.applyFilter(it.filterEntry.Text)
		})
		if it.activeChip == c.key {
			btn.Importance = widget.HighImportance
		} else {
			btn.Importance = widget.LowImportance
		}
		it.chipRow.Add(btn)
	}
	it.chipRow.Refresh()
}

// matchesChip checks whether a row passes the active chip filter.
func (it *InvoiceTable) matchesChip(row core.CSVRow) bool {
	switch it.activeChip {
	case "":
		return true
	case "anhang":
		return row.HatAnhaenge
	case "teilzahlung":
		return row.Teilzahlung
	case "ausgang":
		return row.Ausgangsrechnung || row.Unterordner == "Ausgangsrechnungen"
	case "obuchung":
		return !row.Buchung.Balanced()
	}
	return true
}

// updateSummary recomputes the Netto/MwSt/Brutto totals for the
// currently filtered rows and writes them into summaryLabel.
func (it *InvoiceTable) updateSummary() {
	if it.summaryLabel == nil {
		return
	}
	// Totals are in EUR: convert foreign-currency rows via their stored rate so
	// e.g. a USD 200 invoice contributes its EUR value, not 200 at face value.
	var net, vat, brutto float64
	for _, r := range it.filtered {
		if r.Waehrung != "" && r.Waehrung != "EUR" && r.Wechselkurs > 0 {
			net += r.BetragNetto / r.Wechselkurs
			vat += r.SteuersatzBetrag / r.Wechselkurs
			brutto += r.Bruttobetrag / r.Wechselkurs
		} else {
			net += r.BetragNetto
			vat += r.SteuersatzBetrag
			brutto += r.Bruttobetrag
		}
	}
	sep := ","
	if it.app != nil && it.app.settings.DecimalSeparator != "" {
		sep = it.app.settings.DecimalSeparator
	}
	it.summaryLabel.SetText(fmt.Sprintf(
		"Σ  Netto: %s   ·   MwSt: %s   ·   Brutto: %s",
		formatMoney(net, "EUR", sep),
		formatMoney(vat, "EUR", sep),
		formatMoney(brutto, "EUR", sep)))
}

func (it *InvoiceTable) applyColumnWidths() {
	if it.table == nil {
		return
	}

	it.table.SetColumnWidth(0, 50) // Edit column
	it.table.SetColumnWidth(1, 60) // Filetype column (fits "JPEG")
	var saved map[string]float32
	if it.app != nil {
		saved = it.app.settings.ColumnWidths
	}
	for idx, colID := range it.columnOrder {
		width, ok := columnWidthMap[colID]
		if !ok {
			width = 140
		}
		if w, ok := saved[colID]; ok && w > 0 { // user-adjusted width wins
			width = w
		}
		it.table.SetColumnWidth(idx+2, width) // +2 for edit + filetype columns
	}
}

// captureColumnWidths reads the table's current (possibly user-dragged) column
// widths and stores them into settings keyed by column ID, so they survive a
// table rebuild and app restart. Fyne 2.6 exposes no getter for column widths,
// so the private map is read via reflection (guarded: any failure is a no-op).
func (it *InvoiceTable) captureColumnWidths() {
	if it == nil || it.table == nil || it.app == nil {
		return
	}
	widths := readTableColumnWidths(it.table)
	if len(widths) == 0 {
		return
	}
	if it.app.settings.ColumnWidths == nil {
		it.app.settings.ColumnWidths = map[string]float32{}
	}
	for idx, colID := range it.columnOrder {
		if w, ok := widths[idx+2]; ok && w > 0 { // +2 for the edit + filetype action columns
			it.app.settings.ColumnWidths[colID] = w
		}
	}
}

// readTableColumnWidths returns a copy of a Fyne table's per-column width map
// (keyed by column index). Fyne exposes no public getter, so it reads the
// unexported field via reflection; it returns nil on any failure so a future
// Fyne change can't crash the app.
func readTableColumnWidths(t *widget.Table) (out map[int]float32) {
	defer func() { _ = recover() }()
	v := reflect.ValueOf(t).Elem().FieldByName("columnWidths")
	if !v.IsValid() || v.Kind() != reflect.Map || v.IsNil() {
		return nil
	}
	v = reflect.NewAt(v.Type(), unsafe.Pointer(v.UnsafeAddr())).Elem()
	m, ok := v.Interface().(map[int]float32)
	if !ok {
		return nil
	}
	out = make(map[int]float32, len(m))
	for k, val := range m {
		out[k] = val
	}
	return out
}

// hideTooltip hides the current tooltip popup if visible
func (it *InvoiceTable) hideTooltip() {
	if it.tooltipPopup != nil {
		it.tooltipPopup.Hide()
		it.tooltipPopup = nil
		it.tooltipLabel = nil
	}
}

// onCellEnter highlights the given data row (called from a cell's MouseIn) and
// bumps hoverGen so any row-clear scheduled by the cell we just left is voided.
func (it *InvoiceTable) onCellEnter(hl *hoverLabel) {
	row := hl.rowIndex
	it.hoverGen++
	if it.hoveredRow != row {
		it.hoveredRow = row
		if it.table != nil {
			it.table.Refresh()
		}
	}
	if it.app != nil && it.table != nil {
		it.app.frameRow(hl, it.table) // blue outline around the whole row
	}
}

// onCellLeave schedules clearing the row highlight + tooltip. The short delay
// lets a sibling cell's MouseIn (which bumps hoverGen) cancel it, so moving
// within a row doesn't flicker; the clear only actually runs when the mouse
// has left the table entirely.
func (it *InvoiceTable) onCellLeave() {
	gen := it.hoverGen
	time.AfterFunc(45*time.Millisecond, func() {
		fyne.Do(func() {
			if it.hoverGen != gen {
				return // re-entered another cell — keep the highlight
			}
			if it.hoveredRow != -1 {
				it.hoveredRow = -1
				if it.table != nil {
					it.table.Refresh()
				}
			}
			if it.app != nil {
				it.app.clearRowFrame()
			}
			it.hideTooltip()
		})
	})
}

// rightClickTable wraps the table to add right-click menu support.
// Uses widget.NewSimpleRenderer so Fyne handles Layout/MinSize/Refresh
// propagation for the single wrapped child — a previous hand-rolled
// renderer skipped Layout on Refresh, leaving the inner table at a stale
// width after content changes (visible as an empty strip on the right).
type rightClickTable struct {
	widget.BaseWidget
	table         *InvoiceTable
	wrappedWidget fyne.CanvasObject
}

// CreateRenderer creates the renderer for the wrapper.
func (r *rightClickTable) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(r.wrappedWidget)
}

// Tapped handles left-click (pass through).
func (r *rightClickTable) Tapped(e *fyne.PointEvent) {
	// Left click - do nothing, table handles it
}

// TappedSecondary handles right-click (show context menu).
func (r *rightClickTable) TappedSecondary(e *fyne.PointEvent) {
	// Right-click detected
	if r.table.lastSelectedRow < 0 || r.table.lastSelectedRow >= len(r.table.filtered) {
		return
	}

	it := r.table
	row := it.filtered[it.lastSelectedRow]

	// Base context menu items
	items := []*fyne.MenuItem{
		fyne.NewMenuItem(it.bundle.T("table.delete"), func() {
			if it.app != nil {
				it.app.showDeleteConfirmation(row)
			}
		}),
		fyne.NewMenuItem(it.bundle.T("table.copyCell"), func() {
			if it.lastSelectedCol >= 0 && it.lastSelectedCol < len(it.columnOrder) {
				value := it.getCellValue(it.lastSelectedRow, it.columnOrder[it.lastSelectedCol])
				fyne.CurrentApp().Clipboard().SetContent(value)
			}
		}),
		fyne.NewMenuItem(it.bundle.T("table.copyRow"), func() {
			values := make([]string, len(it.columnOrder))
			for i, colID := range it.columnOrder {
				values[i] = it.getCellValue(it.lastSelectedRow, colID)
			}
			fyne.CurrentApp().Clipboard().SetContent(joinRowValues(values))
		}),
	}

	// "Original / Anhang N öffnen" — added when the invoice has
	// attachments, so the user can jump to each file from the main list.
	if it.app != nil {
		items = append(items, fyne.NewMenuItemSeparator(),
			fyne.NewMenuItem("Original öffnen", func() {
				path := it.app.resolveInvoicePath(row)
				it.app.openFileInOS(path)
			}))
		if row.AnzahlAnhaenge > 0 {
			attPaths := it.app.invoiceAttachmentPaths(row)
			for i, p := range attPaths {
				p := p
				items = append(items,
					fyne.NewMenuItem(fmt.Sprintf("Anhang %d öffnen", i+1), func() {
						it.app.openFileInOS(p)
					}))
			}
		}

		// "Verknüpfung entfernen" — only shown when the invoice is linked to
		// a statement line. Clears BuchungRef after confirmation.
		if row.BuchungRef != "" {
			items = append(items, fyne.NewMenuItemSeparator(),
				fyne.NewMenuItem(it.bundle.T("table.unlink"), func() {
					it.app.unlinkInvoice(row)
				}))
		}
	}

	menu := fyne.NewMenu("", items...)

	// Show popup menu at click position
	if r.table.window != nil {
		widget.ShowPopUpMenuAtPosition(menu, r.table.window.Canvas(), e.AbsolutePosition)
	}
}

// SetWindow sets the window reference for context menus.
func (it *InvoiceTable) SetWindow(window fyne.Window) {
	it.window = window
}

// Container returns the container with the table and filter.
func (it *InvoiceTable) Container() *fyne.Container {
	return it.container
}

// RowCount returns the number of currently visible (filtered) rows.
func (it *InvoiceTable) RowCount() int {
	return len(it.filtered)
}

// SetData sets the table data and refreshes.
func (it *InvoiceTable) SetData(data []core.CSVRow) {
	it.data = data
	it.applyFilter(it.filterEntry.Text)
}

// SetColumnOrder updates the visible column order and refreshes widths.
func (it *InvoiceTable) SetColumnOrder(order []string) {
	it.columnOrder = sanitizeColumnOrder(order)
	it.applyColumnWidths()
	if it.filterEntry != nil {
		it.applyFilter(it.filterEntry.Text)
	} else if it.table != nil {
		it.table.Refresh()
	}
}

// SetDecimalSeparator sets the decimal separator for amount display.
func (it *InvoiceTable) SetDecimalSeparator(sep string) {
	it.decimalSeparator = sep
	if it.table != nil {
		it.table.Refresh()
	}
}

// applyFilter filters the table data based on the query.
func (it *InvoiceTable) applyFilter(query string) {
	query = strings.ToLower(strings.TrimSpace(query))
	it.filtered = it.filtered[:0]
	for _, row := range it.data {
		if !it.matchesChip(row) {
			continue
		}
		if query != "" && !it.matchesFilter(row, query) {
			continue
		}
		it.filtered = append(it.filtered, row)
	}
	it.selectedRow = -1
	it.applySort()
	it.updateSummary()
	it.table.Refresh()
}

// headerLabelFor decorates a header's display text with a sort
// indicator: "▴▾" when this column is not the active sort, "▲"/"▼"
// when it is. Two-character indicator hints that the column is
// clickable; single arrow shows the active direction.
func (it *InvoiceTable) headerLabelFor(text, colID string) string {
	if it.sortColumn == colID {
		if it.sortAscending {
			return text + " ▲"
		}
		return text + " ▼"
	}
	return text + " ▴▾"
}

// toggleSort cycles the sort state for a column: not-sorted →
// ascending → descending → ascending → … . Re-applies filter+sort
// so the table picks up the change immediately. The new state is
// persisted to app preferences so the next launch reopens with it.
func (it *InvoiceTable) toggleSort(colID string) {
	if it.sortColumn == colID {
		it.sortAscending = !it.sortAscending
	} else {
		it.sortColumn = colID
		it.sortAscending = true
	}
	if it.app != nil {
		it.app.persistInvoiceSort(it.sortColumn, it.sortAscending)
	}
	if it.filterEntry != nil {
		it.applyFilter(it.filterEntry.Text)
	} else {
		it.applySort()
		if it.table != nil {
			it.table.Refresh()
		}
	}
}

// applySort sorts the currently filtered rows in place per sortColumn
// + sortAscending. No-op when no sort is configured.
func (it *InvoiceTable) applySort() {
	if it.sortColumn == "" || len(it.filtered) < 2 {
		return
	}
	less := it.lessForColumn(it.sortColumn, it.sortAscending)
	if less == nil {
		return
	}
	sort.SliceStable(it.filtered, func(i, j int) bool {
		return less(it.filtered[i], it.filtered[j])
	})
}

// lessForColumn returns a row-comparator for the given column.
// Picks the natural comparison type per column: numeric for amounts
// and counts, time-parsed for dates, lexicographic everywhere else.
func (it *InvoiceTable) lessForColumn(colID string, asc bool) func(a, b core.CSVRow) bool {
	cmp := func(cond bool) bool {
		if asc {
			return cond
		}
		return !cond
	}
	switch colID {
	case typSortKey:
		return func(a, b core.CSVRow) bool {
			ax := strings.ToLower(filepath.Ext(a.Dateiname))
			bx := strings.ToLower(filepath.Ext(b.Dateiname))
			return cmp(ax < bx)
		}
	case "BetragNetto":
		return func(a, b core.CSVRow) bool { return cmp(a.BetragNetto < b.BetragNetto) }
	case "Steuersatz_Prozent":
		return func(a, b core.CSVRow) bool { return cmp(a.SteuersatzProzent < b.SteuersatzProzent) }
	case "Steuersatz_Betrag":
		return func(a, b core.CSVRow) bool { return cmp(a.SteuersatzBetrag < b.SteuersatzBetrag) }
	case "Bruttobetrag":
		return func(a, b core.CSVRow) bool { return cmp(a.Bruttobetrag < b.Bruttobetrag) }
	case "Rechnungsdatum":
		return func(a, b core.CSVRow) bool {
			return cmp(parseGermanDate(a.Rechnungsdatum).Before(parseGermanDate(b.Rechnungsdatum)))
		}
	case "Bezahldatum":
		return func(a, b core.CSVRow) bool {
			return cmp(parseGermanDate(a.Bezahldatum).Before(parseGermanDate(b.Bezahldatum)))
		}
	case "Jahr":
		return func(a, b core.CSVRow) bool { return cmp(atoiSafe(a.Jahr) < atoiSafe(b.Jahr)) }
	case "Monat":
		return func(a, b core.CSVRow) bool { return cmp(atoiSafe(a.Monat) < atoiSafe(b.Monat)) }
	case "Gegenkonto":
		return func(a, b core.CSVRow) bool { return cmp(a.Gegenkonto < b.Gegenkonto) }
	case "Teilzahlung":
		return func(a, b core.CSVRow) bool {
			// false < true so ascending puts empty cells first
			if a.Teilzahlung == b.Teilzahlung {
				return false
			}
			return cmp(!a.Teilzahlung)
		}
	}
	// Everything else: case-insensitive lexicographic on the rendered value.
	return func(a, b core.CSVRow) bool {
		av := strings.ToLower(it.valueForColumn(a, colID))
		bv := strings.ToLower(it.valueForColumn(b, colID))
		return cmp(av < bv)
	}
}

func parseGermanDate(s string) time.Time {
	t, _ := time.Parse("02.01.2006", s)
	return t
}

func atoiSafe(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

// matchesFilter checks if a row matches the filter query.
func (it *InvoiceTable) matchesFilter(row core.CSVRow, query string) bool {
	for _, colID := range it.columnOrder {
		value := strings.ToLower(it.valueForColumn(row, colID))
		if value != "" && strings.Contains(value, query) {
			return true
		}
	}
	return false
}

// getColumnHeader returns the header text for a column.
func (it *InvoiceTable) getColumnHeader(colID string) string {
	return columnHeaderFor(it.bundle, colID)
}

// columnHeaderFor resolves a column's display name: the i18n translation
// when present (so "Steuersatz_Betrag" shows as "MwSt.-Betrag" rather
// than the raw map fallback "Steuerbetrag"), otherwise the static
// ColumnDisplayNames entry, otherwise the bare column ID.
func columnHeaderFor(bundle *i18n.Bundle, colID string) string {
	if key, ok := core.ColumnTranslationKeys[colID]; ok && bundle != nil {
		if header := bundle.T(key); header != key {
			return header
		}
	}
	if display, ok := core.ColumnDisplayNames[colID]; ok && display != "" {
		return display
	}
	return colID
}

// getCellValue returns the cell value for a row and column.
func (it *InvoiceTable) getCellValue(row int, colID string) string {
	if row >= len(it.filtered) {
		return ""
	}

	return it.valueForColumn(it.filtered[row], colID)
}

// formatAmount formats a float with the configured decimal separator.
func (it *InvoiceTable) formatAmount(amount float64) string {
	// core.FormatAmount adds thousands separators (e.g. "6.300,00") and honours
	// the decimal separator setting.
	return core.FormatAmount(amount, it.decimalSeparator)
}

// formatMoneyCell renders an amount with the ISO currency code before it, e.g.
// "EUR 165,55" or "USD 200,00" — so a foreign-currency invoice is never mistaken
// for EUR. An empty currency defaults to EUR.
func (it *InvoiceTable) formatMoneyCell(amount float64, currency string) string {
	code := strings.TrimSpace(currency)
	if code == "" {
		code = "EUR"
	}
	return code + " " + it.formatAmount(amount)
}

func (it *InvoiceTable) valueForColumn(row core.CSVRow, colID string) string {
	switch colID {
	case "Belegnummer":
		return row.Belegnummer
	case "Dateiname":
		return row.Dateiname
	case "Rechnungsdatum":
		return row.Rechnungsdatum
	case "Jahr":
		return row.Jahr
	case "Monat":
		return row.Monat
	case "Auftraggeber":
		return row.Auftraggeber
	case "Verwendungszweck":
		return row.Verwendungszweck
	case "Rechnungsnummer":
		return row.Rechnungsnummer
	case "VATID":
		return row.VATID
	case "BetragNetto":
		return it.formatMoneyCell(row.BetragNetto, row.Waehrung)
	case "Steuersatz_Prozent":
		return it.formatAmount(row.SteuersatzProzent) // a percentage, no currency
	case "Steuersatz_Betrag":
		return it.formatMoneyCell(row.SteuersatzBetrag, row.Waehrung)
	case "Bruttobetrag":
		return it.formatMoneyCell(row.Bruttobetrag, row.Waehrung)
	case "Waehrung":
		return row.Waehrung
	case "Gegenkonto":
		if row.Gegenkonto == 0 {
			return ""
		}
		if it.app != nil && it.app.chart != nil {
			if acc, ok := it.app.chart.Find(row.Gegenkonto); ok {
				return core.AccountDisplay(acc)
			}
		}
		return fmt.Sprintf("%d", row.Gegenkonto)
	case "Bankkonto":
		return row.Bankkonto
	case "Bezahldatum":
		return row.Bezahldatum
	case "Teilzahlung":
		if row.Teilzahlung {
			return "✓"
		}
		return ""
	case "Ausgangsrechnung":
		if row.Ausgangsrechnung {
			return "✓"
		}
		return ""
	case "Kommentar":
		return row.Kommentar
	case "BetragNetto_EUR":
		if row.BetragNetto_EUR > 0 {
			return it.formatMoneyCell(row.BetragNetto_EUR, "EUR")
		}
		return ""
	case "Gebuehr":
		if row.Gebuehr > 0 {
			return it.formatAmount(row.Gebuehr)
		}
		return ""
	case "HatAnhaenge":
		// Symbol set: ✓ (yes/linked/confirmed) · ⚠ (warning/uncovered) · ○ (open/none)
		// HatAnhaenge uses ✓ for consistency; colour-emoji like 📎 are avoided
		// because the Fyne system font may not render them reliably.
		if row.HatAnhaenge {
			return "✓"
		}
		return ""
	case "AnzahlAnhaenge":
		if row.AnzahlAnhaenge > 0 {
			return fmt.Sprintf("%d", row.AnzahlAnhaenge)
		}
		return ""
	case "Unterordner":
		return row.Unterordner
	case "BuchungRef":
		if row.BuchungRef == core.CashConfirmedRef {
			return "✓ " + it.bundle.T("status.cashConfirmed")
		}
		if row.BuchungRef != "" {
			return "✓ " + core.BuchungRefDisplay(row.BuchungRef) // linked (1 or more lines)
		}
		if it.app != nil && it.app.isCashAccount(row.Bankkonto) {
			if it.app.cashUncovered[row.Dateiname] {
				return "⚠ " + it.bundle.T("status.cashUncovered")
			}
			return "✓ " + it.bundle.T("status.cashCovered")
		}
		return "○" // unlinked
	default:
		return ""
	}
}

func sanitizeColumnOrder(order []string) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0, len(core.DefaultCSVColumns))

	for _, col := range order {
		if _, ok := core.ColumnDisplayNames[col]; ok {
			if _, exists := seen[col]; !exists {
				result = append(result, col)
				seen[col] = struct{}{}
			}
		}
	}

	for _, col := range core.DefaultCSVColumns {
		if _, exists := seen[col]; !exists {
			result = append(result, col)
			seen[col] = struct{}{}
		}
	}

	return result
}

// joinRowValues joins cell values with a tab, for clipboard copy of a row.
func joinRowValues(values []string) string {
	return strings.Join(values, "\t")
}

// CopySelectedRow returns the currently selected row's values joined by tab,
// or "" if nothing is selected. Backs the Ctrl+C (ShortcutCopy) handler.
func (it *InvoiceTable) CopySelectedRow() string {
	if it.selectedRow < 0 || it.selectedRow >= len(it.filtered) {
		return ""
	}
	vals := make([]string, 0, len(it.columnOrder))
	for _, colID := range it.columnOrder {
		vals = append(vals, it.getCellValue(it.selectedRow, colID))
	}
	return joinRowValues(vals)
}

// SelectByDateiname selects and scrolls to the row whose Dateiname
// matches name. No-op if not found.
func (it *InvoiceTable) SelectByDateiname(name string) {
	for i, row := range it.filtered {
		if row.Dateiname == name {
			id := widget.TableCellID{Row: i, Col: 0}
			it.table.Select(id)
			it.table.ScrollTo(id)
			return
		}
	}
}

// RegisterKeyHandler wires ↑/↓/Enter/Del keyboard navigation onto the
// given canvas (expected: the main window canvas). The handler is a no-op
// when a text entry widget has focus so that search/filter typing is
// unaffected. Dialog windows use their own separate canvases, so this
// handler never fires while an edit or delete dialog is open.
func (it *InvoiceTable) RegisterKeyHandler(cv fyne.Canvas) {
	cv.SetOnTypedKey(func(ev *fyne.KeyEvent) {
		// Guard: if any text-entry widget has keyboard focus, let it
		// handle all keys — don't steal from the search/filter box.
		if _, isEntry := cv.Focused().(*widget.Entry); isEntry {
			return
		}

		switch ev.Name {
		case fyne.KeyUp:
			if _, isTable := cv.Focused().(*widget.Table); isTable {
				return
			}
			it.moveSelection(-1)
		case fyne.KeyDown:
			if _, isTable := cv.Focused().(*widget.Table); isTable {
				return
			}
			it.moveSelection(1)
		case fyne.KeyReturn, fyne.KeyEnter:
			it.openSelected()
		case fyne.KeyDelete, fyne.KeyBackspace:
			it.deleteSelected()
		}
	})

	// Ctrl/Cmd+C copies the selected row. Fyne routes copy to the focused
	// widget first (Entry gets Entry-copy); the table is not Shortcutable,
	// so it falls through to this canvas handler.
	// IMPORTANT: canvas.AddShortcut persists on the window canvas across
	// SetContent, so it would also fire in Konten mode (no invoice table).
	// Gate on Belege mode + a live table to avoid copying stale rows.
	cv.AddShortcut(&fyne.ShortcutCopy{}, func(fyne.Shortcut) {
		if it.app == nil || it.app.viewMode != "" || it.app.invoiceTable != it {
			return
		}
		if s := it.CopySelectedRow(); s != "" {
			fyne.CurrentApp().Clipboard().SetContent(s)
		}
	})
}

// moveSelection shifts the selected row by delta, clamped to valid range.
func (it *InvoiceTable) moveSelection(delta int) {
	count := it.RowCount()
	if count == 0 {
		return
	}
	next := it.selectedRow + delta
	if next < 0 {
		next = 0
	}
	if next >= count {
		next = count - 1
	}
	it.selectedRow = next
	id := widget.TableCellID{Row: next, Col: 0}
	it.table.Select(id)
	it.table.ScrollTo(id)
}

// openSelected opens the edit dialog for the currently selected row.
func (it *InvoiceTable) openSelected() {
	if it.selectedRow < 0 || it.selectedRow >= len(it.filtered) || it.app == nil {
		return
	}
	it.app.showEditDialog(it.filtered[it.selectedRow], nil)
}

// deleteSelected triggers the existing delete-confirmation flow for the
// currently selected row — identical to the right-click "Löschen" path.
func (it *InvoiceTable) deleteSelected() {
	if it.selectedRow < 0 || it.selectedRow >= len(it.filtered) || it.app == nil {
		return
	}
	it.app.showDeleteConfirmation(it.filtered[it.selectedRow])
}
