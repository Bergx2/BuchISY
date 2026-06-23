package core

// UStVAOfficial is the VAT return in the official ELSTER Kennzahlen, computed
// from invoice metadata. Net bases (Kz81/86/21/45/84) plus the derived output
// VAT and the Zahllast (Kz83). Structured to feed an ELSTER export later.
type UStVAOfficial struct {
	Kz81  float64 // steuerpflichtige Umsätze 19 % (Bemessungsgrundlage, netto)
	Kz86  float64 // steuerpflichtige Umsätze 7 % (Bemessungsgrundlage, netto)
	Kz21  float64 // nicht steuerbare innergem. sonstige Leistungen (§ 18b), netto
	Kz45  float64 // übrige nicht steuerbare Umsätze (Leistungsort nicht im Inland), netto
	Kz84  float64 // § 13b Bemessungsgrundlage (netto)
	Kz85  float64 // § 13b Steuer
	Kz66  float64 // Vorsteuer aus Rechnungen anderer Unternehmer
	Kz67  float64 // Vorsteuer aus § 13b Leistungen
	USt81 float64 // = Kz81 × 19 % (derived)
	USt86 float64 // = Kz86 × 7 % (derived)
	Kz83  float64 // Zahllast/Überschuss = (USt81+USt86+Kz85) − (Kz66+Kz67)
}

// ComputeUStVAOfficial classifies each invoice into its Kennzahl from the
// Ausgangsrechnung flag, the tax lines, and the counterparty VAT-ID.
func ComputeUStVAOfficial(rows []CSVRow, rules *BookingRules) UStVAOfficial {
	rcSatz := 19.0
	if rc, ok := rules.Rule("reverse_charge"); ok && rc.RcSatz > 0 {
		rcSatz = rc.RcSatz
	}
	var u UStVAOfficial
	for _, r := range rows {
		net := SumNetto(r.TaxLines)
		vat := SumMwSt(r.TaxLines)
		if r.Ausgangsrechnung {
			if vat > 0.005 { // domestic taxable sale
				for _, l := range r.TaxLines {
					switch int(l.SatzProzent + 0.5) {
					case 19:
						u.Kz81 += l.Netto
					case 7:
						u.Kz86 += l.Netto
					}
				}
			} else if IsEUVatID(r.VATID) {
				u.Kz21 += net
			} else {
				u.Kz45 += net
			}
		} else { // incoming
			if IsEUVatID(r.VATID) && vat < 0.005 { // § 13b reverse-charge
				u.Kz84 += net
			} else {
				u.Kz66 += vat
			}
		}
	}
	u.Kz81 = round2(u.Kz81)
	u.Kz86 = round2(u.Kz86)
	u.Kz21 = round2(u.Kz21)
	u.Kz45 = round2(u.Kz45)
	u.Kz84 = round2(u.Kz84)
	u.Kz66 = round2(u.Kz66)
	u.Kz85 = round2(u.Kz84 * rcSatz / 100)
	u.Kz67 = u.Kz85
	u.USt81 = round2(u.Kz81 * 0.19)
	u.USt86 = round2(u.Kz86 * 0.07)
	u.Kz83 = round2((u.USt81 + u.USt86 + u.Kz85) - (u.Kz66 + u.Kz67))
	return u
}
