package ui

import (
	"fmt"
	"image/color"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/bergx2/buchisy/internal/core"
	"github.com/bergx2/buchisy/internal/i18n"
)

// headerBackgroundColor is the light blue used behind table header cells.
var headerBackgroundColor = color.NRGBA{R: 214, G: 233, B: 248, A: 255}

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
}

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
	if hl.tooltip == "" || hl.onHover == nil || hl.tooltipShown {
		return
	}
	// Only surface the tooltip when the cell's text actually got
	// truncated — pointless (and visually noisy) when everything fits.
	if !hl.isTruncated() {
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
	// Don't recreate tooltip on every mouse move - only on MouseIn
}

func (hl *hoverLabel) MouseOut() {
	hl.tooltipShown = false
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
	decimalSeparator string        // Decimal separator for display
	summaryLabel     *widget.Label // Sum bar under the table (Netto/MwSt/Brutto for filtered rows)
	activeChip       string        // active quick-filter chip ("", "anhang", "teilzahlung", "ausgang")
	chipRow          *fyne.Container

	// Sort state. sortColumn is a CSV column ID like "Auftraggeber" or
	// the special sentinel "__typ__" for the file-type column.
	sortColumn    string
	sortAscending bool
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

	// Tooltip show/hide callbacks
	showTooltip := func(text string, pos fyne.Position) {
		if it.window == nil {
			return
		}
		it.hideTooltip()

		// Create label with no wrapping
		label := widget.NewLabel(text)
		label.Wrapping = fyne.TextWrapOff

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
			// Stack: row-background rectangle (zebra) + hoverLabel on top.
			bg := canvas.NewRectangle(color.Transparent)
			hl := newHoverLabel(showTooltip, hideTooltip)
			return container.NewStack(bg, hl)
		},
		func(id widget.TableCellID, cell fyne.CanvasObject) {
			stack := cell.(*fyne.Container)
			bg := stack.Objects[0].(*canvas.Rectangle)
			hoverLabel := stack.Objects[1].(*hoverLabel)
			hoverLabel.TextStyle.Bold = false

			// Zebra: subtle background on every second row.
			if id.Row%2 == 0 {
				bg.FillColor = stripeColor()
			} else {
				bg.FillColor = color.Transparent
			}
			bg.Refresh()

			switch id.Col {
			case 0:
				// Edit: pencil icon, tooltip "Bearbeiten", opens edit dialog.
				hoverLabel.Alignment = fyne.TextAlignCenter
				hoverLabel.SetText("✏️")
				hoverLabel.tooltip = "Bearbeiten"
				dataRow := id.Row
				hoverLabel.onTap = func() {
					if dataRow >= 0 && dataRow < len(it.filtered) && it.app != nil {
						it.app.showEditDialog(it.filtered[dataRow], nil)
					}
				}
			case 1:
				// Filetype column: shows the original file's extension
				// (PDF, JPG, …). Click opens the edit dialog — same as
				// every other cell. The original file itself is one
				// click away from inside the dialog via "Beleg öffnen"
				// or via the row's right-click menu.
				hoverLabel.Alignment = fyne.TextAlignCenter
				ext := strings.ToUpper(strings.TrimPrefix(
					filepath.Ext(it.filtered[id.Row].Dateiname), "."))
				hoverLabel.SetText(ext)
				hoverLabel.tooltip = "Bearbeiten"
				dataRow := id.Row
				hoverLabel.onTap = func() {
					if dataRow >= 0 && dataRow < len(it.filtered) && it.app != nil {
						it.app.showEditDialog(it.filtered[dataRow], nil)
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
					hoverLabel.onTap = nil
					return
				}
				colID := it.columnOrder[colIndex]

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

				// Every data cell is click-to-edit. Captured `dataRow` so
				// the closure stays valid when Fyne recycles the cell widget.
				dataRow := id.Row
				hoverLabel.onTap = func() {
					if dataRow >= 0 && dataRow < len(it.filtered) && it.app != nil {
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
		h := stack.Objects[1].(*hoverLabel)

		// Defaults reset on every update because cell widgets are recycled.
		h.tooltip = ""
		h.onTap = nil
		h.TextStyle.Bold = true

		switch id.Col {
		case 0:
			h.Alignment = fyne.TextAlignCenter
			h.SetText("✏️")
		case 1:
			h.Alignment = fyne.TextAlignCenter
			h.SetText(it.headerLabelFor("Typ", typSortKey))
			h.TextStyle.Bold = it.sortColumn == typSortKey
			h.onTap = func() { it.toggleSort(typSortKey) }
		default:
			colIndex := id.Col - 2
			if colIndex < 0 || colIndex >= len(it.columnOrder) {
				h.Alignment = fyne.TextAlignLeading
				h.SetText("")
				return
			}
			colID := it.columnOrder[colIndex]
			if numericColumns[colID] {
				h.Alignment = fyne.TextAlignTrailing
			} else {
				h.Alignment = fyne.TextAlignLeading
			}
			h.SetText(it.headerLabelFor(it.getColumnHeader(colID), colID))
			h.TextStyle.Bold = it.sortColumn == colID
			h.onTap = func() { it.toggleSort(colID) }
		}
		h.Refresh()
	}

	it.applyColumnWidths()

	// Track selected cell for the right-click context menu.
	it.table.OnSelected = func(id widget.TableCellID) {
		if id.Row >= 0 && id.Row < len(it.filtered) {
			it.lastSelectedRow = id.Row
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
	// the top bar (App.buildTopBar) — the table container itself only
	// holds the table and the summary row.
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
		return row.Unterordner == "Ausgangsrechnungen"
	}
	return true
}

// updateSummary recomputes the Netto/MwSt/Brutto totals for the
// currently filtered rows and writes them into summaryLabel.
func (it *InvoiceTable) updateSummary() {
	if it.summaryLabel == nil {
		return
	}
	var net, vat, brutto float64
	for _, r := range it.filtered {
		net += r.BetragNetto
		vat += r.SteuersatzBetrag
		brutto += r.Bruttobetrag
	}
	sep := ","
	if it.app != nil && it.app.settings.DecimalSeparator != "" {
		sep = it.app.settings.DecimalSeparator
	}
	it.summaryLabel.SetText(fmt.Sprintf(
		"Σ  Netto: %s   ·   MwSt: %s   ·   Brutto: %s",
		formatDecimal(net, sep),
		formatDecimal(vat, sep),
		formatDecimal(brutto, sep)))
}

func (it *InvoiceTable) applyColumnWidths() {
	if it.table == nil {
		return
	}

	it.table.SetColumnWidth(0, 50) // Edit column
	it.table.SetColumnWidth(1, 60) // Filetype column (fits "JPEG")
	for idx, colID := range it.columnOrder {
		width, ok := columnWidthMap[colID]
		if !ok {
			width = 140
		}
		it.table.SetColumnWidth(idx+2, width) // +2 for edit + filetype columns
	}
}

// hideTooltip hides the current tooltip popup if visible
func (it *InvoiceTable) hideTooltip() {
	if it.tooltipPopup != nil {
		it.tooltipPopup.Hide()
		it.tooltipPopup = nil
	}
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
	formatted := fmt.Sprintf("%.2f", amount)
	if it.decimalSeparator == "," {
		formatted = strings.Replace(formatted, ".", ",", 1)
	}
	return formatted
}

func (it *InvoiceTable) valueForColumn(row core.CSVRow, colID string) string {
	switch colID {
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
		return it.formatAmount(row.BetragNetto)
	case "Steuersatz_Prozent":
		return it.formatAmount(row.SteuersatzProzent)
	case "Steuersatz_Betrag":
		return it.formatAmount(row.SteuersatzBetrag)
	case "Bruttobetrag":
		return it.formatAmount(row.Bruttobetrag)
	case "Waehrung":
		return row.Waehrung
	case "Gegenkonto":
		if row.Gegenkonto == 0 {
			return ""
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
	case "Kommentar":
		return row.Kommentar
	case "BetragNetto_EUR":
		if row.BetragNetto_EUR > 0 {
			return it.formatAmount(row.BetragNetto_EUR)
		}
		return ""
	case "Gebuehr":
		if row.Gebuehr > 0 {
			return it.formatAmount(row.Gebuehr)
		}
		return ""
	case "HatAnhaenge":
		if row.HatAnhaenge {
			return "📎"
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
		return row.BuchungRef
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
