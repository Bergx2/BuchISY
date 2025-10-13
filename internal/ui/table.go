package ui

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/bergx2/buchisy/internal/core"
	"github.com/bergx2/buchisy/internal/i18n"
)

// InvoiceTable displays a table of invoices with filtering.
type InvoiceTable struct {
	bundle          *i18n.Bundle
	data            []core.CSVRow
	filtered        []core.CSVRow
	table           *widget.Table
	filterEntry     *widget.Entry
	container       *fyne.Container
	app             *App // Reference to main app for delete callback
	lastSelectedRow int  // Track last selected row for context menu
	window          fyne.Window
	columnOrder     []string
}

var columnWidthMap = map[string]float32{
	"Dateiname":          250,
	"Rechnungsdatum":     140,
	"Datum_Deutsch":      120,
	"Jahr":               80,
	"Monat":              80,
	"Firmenname":         200,
	"Kurzbezeichnung":    220,
	"Rechnungsnummer":    160,
	"BetragNetto":        120,
	"Steuersatz_Prozent": 130,
	"Steuersatz_Betrag":  130,
	"Bruttobetrag":       120,
	"Waehrung":           80,
	"Gegenkonto":         110,
}

// NewInvoiceTable creates a new invoice table.
func NewInvoiceTable(bundle *i18n.Bundle, app *App) *InvoiceTable {
	it := &InvoiceTable{
		bundle:          bundle,
		data:            []core.CSVRow{},
		filtered:        []core.CSVRow{},
		app:             app,
		lastSelectedRow: -1,
		columnOrder:     sanitizeColumnOrder(nil),
	}

	it.filterEntry = widget.NewEntry()
	it.filterEntry.SetPlaceHolder(bundle.T("table.filter.placeholder"))
	it.filterEntry.OnChanged = func(query string) {
		it.applyFilter(query)
	}

	it.table = widget.NewTable(
		func() (int, int) {
			return len(it.filtered) + 1, len(it.columnOrder) + 1
		},
		func() fyne.CanvasObject {
			return container.NewStack(
				widget.NewLabel(""),
				widget.NewButton("", nil),
			)
		},
		func(id widget.TableCellID, cell fyne.CanvasObject) {
			stack := cell.(*fyne.Container)

			if id.Col == 0 {
				// Delete button column (FIRST column now)
				label := stack.Objects[0].(*widget.Label)
				btn := stack.Objects[1].(*widget.Button)

				if id.Row == 0 {
					// Header
					label.SetText("")
					label.TextStyle.Bold = true
					btn.Hide()
				} else {
					// Data row
					label.Hide()
					btn.SetText("🗑️")
					btn.Show()

					// Set delete handler
					dataRow := id.Row - 1
					btn.OnTapped = func() {
						if dataRow >= 0 && dataRow < len(it.filtered) && it.app != nil {
							it.app.showDeleteConfirmation(it.filtered[dataRow])
						}
					}
				}
			} else {
				// Regular text columns (shift by -1 since delete is first)
				label := stack.Objects[0].(*widget.Label)
				btn := stack.Objects[1].(*widget.Button)
				btn.Hide()

				label.Show()
				label.Truncation = fyne.TextTruncateEllipsis

				colIndex := id.Col - 1
				if colIndex >= len(it.columnOrder) {
					label.SetText("")
					return
				}
				colID := it.columnOrder[colIndex]

				if id.Row == 0 {
					// Header row
					label.TextStyle.Bold = true
					label.SetText(it.getColumnHeader(colID))
				} else {
					// Data row
					label.TextStyle.Bold = false
					label.SetText(it.getCellValue(id.Row-1, colID))
				}
			}
		},
	)

	it.applyColumnWidths()

	// Track selected row for right-click context menu
	it.table.OnSelected = func(id widget.TableCellID) {
		if id.Row > 0 {
			it.lastSelectedRow = id.Row - 1 // Convert to data index
		}
	}

	// Create wrapper with right-click support
	tableWrapper := &rightClickTable{
		table:         it,
		wrappedWidget: it.table,
	}

	it.container = container.NewBorder(
		it.filterEntry,
		nil, nil, nil,
		tableWrapper,
	)

	return it
}

func (it *InvoiceTable) applyColumnWidths() {
	if it.table == nil {
		return
	}

	it.table.SetColumnWidth(0, 50) // Delete button column
	for idx, colID := range it.columnOrder {
		width, ok := columnWidthMap[colID]
		if !ok {
			width = 140
		}
		it.table.SetColumnWidth(idx+1, width)
	}
}

// rightClickTable wraps the table to add right-click menu support.
type rightClickTable struct {
	widget.BaseWidget
	table         *InvoiceTable
	wrappedWidget fyne.CanvasObject
}

// CreateRenderer creates the renderer for the wrapper.
func (r *rightClickTable) CreateRenderer() fyne.WidgetRenderer {
	return &rightClickTableRenderer{
		wrapper: r,
	}
}

// rightClickTableRenderer renders the wrapped table.
type rightClickTableRenderer struct {
	wrapper *rightClickTable
}

func (r *rightClickTableRenderer) Layout(size fyne.Size) {
	r.wrapper.wrappedWidget.Resize(size)
}

func (r *rightClickTableRenderer) MinSize() fyne.Size {
	return r.wrapper.wrappedWidget.MinSize()
}

func (r *rightClickTableRenderer) Refresh() {
	r.wrapper.wrappedWidget.Refresh()
}

func (r *rightClickTableRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.wrapper.wrappedWidget}
}

func (r *rightClickTableRenderer) Destroy() {}

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

	row := r.table.filtered[r.table.lastSelectedRow]

	// Create context menu
	menu := fyne.NewMenu("",
		fyne.NewMenuItem(r.table.bundle.T("table.delete"), func() {
			if r.table.app != nil {
				r.table.app.showDeleteConfirmation(row)
			}
		}),
	)

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

// applyFilter filters the table data based on the query.
func (it *InvoiceTable) applyFilter(query string) {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		it.filtered = it.data
	} else {
		it.filtered = []core.CSVRow{}
		for _, row := range it.data {
			if it.matchesFilter(row, query) {
				it.filtered = append(it.filtered, row)
			}
		}
	}
	it.table.Refresh()
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
	if key, ok := core.ColumnTranslationKeys[colID]; ok {
		header := it.bundle.T(key)
		if header != key {
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

func (it *InvoiceTable) valueForColumn(row core.CSVRow, colID string) string {
	switch colID {
	case "Dateiname":
		return row.Dateiname
	case "Rechnungsdatum":
		return row.Rechnungsdatum
	case "Datum_Deutsch":
		return row.DatumDeutsch
	case "Jahr":
		return row.Jahr
	case "Monat":
		return row.Monat
	case "Firmenname":
		return row.Firmenname
	case "Kurzbezeichnung":
		return row.Kurzbezeichnung
	case "Rechnungsnummer":
		return row.Rechnungsnummer
	case "BetragNetto":
		return fmt.Sprintf("%.2f", row.BetragNetto)
	case "Steuersatz_Prozent":
		return fmt.Sprintf("%.2f", row.SteuersatzProzent)
	case "Steuersatz_Betrag":
		return fmt.Sprintf("%.2f", row.SteuersatzBetrag)
	case "Bruttobetrag":
		return fmt.Sprintf("%.2f", row.Bruttobetrag)
	case "Waehrung":
		return row.Waehrung
	case "Gegenkonto":
		if row.Gegenkonto == 0 {
			return ""
		}
		return fmt.Sprintf("%d", row.Gegenkonto)
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
