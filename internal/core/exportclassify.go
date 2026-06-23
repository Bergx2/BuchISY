package core

// ExportSkip records why a receipt was left out of an export.
type ExportSkip struct {
	Dateiname string
	Grund     string
}

// ExportClassification splits a period's rows by export eligibility.
type ExportClassification struct {
	Exportable      []CSVRow
	AlreadyExported []CSVRow
	Skipped         []ExportSkip
}

// ClassifyForExport partitions rows into exportable, already-exported, and
// skipped (with a reason). An exportable row has a balanced booking with a
// valid payment/base entry (direction-aware). Already-exported exportable rows
// are added to Exportable only when includeExported is true.
func ClassifyForExport(rows []CSVRow, includeExported bool) ExportClassification {
	var c ExportClassification
	for _, r := range rows {
		_, _, ok := r.Buchung.PaymentAndCounters(r.Ausgangsrechnung)
		if !r.Buchung.Balanced() || !ok {
			grund := "nicht ausgeglichen"
			if len(r.Buchung.Entries) == 0 {
				grund = "keine Buchung"
			}
			c.Skipped = append(c.Skipped, ExportSkip{Dateiname: r.Dateiname, Grund: grund})
			continue
		}
		if r.Exportiert {
			c.AlreadyExported = append(c.AlreadyExported, r)
			if includeExported {
				c.Exportable = append(c.Exportable, r)
			}
			continue
		}
		c.Exportable = append(c.Exportable, r)
	}
	return c
}
