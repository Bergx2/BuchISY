package core

import (
	"encoding/json"
	"strings"
)

// TaxLine is one VAT line of a receipt: a net amount taxed at SatzProzent
// percent, yielding MwStBetrag of tax. A receipt may have several.
type TaxLine struct {
	Netto       float64 `json:"netto"`
	SatzProzent float64 `json:"satz_prozent"`
	MwStBetrag  float64 `json:"mwst_betrag"`
}

// SumNetto returns the total net of all lines.
func SumNetto(lines []TaxLine) float64 {
	var s float64
	for _, l := range lines {
		s += l.Netto
	}
	return s
}

// SumMwSt returns the total VAT of all lines.
func SumMwSt(lines []TaxLine) float64 {
	var s float64
	for _, l := range lines {
		s += l.MwStBetrag
	}
	return s
}

// ComputeBrutto returns net + vat over all lines plus the (un-taxed) Trinkgeld.
func ComputeBrutto(lines []TaxLine, trinkgeld float64) float64 {
	return SumNetto(lines) + SumMwSt(lines) + trinkgeld
}

// PrimarySatz returns the VAT rate of the first line (for the legacy
// SteuersatzProzent display field), or 0 when there are no lines.
func PrimarySatz(lines []TaxLine) float64 {
	if len(lines) == 0 {
		return 0
	}
	return lines[0].SatzProzent
}

// MarshalTaxLines encodes lines as compact JSON; empty input yields "".
func MarshalTaxLines(lines []TaxLine) string {
	if len(lines) == 0 {
		return ""
	}
	b, err := json.Marshal(lines)
	if err != nil {
		return ""
	}
	return string(b)
}

// ParseTaxLines decodes lines from JSON; "" or invalid input yields nil.
func ParseTaxLines(s string) []TaxLine {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var lines []TaxLine
	if err := json.Unmarshal([]byte(s), &lines); err != nil {
		return nil
	}
	return lines
}

// ReconstructTaxLines builds a single TaxLine from the legacy aggregate
// fields, used when a row has no Steuerzeilen detail. Returns nil if the
// aggregates are all zero.
func ReconstructTaxLines(netto, satzProzent, mwst float64) []TaxLine {
	if netto == 0 && satzProzent == 0 && mwst == 0 {
		return nil
	}
	return []TaxLine{{Netto: netto, SatzProzent: satzProzent, MwStBetrag: mwst}}
}
