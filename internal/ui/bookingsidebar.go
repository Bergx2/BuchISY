package ui

import (
	"fmt"
	"image/color"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/bergx2/buchisy/internal/core"
)

// circledDigits is the Unicode "Enclosed Alphanumerics" range
// U+2460..U+2473 → "①" … "⑳". Index 0 = "①".
var circledDigits = []rune{
	'①', '②', '③', '④', '⑤',
	'⑥', '⑦', '⑧', '⑨', '⑩',
	'⑪', '⑫', '⑬', '⑭', '⑮',
	'⑯', '⑰', '⑱', '⑲', '⑳',
}

// circledNumber returns "①" for 1, "②" for 2, etc. For values outside
// 1..20 it falls back to "(N)".
func circledNumber(n int) string {
	if n >= 1 && n <= len(circledDigits) {
		return string(circledDigits[n-1])
	}
	return fmt.Sprintf("(%d)", n)
}

// buildBookingSidebar renders the clickable per-page booking list for
// a single bank statement. onPick fires when the user taps a row.
//
// If the parser hasn't run yet (or the PDF mtime has changed), it
// runs here synchronously — typical statements have ≤ 30 transaction
// lines, so the cost is negligible (well under 100ms).
//
// Returns a scrollable container with a fixed-ish width suitable for
// dropping into an HSplit. When the statement has no detected
// bookings, returns a small placeholder so the layout doesn't
// collapse.
func (a *App) buildBookingSidebar(
	statementPath string,
	meta *core.StatementMetadata,
	onPick func(b core.StatementBooking),
) fyne.CanvasObject {
	if changed, err := core.EnsureBookingsParsed(statementPath, meta); err != nil {
		a.logger.Warn("Could not parse bookings for %s: %v", statementPath, err)
	} else if changed {
		// Persist the freshly parsed list back to metadata.json so we
		// don't repeat the work next time.
		folder := a.statementFolder(a.kontenAccount)
		metaMap, _ := core.LoadStatementMeta(folder)
		// Refresh from disk to avoid clobbering concurrent edits, then
		// overwrite just this entry.
		rel := relFromStatementPath(folder, statementPath)
		if rel != "" {
			metaMap[rel] = *meta
			if err := core.SaveStatementMeta(folder, metaMap); err != nil {
				a.logger.Warn("Save statement metadata: %v", err)
			}
		}
	}

	if len(meta.Bookings) == 0 {
		return container.NewCenter(widget.NewLabel("Keine Buchungen erkannt."))
	}

	// Group bookings by page so we can insert "Seite N" headers.
	rows := container.NewVBox()
	currentPage := -1
	for _, b := range meta.Bookings {
		if b.Page != currentPage {
			currentPage = b.Page
			rows.Add(bookingPageHeader(currentPage + 1))
		}
		rows.Add(a.buildBookingRow(b, onPick))
	}
	return container.NewVScroll(rows)
}

// bookingPageHeader is the small "Seite N" separator above each page's
// booking list.
func bookingPageHeader(page int) fyne.CanvasObject {
	lbl := widget.NewLabelWithStyle(
		fmt.Sprintf("Seite %d", page),
		fyne.TextAlignLeading,
		fyne.TextStyle{Bold: true},
	)
	return container.NewVBox(lbl, widget.NewSeparator())
}

// buildBookingRow renders one tappable booking. The leading indicator
// is either the plain number (unlinked) or the circled number
// (linked); both are sized so a row visually shifts as soon as a
// linkage is created or removed.
func (a *App) buildBookingRow(
	b core.StatementBooking,
	onPick func(b core.StatementBooking),
) fyne.CanvasObject {
	linked := b.InvoiceRef != nil

	var indicator string
	if linked {
		indicator = circledNumber(b.LineIdx)
	} else {
		indicator = fmt.Sprintf("%d", b.LineIdx)
	}
	numLabel := canvas.NewText(indicator, color.NRGBA{R: 30, G: 80, B: 160, A: 255})
	numLabel.TextStyle = fyne.TextStyle{Bold: true}
	numLabel.TextSize = 16
	numLabel.Alignment = fyne.TextAlignTrailing
	numCell := container.NewGridWrap(fyne.NewSize(34, 22), numLabel)

	dateLbl := widget.NewLabelWithStyle(b.Date, fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})

	snippet := snippetAfterDate(b.Text)
	snippetLbl := widget.NewLabel(snippet)
	snippetLbl.Wrapping = fyne.TextWrapOff
	snippetLbl.Truncation = fyne.TextTruncateEllipsis

	body := container.NewBorder(nil, nil, container.NewHBox(numCell, dateLbl), nil, snippetLbl)

	card := newClickableCard(body, func() {
		if onPick != nil {
			onPick(b)
		}
	})
	return card
}

// snippetAfterDate returns the text right of the leading date, so we
// don't redundantly show "05.01.2026" twice in the row.
func snippetAfterDate(text string) string {
	t := strings.TrimSpace(text)
	if len(t) < 10 {
		return t
	}
	// Strip leading "DD.MM.YYYY " or "DD.MM. ".
	for _, prefixLen := range []int{10, 6} {
		if len(t) > prefixLen && t[2] == '.' && t[5] == '.' {
			return strings.TrimSpace(t[prefixLen:])
		}
	}
	return t
}

// onBookingTapped is the routing point for clicks in the booking
// sidebar. Step 3 of the rollout keeps this minimal: linked rows
// open the invoice; unlinked rows surface a toast so we can see the
// sidebar is wired, while step 4 will replace the toast with an
// invoice-picker dialog.
func (a *App) onBookingTapped(folder, statementRel string, b core.StatementBooking) {
	if b.InvoiceRef == nil {
		a.showToast(fmt.Sprintf("Buchung %d (S.%d) — Beleg-Auswahl folgt", b.LineIdx, b.Page+1))
		return
	}
	invoicePath := a.invoiceAbsPath(b.InvoiceRef)
	if invoicePath == "" {
		a.showToast("Verknüpfter Beleg nicht mehr gefunden.")
		return
	}
	a.openInvoiceForEdit(invoicePath)
}

// invoiceAbsPath resolves an InvoiceRef back to an absolute file path
// under the current storage root. The caller decides what to do if
// the file is missing.
func (a *App) invoiceAbsPath(ref *core.InvoiceRef) string {
	if ref == nil {
		return ""
	}
	return filepath.Join(a.settings.StorageRoot, ref.MonthFolder, ref.Filename)
}

// openInvoiceForEdit is a placeholder that surfaces a toast for now.
// Full Konten → Belege cross-jump (switching year/month and opening
// the row's edit dialog) lands in step 6.
func (a *App) openInvoiceForEdit(path string) {
	a.showToast("Öffnen aus Konten: kommt in Schritt 6")
	a.logger.Info("would open invoice for edit: %s", path)
}

// relFromStatementPath turns an absolute statement file path back into
// the bank-account-folder-relative key used in metadata.json. Returns
// "" if path is not inside folder.
func relFromStatementPath(folder, absPath string) string {
	folder = strings.TrimRight(folder, `/\`)
	if !strings.HasPrefix(absPath, folder) {
		return ""
	}
	rest := strings.TrimPrefix(absPath, folder)
	rest = strings.TrimLeft(rest, `/\`)
	return rest
}
