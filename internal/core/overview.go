package core

// OverviewKPI aggregates per-month (or per-year) KPIs for the overview dialog.
type OverviewKPI struct {
	Count int
	Netto float64
	USt   float64
	Brutto float64
	// Zahllast is a rough indicator of VAT payable:
	// Σ SteuersatzBetrag on outgoing invoices (Ausgangsrechnung=true)
	// minus Σ SteuersatzBetrag on incoming invoices (Ausgangsrechnung=false).
	// This is an approximation — actual Zahllast is determined by the UStVA.
	Zahllast float64
	// OpenReconcile counts rows that have a non-empty Bankkonto but an empty
	// BuchungRef. These are invoices linked to a bank/credit-card account that
	// have not yet been matched to a bank statement line.
	OpenReconcile int
	// Warnings counts rows for which InvoiceWarnings returns at least one entry.
	Warnings int
}

// OverviewKPIs computes aggregated KPIs for a slice of invoice rows.
func OverviewKPIs(rows []CSVRow) OverviewKPI {
	var kpi OverviewKPI
	kpi.Count = len(rows)
	for _, r := range rows {
		kpi.Netto += r.BetragNetto
		kpi.USt += r.SteuersatzBetrag
		kpi.Brutto += r.Bruttobetrag

		if r.Ausgangsrechnung {
			kpi.Zahllast += r.SteuersatzBetrag
		} else {
			kpi.Zahllast -= r.SteuersatzBetrag
		}

		if r.Bankkonto != "" && r.BuchungRef == "" {
			kpi.OpenReconcile++
		}

		if len(InvoiceWarnings(r)) > 0 {
			kpi.Warnings++
		}
	}
	return kpi
}
