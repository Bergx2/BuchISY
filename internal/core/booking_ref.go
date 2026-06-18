package core

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// BuchungRef is the parsed form of a CSVRow.BuchungRef string. The
// wire format is "<statementFilename>|<page>|<lineIdx>" where page is
// 0-based and lineIdx is 1-based, exactly as carried in
// StatementBooking.
type BuchungRef struct {
	StatementFilename string
	Page              int
	LineIdx           int
}

// String returns the wire format.
func (r BuchungRef) String() string {
	if r.StatementFilename == "" {
		return ""
	}
	return fmt.Sprintf("%s|%d|%d", r.StatementFilename, r.Page, r.LineIdx)
}

// IsZero reports whether the ref is unset.
func (r BuchungRef) IsZero() bool {
	return r.StatementFilename == ""
}

// Display returns a short user-facing label like
// "Konto_...0002.pdf · S.1 Z.3" suitable for tooltips and table cells.
func (r BuchungRef) Display() string {
	if r.IsZero() {
		return ""
	}
	return fmt.Sprintf("%s · S.%d Z.%d", r.StatementFilename, r.Page+1, r.LineIdx)
}

// ParseBuchungRef parses the wire format. Returns zero value when the
// string is empty or malformed (we don't fail loudly because legacy
// CSVs may have garbage in this slot).
func ParseBuchungRef(s string) BuchungRef {
	s = strings.TrimSpace(s)
	if s == "" {
		return BuchungRef{}
	}
	parts := strings.Split(s, "|")
	if len(parts) != 3 {
		return BuchungRef{}
	}
	page, err1 := strconv.Atoi(parts[1])
	line, err2 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil {
		return BuchungRef{}
	}
	return BuchungRef{
		StatementFilename: parts[0],
		Page:              page,
		LineIdx:           line,
	}
}

// EnsureBookingsParsed makes sure StatementMetadata.Bookings is current
// for the given PDF. If the file's mtime hasn't changed since the last
// parse, nothing happens; otherwise the parser runs and the result is
// stored back into meta (the caller is responsible for persisting it).
//
// Returns true when meta was modified.
func EnsureBookingsParsed(path string, meta *StatementMetadata) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, fmt.Errorf("stat statement PDF: %w", err)
	}
	mtime := info.ModTime().Unix()
	if meta.BookingsParsedMtime == mtime && len(meta.Bookings) > 0 {
		return false, nil
	}
	parsed, err := ParseStatementBookings(path)
	if err != nil {
		return false, err
	}
	// Preserve any existing InvoiceRef linkage by matching on (page,lineIdx).
	if len(meta.Bookings) > 0 {
		oldByKey := make(map[[2]int]*InvoiceRef, len(meta.Bookings))
		for _, old := range meta.Bookings {
			if old.InvoiceRef != nil {
				oldByKey[[2]int{old.Page, old.LineIdx}] = old.InvoiceRef
			}
		}
		for i := range parsed {
			if ref, ok := oldByKey[[2]int{parsed[i].Page, parsed[i].LineIdx}]; ok {
				parsed[i].InvoiceRef = ref
			}
		}
	}
	meta.Bookings = parsed
	meta.BookingsParsedMtime = mtime
	return true, nil
}
