package core

// LineRef represents a single bank statement line for reconciliation purposes.
// Key is the line's BuchungRef string (uniquely identifies it).
type LineRef struct {
	Key           string
	Betrag        float64
	IstGutschrift bool
}

// ReconcileResult holds the reconciliation summary for a set of statement lines.
type ReconcileResult struct {
	LinesTotal   int
	LinesMatched int
	LinesOpen    int
	// OpenBelastung is the sum of unmatched debit line amounts.
	OpenBelastung float64
	// OpenGutschrift is the sum of unmatched credit line amounts.
	OpenGutschrift float64
}

// ReconcileSummary computes reconciliation statistics for a set of statement lines.
// A line is considered matched iff linked[line.Key] is true.
// Unmatched debit lines (IstGutschrift=false) contribute to OpenBelastung;
// unmatched credit lines (IstGutschrift=true) contribute to OpenGutschrift.
func ReconcileSummary(lines []LineRef, linked map[string]bool) ReconcileResult {
	var result ReconcileResult
	result.LinesTotal = len(lines)

	for _, line := range lines {
		if linked[line.Key] {
			result.LinesMatched++
		} else {
			result.LinesOpen++
			if line.IstGutschrift {
				result.OpenGutschrift += line.Betrag
			} else {
				result.OpenBelastung += line.Betrag
			}
		}
	}

	return result
}
