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
// EUR gross (converted via RowEUR) plus the EUR CC/FX fee (Gebuehr is already in
// EUR — RowEUR keeps it) minus any discount. For a foreign row with a missing
// rate the face-value gross + fee − rabatt is returned.
func InvoiceEURAmount(row CSVRow) float64 {
	eurRow, _ := RowEUR(row)
	return round2(eurRow.Bruttobetrag + eurRow.Gebuehr - eurRow.Rabatt)
}

// MatchConfig tunes the matcher.
type MatchConfig struct {
	DateWindowDays      int                 // auto-link only within this many days
	ForeignTolerancePct float64             // amount tolerance for non-EUR invoices (percent)
	Aliases             map[string][]string // lowercase supplier → learned statement tokens
}

// DefaultMatchConfig returns sensible defaults.
func DefaultMatchConfig() MatchConfig {
	return MatchConfig{DateWindowDays: 5, ForeignTolerancePct: 1.5}
}

// matchToStatement ranks statement lines by amount + date proximity + supplier-name overlap.
// wantCredit: if true, matches INCOMING credits (IstGutschrift=true); if false, matches DEBIT lines (IstGutschrift=false).
// cfg controls date window, foreign-currency tolerance, and alias token boosts.
// Returns the outcome classification and candidate lines sorted by score (highest first).
func matchToStatement(row CSVRow, lines []StatementBooking, cfg MatchConfig, wantCredit bool) (MatchKind, []ScoredLine) {
	amount := InvoiceEURAmount(row)
	if amount <= 0 {
		return MatchNone, nil
	}
	// Amount tolerance: strict for EUR; percentage band for foreign (rate drift).
	tol := 0.01
	if row.Waehrung != "" && row.Waehrung != "EUR" && cfg.ForeignTolerancePct > 0 {
		if band := amount * cfg.ForeignTolerancePct / 100; band > tol {
			tol = band
		}
	}
	invDate := row.Bezahldatum
	if invDate == "" {
		invDate = row.Rechnungsdatum
	}
	nameTokens := tokenize(row.Auftraggeber)
	aliasTokens := cfg.Aliases[strings.ToLower(strings.TrimSpace(row.Auftraggeber))]
	window := cfg.DateWindowDays
	if window <= 0 {
		window = 5
	}

	var cands []ScoredLine
	for _, l := range lines {
		if l.IstGutschrift != wantCredit { // skip lines not matching the desired type
			continue
		}
		if absf(l.Betrag-amount) > tol {
			continue
		}
		days := dayDistance(invDate, l.Date)
		dateScore := 1.0 / (1.0 + float64(days)) // 0 days → 1.0, decays
		lineTokens := tokenize(l.Text)
		nameScore := tokenOverlap(nameTokens, lineTokens)
		if a := tokenOverlap(aliasTokens, lineTokens); a > nameScore {
			nameScore = a // learned alias can rescue a no-shared-word supplier
		}
		cands = append(cands, ScoredLine{Line: l, Score: dateScore*2 + nameScore})
	}
	if len(cands) == 0 {
		return MatchNone, nil
	}
	sort.SliceStable(cands, func(i, j int) bool { return cands[i].Score > cands[j].Score })

	// Auto: exactly one amount-match, and it is within the configured window.
	if len(cands) == 1 && dayDistance(invDate, cands[0].Line.Date) <= window {
		return MatchAuto, cands
	}
	return MatchSuggest, cands
}

// MatchInvoiceToStatement matches an expense invoice to statement DEBIT lines
// (credits excluded). cfg controls window, foreign tolerance, alias boosts.
func MatchInvoiceToStatement(row CSVRow, lines []StatementBooking, cfg MatchConfig) (MatchKind, []ScoredLine) {
	return matchToStatement(row, lines, cfg, false)
}

// MatchRevenueToStatement matches an outgoing invoice (Ausgangsrechnung) to
// incoming bank CREDIT lines (Gutschriften) by amount + date + customer-name
// overlap — the mirror of MatchInvoiceToStatement.
func MatchRevenueToStatement(row CSVRow, lines []StatementBooking, cfg MatchConfig) (MatchKind, []ScoredLine) {
	return matchToStatement(row, lines, cfg, true)
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

// GroupMatch holds one n:1 grouped-payment result: N invoice filenames summing
// to a single statement line's Betrag.
type GroupMatch struct {
	Dateinamen []string         // invoice Dateiname values in the group
	Line       StatementBooking // the statement line that matches the sum
	File       string           // source statement file (filled by the caller)
}

// findGroupedPayments is the internal implementation of grouped-payment matching.
// wantCredit: if false, matches DEBIT lines (IstGutschrift=false); if true, matches
// INCOMING CREDIT lines (IstGutschrift=true).
// Finds statement lines whose Betrag equals the sum of 2 or 3 invoices within
// cfg.DateWindowDays of that line. Only invoices with InvoiceEURAmount > 0 are
// considered. Returns one disjoint group per line (first match wins); invoices are
// not reused across groups. File is left empty — the caller fills it from the
// statement cache.
func findGroupedPayments(invoices []CSVRow, lines []StatementBooking, cfg MatchConfig, wantCredit bool) []GroupMatch {
	window := cfg.DateWindowDays
	if window <= 0 {
		window = 5
	}

	usedFilenames := map[string]bool{}
	var results []GroupMatch

	for _, l := range lines {
		if l.IstGutschrift != wantCredit || l.Betrag <= 0 {
			continue
		}
		// Build windowed candidate list for this line.
		var candidates []CSVRow
		for _, inv := range invoices {
			if usedFilenames[inv.Dateiname] {
				continue
			}
			amt := InvoiceEURAmount(inv)
			if amt <= 0 {
				continue
			}
			// Date proximity check.
			invDate := inv.Bezahldatum
			if invDate == "" {
				invDate = inv.Rechnungsdatum
			}
			if dayDistance(invDate, l.Date) > window {
				continue
			}
			candidates = append(candidates, inv)
		}

		// Search pairs (size 2).
		found := false
		for i := 0; i < len(candidates) && !found; i++ {
			for j := i + 1; j < len(candidates) && !found; j++ {
				sum := round2(InvoiceEURAmount(candidates[i]) + InvoiceEURAmount(candidates[j]))
				if absf(sum-l.Betrag) <= 0.01 {
					names := []string{candidates[i].Dateiname, candidates[j].Dateiname}
					results = append(results, GroupMatch{Dateinamen: names, Line: l})
					usedFilenames[candidates[i].Dateiname] = true
					usedFilenames[candidates[j].Dateiname] = true
					found = true
				}
			}
		}
		if found {
			continue
		}

		// Search triples (size 3).
		for i := 0; i < len(candidates) && !found; i++ {
			for j := i + 1; j < len(candidates) && !found; j++ {
				for k := j + 1; k < len(candidates) && !found; k++ {
					sum := round2(InvoiceEURAmount(candidates[i]) + InvoiceEURAmount(candidates[j]) + InvoiceEURAmount(candidates[k]))
					if absf(sum-l.Betrag) <= 0.01 {
						names := []string{candidates[i].Dateiname, candidates[j].Dateiname, candidates[k].Dateiname}
						results = append(results, GroupMatch{Dateinamen: names, Line: l})
						usedFilenames[candidates[i].Dateiname] = true
						usedFilenames[candidates[j].Dateiname] = true
						usedFilenames[candidates[k].Dateiname] = true
						found = true
					}
				}
			}
		}
	}

	return results
}

// FindGroupedPayments finds statement DEBIT lines (non-credit) whose Betrag equals the
// sum of 2 or 3 invoices within cfg.DateWindowDays of that line. Only invoices
// with InvoiceEURAmount > 0 are considered. Returns one disjoint group per line
// (first match wins); invoices are not reused across groups. File is left empty
// — the caller fills it from the statement cache.
func FindGroupedPayments(invoices []CSVRow, lines []StatementBooking, cfg MatchConfig) []GroupMatch {
	return findGroupedPayments(invoices, lines, cfg, false)
}

// FindGroupedRevenuePayments finds statement CREDIT lines (Gutschriften) whose Betrag
// equals the sum of 2 or 3 outgoing invoices (Ausgangsrechnungen) within cfg.DateWindowDays
// of that line. Returns one disjoint group per credit line (first match wins); invoices
// are not reused across groups. File is left empty — the caller fills it from the
// statement cache.
func FindGroupedRevenuePayments(invoices []CSVRow, lines []StatementBooking, cfg MatchConfig) []GroupMatch {
	return findGroupedPayments(invoices, lines, cfg, true)
}

// SplitMatch is the inverse of GroupMatch: ONE invoice whose EUR gross equals
// the SUM of several statement lines (e.g. a monthly bank-fee statement settled
// as separate per-transaction debits). File is filled by the caller from the
// statement cache.
type SplitMatch struct {
	Dateiname string
	File      string
	Lines     []StatementBooking
}

// splitWindowDays bounds how far statement lines may sit from the invoice date
// for a split match. Generous (a billing period plus slack) because the
// individual charges of a periodic statement accrue across the whole month,
// far from the statement's own date — unlike a 1:1 payment.
const splitWindowDays = 62

// maxSplitLines caps the subset size searched, keeping the subset-sum bounded.
const maxSplitLines = 8

// FindSplitPayments finds, for each invoice, a set of 2..maxSplitLines statement
// DEBIT lines whose Betrag sum equals the invoice's EUR gross, within
// splitWindowDays of the invoice date. One disjoint result per invoice (first
// subset found wins); lines are not reused across invoices. The mirror of
// FindGroupedPayments (which sums invoices to one line). File is left empty —
// the caller fills it from the statement cache.
func FindSplitPayments(invoices []CSVRow, lines []StatementBooking, cfg MatchConfig) []SplitMatch {
	usedLine := map[[2]int]bool{} // (page,lineIdx) already claimed by an earlier split
	var results []SplitMatch

	for _, inv := range invoices {
		target := InvoiceEURAmount(inv)
		if target <= 0 {
			continue
		}
		invDate := inv.Bezahldatum
		if invDate == "" {
			invDate = inv.Rechnungsdatum
		}

		// Candidate debit lines within the window, not already used.
		var cand []StatementBooking
		for _, l := range lines {
			if l.IstGutschrift || l.Betrag <= 0 {
				continue
			}
			if usedLine[[2]int{l.Page, l.LineIdx}] {
				continue
			}
			if dayDistance(invDate, l.Date) > splitWindowDays {
				continue
			}
			cand = append(cand, l)
		}
		// Need at least 2 lines to be a "split" (a single line is a 1:1 match).
		if len(cand) < 2 {
			continue
		}

		if subset, ok := subsetSum(cand, target, maxSplitLines); ok {
			results = append(results, SplitMatch{Dateiname: inv.Dateiname, Lines: subset})
			for _, l := range subset {
				usedLine[[2]int{l.Page, l.LineIdx}] = true
			}
		}
	}
	return results
}

// subsetSum returns a subset of size 2..maxK whose Betrag sums to target (±0.01),
// found via depth-first search with sum pruning. Returns the first such subset
// (deterministic by input order) or ok=false.
func subsetSum(lines []StatementBooking, target float64, maxK int) ([]StatementBooking, bool) {
	var chosen []StatementBooking
	var dfs func(start int, sum float64) bool
	dfs = func(start int, sum float64) bool {
		if len(chosen) >= 2 && absf(sum-target) <= 0.01 {
			return true
		}
		if len(chosen) >= maxK || sum > target+0.01 {
			return false
		}
		for i := start; i < len(lines); i++ {
			chosen = append(chosen, lines[i])
			if dfs(i+1, round2(sum+lines[i].Betrag)) {
				return true
			}
			chosen = chosen[:len(chosen)-1]
		}
		return false
	}
	if dfs(0, 0) {
		out := make([]StatementBooking, len(chosen))
		copy(out, chosen)
		return out, true
	}
	return nil, false
}

// partialPaymentLines is the internal implementation of partial-payment matching.
// wantCredit: if false, matches DEBIT lines (IstGutschrift=false); if true, matches
// INCOMING CREDIT lines (IstGutschrift=true).
// Returns statement lines that look like a partial payment for the given invoice
// (Teilzahlung=true). Only lines matching wantCredit with 0 < Betrag < InvoiceEURAmount(row)-0.01
// are returned, ranked by date proximity to the invoice's Bezahldatum (or Rechnungsdatum).
func partialPaymentLines(row CSVRow, lines []StatementBooking, wantCredit bool) []ScoredLine {
	if !row.Teilzahlung {
		return nil
	}
	fullAmt := InvoiceEURAmount(row)
	invDate := row.Bezahldatum
	if invDate == "" {
		invDate = row.Rechnungsdatum
	}

	var cands []ScoredLine
	for _, l := range lines {
		if l.IstGutschrift != wantCredit {
			continue
		}
		if l.Betrag <= 0 || l.Betrag >= fullAmt-0.01 {
			continue
		}
		days := dayDistance(invDate, l.Date)
		score := 1.0 / (1.0 + float64(days))
		cands = append(cands, ScoredLine{Line: l, Score: score})
	}
	sort.SliceStable(cands, func(i, j int) bool { return cands[i].Score > cands[j].Score })
	return cands
}

// PartialPaymentLines returns statement DEBIT lines that look like a partial payment for
// the given invoice (Teilzahlung=true). Only non-credit lines with
// 0 < Betrag < InvoiceEURAmount(row)-0.01 are returned, ranked by date proximity
// to the invoice's Bezahldatum (or Rechnungsdatum).
func PartialPaymentLines(row CSVRow, lines []StatementBooking) []ScoredLine {
	return partialPaymentLines(row, lines, false)
}

// RevenuePartialPaymentLines returns statement CREDIT lines that look like a partial
// payment for the given outgoing invoice (Ausgangsrechnung, Teilzahlung=true).
// Only incoming credit lines with 0 < Betrag < InvoiceEURAmount(row)-0.01 are
// returned, ranked by date proximity to the invoice's Bezahldatum (or Rechnungsdatum).
func RevenuePartialPaymentLines(row CSVRow, lines []StatementBooking) []ScoredLine {
	return partialPaymentLines(row, lines, true)
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
