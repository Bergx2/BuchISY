package core

import (
	"strings"
	"time"
)

// OpenItem represents a single open receivable (Forderung) or payable (Verbindlichkeit).
type OpenItem struct {
	Belegnummer     string
	Rechnungsnummer string
	Datum           string
	Partner         string
	Betrag          float64
	AgeDays         int
	Bucket          string // "0–30" | "31–60" | "61–90" | ">90"
}

// OpenItems holds the computed open receivables and payables with their totals.
type OpenItems struct {
	Forderungen             []OpenItem
	Verbindlichkeiten       []OpenItem
	ForderungenGesamt       float64
	VerbindlichkeitenGesamt float64
}

// agingBucket returns the aging bucket label for a given number of days.
func agingBucket(days int) string {
	switch {
	case days <= 30:
		return "0–30"
	case days <= 60:
		return "31–60"
	case days <= 90:
		return "61–90"
	default:
		return ">90"
	}
}

// ComputeOpenItems returns all open receivables (Forderungen) and payables
// (Verbindlichkeiten) from rows, evaluated as of asOf.
//
// An item is OPEN when Bezahldatum == "" AND BuchungRef == "".
// Open + Ausgangsrechnung == true  → Forderung  (receivable).
// Open + Ausgangsrechnung == false → Verbindlichkeit (payable).
// Betrag = Bruttobetrag. AgeDays = days from Rechnungsdatum to asOf (0 if
// Rechnungsdatum is empty or unparseable). Bucket boundaries: 0–30 / 31–60 /
// 61–90 / >90 days.
func ComputeOpenItems(rows []CSVRow, asOf time.Time) OpenItems {
	var oi OpenItems
	for _, r := range rows {
		if strings.TrimSpace(r.Bezahldatum) != "" || strings.TrimSpace(r.BuchungRef) != "" {
			continue
		}

		var ageDays int
		if t, err := time.Parse("02.01.2006", strings.TrimSpace(r.Rechnungsdatum)); err == nil {
			diff := asOf.Sub(t)
			if diff >= 0 {
				ageDays = int(diff.Hours() / 24)
			}
		}

		item := OpenItem{
			Belegnummer:     r.Belegnummer,
			Rechnungsnummer: r.Rechnungsnummer,
			Datum:           r.Rechnungsdatum,
			Partner:         r.Auftraggeber,
			Betrag:          r.Bruttobetrag,
			AgeDays:         ageDays,
			Bucket:          agingBucket(ageDays),
		}

		if r.Ausgangsrechnung {
			oi.Forderungen = append(oi.Forderungen, item)
			oi.ForderungenGesamt += item.Betrag
		} else {
			oi.Verbindlichkeiten = append(oi.Verbindlichkeiten, item)
			oi.VerbindlichkeitenGesamt += item.Betrag
		}
	}
	return oi
}
