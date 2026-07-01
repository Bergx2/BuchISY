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

// buchungRefSep separates multiple statement-line references in a single
// CSVRow.BuchungRef value (1 receipt → N statement lines, e.g. a bank-fee
// statement settled as several separate debits). A value with no separator is
// a single ref and parses exactly as before — fully backward compatible.
const buchungRefSep = ";"

// ParseBuchungRefs parses one or more refs from a BuchungRef value. A plain
// single ref yields a one-element slice; an empty/garbage value yields nil.
func ParseBuchungRefs(s string) []BuchungRef {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var out []BuchungRef
	for _, part := range strings.Split(s, buchungRefSep) {
		if r := ParseBuchungRef(part); !r.IsZero() {
			out = append(out, r)
		}
	}
	return out
}

// JoinBuchungRefs renders multiple refs into the wire format. Zero refs are
// skipped; the result is "" when nothing remains.
func JoinBuchungRefs(refs []BuchungRef) string {
	var parts []string
	for _, r := range refs {
		if !r.IsZero() {
			parts = append(parts, r.String())
		}
	}
	return strings.Join(parts, buchungRefSep)
}

// FirstBuchungRef returns the first parsed ref (or zero), for callers that only
// need one line (e.g. "open the statement at this line").
func FirstBuchungRef(s string) BuchungRef {
	refs := ParseBuchungRefs(s)
	if len(refs) == 0 {
		return BuchungRef{}
	}
	return refs[0]
}

// BuchungRefDisplay is the multi-ref-aware version of BuchungRef.Display(): a
// single ref reads as before; several refs read "<file> · N Zeilen: S.1 Z.3, …".
func BuchungRefDisplay(s string) string {
	refs := ParseBuchungRefs(s)
	switch len(refs) {
	case 0:
		return ""
	case 1:
		return refs[0].Display()
	default:
		short := make([]string, len(refs))
		for i, r := range refs {
			short[i] = fmt.Sprintf("S.%d Z.%d", r.Page+1, r.LineIdx)
		}
		return fmt.Sprintf("%s · %d Zeilen: %s", refs[0].StatementFilename, len(refs), strings.Join(short, ", "))
	}
}

// StatementParserVersion identifies the current statement-parsing logic. BUMP
// IT whenever the parser (ParseStatementBookings and friends) changes so that
// already-cached statement bookings are re-parsed automatically — otherwise a
// parser fix would not reach statements whose mtime is unchanged.
const StatementParserVersion = 2

// statementCacheStale reports whether meta.Bookings must be re-parsed: the file
// changed, nothing is cached yet, or the cache was produced by an older parser.
func statementCacheStale(meta *StatementMetadata, mtime int64) bool {
	return meta.BookingsParsedMtime != mtime ||
		len(meta.Bookings) == 0 ||
		meta.BookingsParserVersion != StatementParserVersion
}

// EnsureBookingsParsed makes sure StatementMetadata.Bookings is current for the
// given PDF. If the file's mtime and the parser version both match the cache,
// nothing happens; otherwise the parser runs and the result is stored back into
// meta (the caller is responsible for persisting it).
//
// Returns true when meta was modified.
func EnsureBookingsParsed(path string, meta *StatementMetadata) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, fmt.Errorf("stat statement PDF: %w", err)
	}
	mtime := info.ModTime().Unix()
	if !statementCacheStale(meta, mtime) {
		return false, nil
	}
	parsed, err := ParseStatementBookings(path)
	if err != nil {
		return false, err
	}
	reattachInvoiceRefs(meta.Bookings, parsed)
	meta.Bookings = parsed
	meta.BookingsParsedMtime = mtime
	meta.BookingsParserVersion = StatementParserVersion
	return true, nil
}

// reattachInvoiceRefs copies each old booking's InvoiceRef onto the re-parsed
// booking with the SAME STABLE IDENTITY — page + amount (cents) + date — rather
// than the same line index. A parser change that adds or drops a booking shifts
// line indices, so index-matching would silently move a link to the wrong
// booking; the amount+date identity is stable across such changes (and works for
// every parser, including Qonto, which carries no Y position). Duplicates of one
// identity are matched in order so two identical same-day debits keep their
// respective links.
func reattachInvoiceRefs(old, parsed []StatementBooking) {
	if len(old) == 0 {
		return
	}
	type ident struct {
		page  int
		cents int64
		date  string
	}
	key := func(b StatementBooking) ident {
		return ident{page: b.Page, cents: int64(b.Betrag*100 + 0.5), date: b.Date}
	}
	byID := make(map[ident][]*InvoiceRef)
	for i := range old {
		if old[i].InvoiceRef != nil {
			k := key(old[i])
			byID[k] = append(byID[k], old[i].InvoiceRef)
		}
	}
	for i := range parsed {
		k := key(parsed[i])
		if q := byID[k]; len(q) > 0 {
			parsed[i].InvoiceRef = q[0]
			byID[k] = q[1:]
		}
	}
}
