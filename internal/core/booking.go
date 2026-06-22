package core

import "fmt"

// InvoiceRef is a stable pointer from a statement booking back to a
// saved invoice (the main Beleg). The full invoice "Rechnung" — main
// PDF plus any attachments — is considered linked as a single unit, so
// only the main file's identity is stored here.
//
// MonthFolder is the storage-root-relative folder, e.g. "2026/2026-01"
// when month-subfolders are enabled. Filename is the basename of the
// invoice PDF inside that folder.
type InvoiceRef struct {
	MonthFolder string `json:"month_folder"`
	Filename    string `json:"filename"`
}

// String returns "MonthFolder/Filename".
func (r InvoiceRef) String() string {
	if r.MonthFolder == "" {
		return r.Filename
	}
	return r.MonthFolder + "/" + r.Filename
}

// StatementBooking is one detected transaction line on a bank
// statement page. The list is built by ParseStatementBookings and
// cached inside the StatementMetadata.
type StatementBooking struct {
	Page       int         `json:"page"`                  // 0-based PDF page
	LineIdx    int         `json:"line_idx"`              // 1-based, restarts per page
	Date       string      `json:"date"`                  // "DD.MM.YYYY" or "DD.MM."
	TopPt      float64     `json:"top_pt"`                // vertical position in PDF points
	LeftPt     float64     `json:"left_pt"`               // leftmost x position
	Text       string      `json:"text"`                  // full visible line text
	Betrag     float64     `json:"betrag,omitempty"`      // parsed absolute amount of the line
	IstGutschrift bool     `json:"gutschrift,omitempty"` // clearly an incoming credit (Haben)
	InvoiceRef *InvoiceRef `json:"invoice_ref,omitempty"` // nil = unlinked
}

// Display returns a short human label like "S.1 Z.3 — 14.01.2026".
func (b StatementBooking) Display() string {
	return fmt.Sprintf("S.%d Z.%d — %s", b.Page+1, b.LineIdx, b.Date)
}
