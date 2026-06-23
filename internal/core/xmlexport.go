package core

import "encoding/xml"

// BuildUStVAXML renders the UStVA as a structured, machine-readable XML of the
// official Kennzahlen (net bases + derived VAT + Zahllast) for the period. Not
// an ELSTER ERiC transmission — a clean export the tax advisor can process. Only
// non-zero Kennzahlen are emitted; Kz 83 is always emitted.
func BuildUStVAXML(u UStVAOfficial, zeitraum, ownVatID string) ([]byte, error) {
	type kz struct {
		Nr          string  `xml:"nr,attr"`
		Bezeichnung string  `xml:"bezeichnung,attr"`
		Wert        float64 `xml:"wert"`
	}
	type doc struct {
		XMLName   xml.Name `xml:"UmsatzsteuerVoranmeldung"`
		Zeitraum  string   `xml:"zeitraum,attr"`
		UStIdNr   string   `xml:"ust_idnr,attr,omitempty"`
		Kennzahl  []kz     `xml:"kennzahl"`
	}
	d := doc{Zeitraum: zeitraum, UStIdNr: ownVatID}
	add := func(nr, name string, wert float64, always bool) {
		if wert != 0 || always {
			d.Kennzahl = append(d.Kennzahl, kz{nr, name, round2(wert)})
		}
	}
	add("81", "Steuerpflichtige Umsätze 19 %", u.Kz81, false)
	add("86", "Steuerpflichtige Umsätze 7 %", u.Kz86, false)
	add("21", "Innergem. sonstige Leistungen (§ 18b UStG)", u.Kz21, false)
	add("45", "Übrige nicht steuerbare Umsätze (Ausland)", u.Kz45, false)
	add("84", "§ 13b Bemessungsgrundlage", u.Kz84, false)
	add("85", "§ 13b Steuer", u.Kz85, false)
	add("66", "Vorsteuer aus Rechnungen", u.Kz66, false)
	add("67", "Vorsteuer aus § 13b-Leistungen", u.Kz67, false)
	add("83", "Verbleibende Vorauszahlung / Überschuss", u.Kz83, true)

	out, err := xml.MarshalIndent(d, "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), out...), nil
}

// BuildZMXML renders the Zusammenfassende Meldung as structured XML: one
// Meldezeile per EU customer VAT-ID (net + Art der Leistung) plus the control
// total and the own VAT-ID.
func BuildZMXML(z ZM, zeitraum, ownVatID string) ([]byte, error) {
	type zeile struct {
		UStIdNr        string  `xml:"ust_idnr"`
		Summe          float64 `xml:"summe"`
		ArtDerLeistung string  `xml:"art_der_leistung"`
	}
	type doc struct {
		XMLName       xml.Name `xml:"ZusammenfassendeMeldung"`
		Zeitraum      string   `xml:"zeitraum,attr"`
		UStIdNr       string   `xml:"ust_idnr,attr,omitempty"`
		Kontrollsumme float64  `xml:"kontrollsumme"`
		Meldezeile    []zeile  `xml:"meldezeile"`
	}
	d := doc{Zeitraum: zeitraum, UStIdNr: ownVatID, Kontrollsumme: z.Kontrollsumme}
	for _, l := range z.Zeilen {
		d.Meldezeile = append(d.Meldezeile, zeile{UStIdNr: l.UStIdNr, Summe: l.Netto, ArtDerLeistung: "Sonstige Leistung"})
	}
	out, err := xml.MarshalIndent(d, "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), out...), nil
}
