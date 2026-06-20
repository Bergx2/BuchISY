package core

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
