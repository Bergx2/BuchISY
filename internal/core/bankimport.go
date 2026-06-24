package core

import (
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// DetectBankFormat inspects the raw bytes of a bank-statement file and returns
// the format string: "camt" for CAMT.053 XML, "mt940" for MT940 text, or ""
// when the format is not recognised.
func DetectBankFormat(data []byte) string {
	s := string(data)
	if strings.Contains(s, "<Document") && strings.Contains(s, "BkToCstmrStmt") {
		return "camt"
	}
	if strings.Contains(s, ":61:") {
		return "mt940"
	}
	return ""
}

// ParseCAMT053 parses a CAMT.053 XML bank statement and returns one
// StatementBooking per <Ntry> element. Namespaces are ignored — elements are
// matched by their local name only.
func ParseCAMT053(data []byte) ([]StatementBooking, error) {
	// We decode the XML token-by-token so we can match element local-names
	// without caring about namespace URIs (real-world CAMT files use many
	// namespace variants). A struct-based xml.Unmarshal with
	// `xml:"urn:...:Ntry"` tags would fail when the namespace differs.

	type ntryState struct {
		// accumulated from inner elements
		amt        string // raw text of <Amt>
		cdtDbtInd  string
		bookgDt    string // YYYY-MM-DD from BookgDt/Dt
		valDt      string // YYYY-MM-DD from ValDt/Dt
		addtlNtry  string
		ustrdParts []string

		// element-path tracking
		path    []string // current element stack (local names)
		inNtry  bool
	}

	dec := xml.NewDecoder(strings.NewReader(string(data)))
	dec.CharsetReader = func(charset string, input io.Reader) (io.Reader, error) {
		return input, nil // treat all charsets as UTF-8 for our purposes
	}

	var bookings []StatementBooking
	idx := 0

	var st ntryState

	localName := func(name xml.Name) string {
		return name.Local
	}

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("CAMT parse error: %w", err)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			ln := localName(t.Name)
			st.path = append(st.path, ln)
			if ln == "Ntry" {
				st = ntryState{inNtry: true, path: st.path}
			}

		case xml.EndElement:
			ln := localName(t.Name)
			if ln == "Ntry" && st.inNtry {
				// Build booking from accumulated state
				idx++
				date := st.bookgDt
				if date == "" {
					date = st.valDt
				}
				date = camtDateToGerman(date)

				text := strings.TrimSpace(st.addtlNtry)
				if text == "" && len(st.ustrdParts) > 0 {
					text = strings.Join(st.ustrdParts, " ")
				}

				betrag := parseCAMTAmount(st.amt)
				isCredit := st.cdtDbtInd == "CRDT"

				bookings = append(bookings, StatementBooking{
					Page:          0,
					LineIdx:       idx,
					Date:          date,
					Text:          text,
					Betrag:        betrag,
					IstGutschrift: isCredit,
				})
				// Reset state but keep path
				path := st.path
				if len(path) > 0 {
					path = path[:len(path)-1] // pop "Ntry"
				}
				st = ntryState{path: path}
				continue
			}
			if len(st.path) > 0 {
				st.path = st.path[:len(st.path)-1]
			}

		case xml.CharData:
			if !st.inNtry || len(st.path) == 0 {
				continue
			}
			text := strings.TrimSpace(string(t))
			if text == "" {
				continue
			}
			depth := len(st.path)
			if depth < 1 {
				continue
			}
			cur := st.path[depth-1]

			switch {
			case cur == "Amt":
				st.amt = text
			case cur == "CdtDbtInd":
				st.cdtDbtInd = text
			case cur == "Dt" && depth >= 3 && st.path[depth-2] == "BookgDt":
				if st.bookgDt == "" {
					st.bookgDt = text
				}
			case cur == "Dt" && depth >= 3 && st.path[depth-2] == "ValDt":
				if st.valDt == "" {
					st.valDt = text
				}
			case cur == "AddtlNtryInf":
				st.addtlNtry = text
			case cur == "Ustrd":
				st.ustrdParts = append(st.ustrdParts, text)
			}
		}
	}

	return bookings, nil
}

// camtDateToGerman converts "YYYY-MM-DD" → "DD.MM.YYYY". Returns the input
// unchanged when it doesn't match the expected format.
func camtDateToGerman(s string) string {
	s = strings.TrimSpace(s)
	if len(s) != 10 || s[4] != '-' || s[7] != '-' {
		return s
	}
	return s[8:10] + "." + s[5:7] + "." + s[0:4]
}

// parseCAMTAmount converts a CAMT amount string (dot decimal, e.g. "300.00")
// to float64. Returns 0 on error.
func parseCAMTAmount(s string) float64 {
	s = strings.TrimSpace(s)
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}

// ParseMT940 parses an MT940 bank statement text and returns one
// StatementBooking per :61: transaction line.
//
// The MT940 format uses:
//   - :61: <YYMMDD>[<YYMMDD>]<C|D|RC|RD><amount,decimal>...
//   - :86: purpose / narration text (one or more lines until the next tag or "-")
func ParseMT940(data []byte) ([]StatementBooking, error) {
	// Normalise line endings
	content := strings.ReplaceAll(string(data), "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	lines := strings.Split(content, "\n")

	// We collect tagged "fields": everything between :XX: markers is one field.
	type field struct {
		tag   string
		value string // possibly multi-line
	}

	var fields []field
	var cur *field

	for _, ln := range lines {
		if strings.HasPrefix(ln, ":") {
			// Check if this looks like a tag :XX: or :XXX:
			// ln[1:] has the content after the first colon.
			// end is the index of the second colon within ln[1:].
			end := strings.Index(ln[1:], ":")
			if end >= 0 && end <= 4 {
				// Flush previous field
				if cur != nil {
					fields = append(fields, *cur)
				}
				// tag is the content between the two colons (no colons included)
				tag := ln[1 : end+1] // e.g. "61", "86", "28C"
				// value starts after the second colon: position end+2 in ln
				value := ""
				if end+2 < len(ln) {
					value = strings.TrimSpace(ln[end+2:])
				}
				cur = &field{tag: tag, value: value}
				continue
			}
		}
		if ln == "-" {
			// MT940 block end
			if cur != nil {
				fields = append(fields, *cur)
				cur = nil
			}
			continue
		}
		if cur != nil && ln != "" {
			cur.value += "\n" + ln
		}
	}
	if cur != nil {
		fields = append(fields, *cur)
	}

	// Walk fields: :61: produces a booking; the next :86: provides the text.
	var bookings []StatementBooking
	idx := 0

	for i, f := range fields {
		if f.tag != "61" {
			continue
		}
		v := strings.TrimSpace(f.value)
		// :61: format: YYMMDD[YYMMDD]<C|D|RC|RD><amount><rest>
		if len(v) < 10 {
			continue // too short to hold a date + mark
		}
		dateStr := v[0:6] // YYMMDD
		rest := v[6:]

		// Skip optional entry date (MMDD = 4 digits) before the C/D mark.
		// We detect it by checking if the next chars before a C/D look like digits.
		// Strategy: scan past leading digits until we hit C, D, R.
		digitSkip := 0
		for digitSkip < len(rest) && rest[digitSkip] >= '0' && rest[digitSkip] <= '9' {
			digitSkip++
		}
		rest = rest[digitSkip:]

		// Credit/debit mark: RC, RD, C, D (check two-char first)
		var mark string
		if len(rest) >= 2 && (rest[0:2] == "RC" || rest[0:2] == "RD") {
			mark = rest[0:2]
			rest = rest[2:]
		} else if len(rest) >= 1 && (rest[0] == 'C' || rest[0] == 'D') {
			mark = rest[0:1]
			rest = rest[1:]
		} else {
			continue // cannot parse mark
		}

		// Amount: digits and comma until a non-digit/non-comma character
		amtEnd := 0
		for amtEnd < len(rest) && (rest[amtEnd] >= '0' && rest[amtEnd] <= '9' || rest[amtEnd] == ',') {
			amtEnd++
		}
		if amtEnd == 0 {
			continue
		}
		amtStr := rest[0:amtEnd]

		betrag := parseMT940Amount(amtStr)
		isCredit := strings.HasPrefix(mark, "C")
		date := mt940DateToGerman(dateStr)

		// Look for :86: right after this :61:
		text := ""
		if i+1 < len(fields) && fields[i+1].tag == "86" {
			text = strings.TrimSpace(fields[i+1].value)
			// Remove continuation line markers (some banks prefix with ?)
			// and collapse newlines to space
			text = strings.ReplaceAll(text, "\n", " ")
			text = strings.TrimSpace(text)
		}

		idx++
		bookings = append(bookings, StatementBooking{
			Page:          0,
			LineIdx:       idx,
			Date:          date,
			Text:          text,
			Betrag:        betrag,
			IstGutschrift: isCredit,
		})
	}

	return bookings, nil
}

// mt940DateToGerman converts "YYMMDD" → "DD.MM.20YY".
func mt940DateToGerman(s string) string {
	if len(s) != 6 {
		return s
	}
	yy := s[0:2]
	mm := s[2:4]
	dd := s[4:6]
	return dd + "." + mm + ".20" + yy
}

// parseMT940Amount converts a comma-decimal amount string (e.g. "300,00")
// to float64. Returns 0 on error.
func parseMT940Amount(s string) float64 {
	s = strings.ReplaceAll(s, ",", ".")
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	if v < 0 {
		v = -v
	}
	return v
}
