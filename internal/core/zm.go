package core

import (
	"sort"
	"strings"
)

// euVatPrefixes are the 2-letter country codes of EU member states that prefix a
// USt-IdNr (Greece uses "EL"). "DE" is intentionally excluded — a domestic
// customer is never a ZM counterparty.
var euVatPrefixes = map[string]bool{
	"AT": true, "BE": true, "BG": true, "CY": true, "CZ": true, "DK": true,
	"EE": true, "EL": true, "ES": true, "FI": true, "FR": true, "HR": true,
	"HU": true, "IE": true, "IT": true, "LT": true, "LU": true, "LV": true,
	"MT": true, "NL": true, "PL": true, "PT": true, "RO": true, "SE": true,
	"SI": true, "SK": true,
}

// IsEUVatID reports whether s looks like an EU VAT-ID of another member state
// (2-letter EU prefix, not DE, plus at least one following character).
func IsEUVatID(s string) bool {
	s = strings.ToUpper(strings.TrimSpace(s))
	if len(s) < 3 {
		return false
	}
	return euVatPrefixes[s[:2]]
}

// ZMZeile is one ZM line: a customer's EU VAT-ID and the summed net supplies.
type ZMZeile struct {
	UStIdNr string
	Netto   float64
}

// ZM is the Zusammenfassende Meldung for a period: one line per EU customer
// VAT-ID plus the control total.
type ZM struct {
	Zeilen        []ZMZeile
	Kontrollsumme float64
}

// ComputeZM sums the net of intra-EU reverse-charge supplies (outgoing invoices
// to an EU customer VAT-ID with no VAT charged), grouped per customer VAT-ID.
func ComputeZM(rows []CSVRow) ZM {
	rows = RowsEUR(rows) // EU reverse-charge sales must be reported in EUR
	byVat := map[string]float64{}
	for _, r := range rows {
		if !r.Ausgangsrechnung || !IsEUVatID(r.VATID) || SumMwSt(r.TaxLines) != 0 {
			continue
		}
		byVat[strings.ToUpper(strings.TrimSpace(r.VATID))] += SumNetto(r.TaxLines)
	}
	var z ZM
	for vat, netto := range byVat {
		netto = round2(netto)
		z.Zeilen = append(z.Zeilen, ZMZeile{UStIdNr: vat, Netto: netto})
		z.Kontrollsumme += netto
	}
	sort.Slice(z.Zeilen, func(i, j int) bool { return z.Zeilen[i].UStIdNr < z.Zeilen[j].UStIdNr })
	z.Kontrollsumme = round2(z.Kontrollsumme)
	return z
}
