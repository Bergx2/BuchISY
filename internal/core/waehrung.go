package core

// ForeignConversion holds the EUR amounts derived from a foreign-currency
// payment plus its credit-card fee.
type ForeignConversion struct {
	BruttoEUR  float64
	NettoEUR   float64
	GebuehrEUR float64
	GesamtEUR  float64
}

// ConvertForeignPayment converts a foreign gross/net to EUR at the given rate
// (foreign units per EUR) and adds a percentage credit-card fee on the EUR
// gross. kurs <= 0 yields all-zero (no divide-by-zero).
func ConvertForeignPayment(bruttoForeign, nettoForeign, kurs, gebuehrProzent float64) ForeignConversion {
	if kurs <= 0 {
		return ForeignConversion{}
	}
	brutto := round2(bruttoForeign / kurs)
	netto := round2(nettoForeign / kurs)
	gebuehr := round2(brutto * gebuehrProzent / 100)
	return ForeignConversion{
		BruttoEUR:  brutto,
		NettoEUR:   netto,
		GebuehrEUR: gebuehr,
		GesamtEUR:  round2(brutto + gebuehr),
	}
}
