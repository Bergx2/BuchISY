package core

import (
	"fmt"
	"strings"
)

// DATEVHeader carries the optional identifiers + period for an EXTF export.
type DATEVHeader struct {
	BeraterNr string
	MandantNr string
	WJBeginn  string // YYYYMMDD
	ErzeugtAm string // YYYYMMDDHHMMSSmmm
	DatumVon  string // YYYYMMDD
	DatumBis  string // YYYYMMDD
}

// datevAmount formats an amount with a comma decimal, unsigned, two decimals.
func datevAmount(v float64) string {
	return strings.Replace(fmt.Sprintf("%.2f", v), ".", ",", 1)
}

// datevBeleg converts a DD.MM.YYYY date to the DDMM Belegdatum form.
func datevBeleg(rechnungsdatum string) string {
	parts := strings.Split(rechnungsdatum, ".")
	if len(parts) < 2 {
		return ""
	}
	return parts[0] + parts[1]
}

func datevClean(s string, max int) string {
	s = strings.ReplaceAll(s, `"`, "")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > max {
		s = s[:max]
	}
	return s
}

// BuildDATEVStapel renders the bookings of rows as an EXTF Buchungsstapel.
// Returns the file bytes, the number of booking rows written, and the number
// of invoices skipped because they had no balanced booking.
func BuildDATEVStapel(h DATEVHeader, rows []CSVRow) ([]byte, int, int) {
	var b strings.Builder
	header := fmt.Sprintf(`"EXTF";700;21;"Buchungsstapel";13;%s;;;;;"%s";"%s";%s;4;%s;%s;"";"";"";"";0;"EUR";"";"";"";""`,
		h.ErzeugtAm, h.BeraterNr, h.MandantNr, h.WJBeginn, h.DatumVon, h.DatumBis)
	b.WriteString(header + "\r\n")
	b.WriteString(`Umsatz (ohne Soll/Haben-Kz);Soll/Haben-Kennzeichen;WKZ Umsatz;Kurs;Basis-Umsatz;WKZ Basis-Umsatz;Konto;Gegenkonto (ohne BU-Schlüssel);BU-Schlüssel;Belegdatum;Belegfeld 1;Belegfeld 2;Skonto;Buchungstext` + "\r\n")

	exported, skipped := 0, 0
	for _, r := range rows {
		pay, ok := r.Buchung.PaymentEntry()
		if !r.Buchung.Balanced() || !ok {
			skipped++
			continue
		}
		beleg := datevBeleg(r.Rechnungsdatum)
		belegfeld := datevClean(r.Rechnungsnummer, 36)
		text := datevClean(strings.TrimSpace(r.Auftraggeber+" "+r.Verwendungszweck), 60)
		for _, e := range r.Buchung.DebitEntries() {
			b.WriteString(fmt.Sprintf(`%s;"S";"EUR";;;;%d;%d;;%s;"%s";;;"%s"`+"\r\n",
				datevAmount(e.Betrag), e.Konto, pay.Konto, beleg, belegfeld, text))
			exported++
		}
	}
	return []byte(b.String()), exported, skipped
}
