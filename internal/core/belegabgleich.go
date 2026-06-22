package core

import (
	"sort"
	"strings"
	"time"
)

// MatchKind classifies an invoice's reconciliation outcome.
type MatchKind int

const (
	MatchNone MatchKind = iota
	MatchSuggest
	MatchAuto
)

// ScoredLine is a candidate statement line with its rank score (higher = better).
type ScoredLine struct {
	Line  StatementBooking
	Score float64
}

// InvoiceEURAmount returns the amount that should appear on the statement: the
// gross in EUR. For a foreign-currency invoice that is the converted gross plus
// the credit-card fee; otherwise the Bruttobetrag.
func InvoiceEURAmount(row CSVRow) float64 {
	if row.Waehrung != "" && row.Waehrung != "EUR" && row.Wechselkurs > 0 {
		return round2(round2(row.Bruttobetrag/row.Wechselkurs) + row.Gebuehr)
	}
	return round2(row.Bruttobetrag)
}

// MatchInvoiceToStatement ranks the statement lines whose amount matches the
// invoice (within 0.01) by date proximity + supplier-name overlap, and
// classifies the outcome.
func MatchInvoiceToStatement(row CSVRow, lines []StatementBooking) (MatchKind, []ScoredLine) {
	amount := InvoiceEURAmount(row)
	if amount <= 0 {
		return MatchNone, nil
	}
	invDate := row.Bezahldatum
	if invDate == "" {
		invDate = row.Rechnungsdatum
	}
	nameTokens := tokenize(row.Auftraggeber)

	var cands []ScoredLine
	for _, l := range lines {
		if absf(l.Betrag-amount) > 0.01 {
			continue
		}
		days := dayDistance(invDate, l.Date)
		dateScore := 1.0 / (1.0 + float64(days)) // 0 days → 1.0, decays
		nameScore := tokenOverlap(nameTokens, tokenize(l.Text))
		cands = append(cands, ScoredLine{Line: l, Score: dateScore*2 + nameScore})
	}
	if len(cands) == 0 {
		return MatchNone, nil
	}
	sort.SliceStable(cands, func(i, j int) bool { return cands[i].Score > cands[j].Score })

	// Auto: exactly one amount-match, and it is within ±5 days.
	if len(cands) == 1 && dayDistance(invDate, cands[0].Line.Date) <= 5 {
		return MatchAuto, cands
	}
	return MatchSuggest, cands
}

func absf(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// tokenize lowercases and splits a string into word tokens of length >= 3.
func tokenize(s string) []string {
	fields := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && !(r >= 'ä' && r <= 'ÿ')
	})
	var out []string
	for _, f := range fields {
		if len(f) >= 3 {
			out = append(out, f)
		}
	}
	return out
}

// tokenOverlap returns the fraction of a's tokens that appear (as substring) in b.
func tokenOverlap(a, b []string) float64 {
	if len(a) == 0 {
		return 0
	}
	hit := 0
	for _, t := range a {
		for _, u := range b {
			if strings.Contains(u, t) || strings.Contains(t, u) {
				hit++
				break
			}
		}
	}
	return float64(hit) / float64(len(a))
}

// dayDistance returns the absolute day difference between two DD.MM.YYYY (or
// DD.MM.) dates; a missing/short year is treated as the other date's year. A
// huge number is returned when either is unparseable.
func dayDistance(a, b string) int {
	ta, oka := parseFlexDate(a, b)
	tb, okb := parseFlexDate(b, a)
	if !oka || !okb {
		return 9999
	}
	d := ta.Sub(tb).Hours() / 24
	if d < 0 {
		d = -d
	}
	return int(d + 0.5)
}

// parseFlexDate parses "DD.MM.YYYY" or "DD.MM." (taking the year from other).
func parseFlexDate(s, other string) (time.Time, bool) {
	parts := strings.Split(strings.TrimSpace(s), ".")
	if len(parts) < 2 {
		return time.Time{}, false
	}
	year := ""
	if len(parts) >= 3 {
		year = strings.TrimSpace(parts[2])
	}
	if year == "" {
		op := strings.Split(strings.TrimSpace(other), ".")
		if len(op) >= 3 {
			year = strings.TrimSpace(op[2])
		}
	}
	if len(year) == 2 {
		year = "20" + year
	}
	if year == "" {
		return time.Time{}, false
	}
	t, err := time.Parse("2.1.2006", parts[0]+"."+parts[1]+"."+year)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}
