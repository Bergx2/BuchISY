package core

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/go-pdf/fpdf"
)

// newReportPDF starts an A4 report with a bold title and returns the document
// plus a cp1252 translator (so ä/ö/ü/ß/€ render with the core Arial font).
// orientation is "P" (portrait) or "L" (landscape).
func newReportPDF(title, orientation string) (*fpdf.Fpdf, func(string) string) {
	pdf := fpdf.New(orientation, "mm", "A4", "")
	tr := pdf.UnicodeTranslatorFromDescriptor("") // cp1252
	pdf.SetTitle(title, true)
	pdf.AddPage()
	pdf.SetFont("Arial", "B", 14)
	pdf.CellFormat(0, 10, tr(title), "", 1, "L", false, 0, "")
	pdf.SetFont("Arial", "", 9)
	pdf.Ln(1)
	return pdf, tr
}

// pdfAmount formats an amount with a German decimal comma.
func pdfAmount(v float64) string {
	return strings.Replace(fmt.Sprintf("%.2f", v), ".", ",", 1)
}

// pdfTableHeader draws the bold column-header row.
func pdfTableHeader(pdf *fpdf.Fpdf, tr func(string) string, headers []string, widths []float64) {
	pdf.SetFont("Arial", "B", 9)
	for i, h := range headers {
		pdf.CellFormat(widths[i], 7, tr(h), "1", 0, "L", false, 0, "")
	}
	pdf.Ln(7)
	pdf.SetFont("Arial", "", 9)
}

// pdfPageBreak adds a new page (and redraws the header) when the next row of
// height rowH would overflow the bottom margin.
func pdfPageBreak(pdf *fpdf.Fpdf, tr func(string) string, headers []string, widths []float64, rowH float64) {
	_, pageH := pdf.GetPageSize()
	_, _, _, bottom := pdf.GetMargins()
	if pdf.GetY()+rowH > pageH-bottom {
		pdf.AddPage()
		pdfTableHeader(pdf, tr, headers, widths)
	}
}

// kontoLabelPDF renders "Number" or "Number Name" for a booking account.
func kontoLabelPDF(chart *ChartOfAccounts, konto int) string {
	if chart != nil {
		if acc, ok := chart.Find(konto); ok {
			return fmt.Sprintf("%d %s", konto, acc.Name)
		}
	}
	return fmt.Sprintf("%d", konto)
}

// BuildBookingJournalPDF renders the booking journal: one row per Soll entry of
// each balanced booking, against the payment account as counter-account.
func BuildBookingJournalPDF(rows []CSVRow, chart *ChartOfAccounts, title string) ([]byte, error) {
	pdf, tr := newReportPDF(title, "L")

	headers := []string{"Datum", "Beleg", "Auftraggeber", "Soll-Konto", "Haben-Konto", "Betrag"}
	widths := []float64{20, 35, 70, 55, 55, 25}
	pdfTableHeader(pdf, tr, headers, widths)

	var total float64
	for _, r := range rows {
		pay, ok := r.Buchung.PaymentEntry()
		if !r.Buchung.Balanced() || !ok {
			continue
		}
		for _, e := range r.Buchung.DebitEntries() {
			pdfPageBreak(pdf, tr, headers, widths, 6)
			cells := []struct {
				w     float64
				txt   string
				align string
			}{
				{widths[0], r.Rechnungsdatum, "L"},
				{widths[1], truncate(r.Rechnungsnummer, 22), "L"},
				{widths[2], truncate(r.Auftraggeber, 40), "L"},
				{widths[3], truncate(kontoLabelPDF(chart, e.Konto), 32), "L"},
				{widths[4], truncate(kontoLabelPDF(chart, pay.Konto), 32), "L"},
				{widths[5], pdfAmount(e.Betrag), "R"},
			}
			for _, c := range cells {
				pdf.CellFormat(c.w, 6, tr(c.txt), "1", 0, c.align, false, 0, "")
			}
			pdf.Ln(6)
			total += e.Betrag
		}
	}

	pdfPageBreak(pdf, tr, headers, widths, 7)
	pdf.SetFont("Arial", "B", 9)
	pdf.CellFormat(widths[0]+widths[1]+widths[2]+widths[3]+widths[4], 7, tr("Summe"), "1", 0, "R", false, 0, "")
	pdf.CellFormat(widths[5], 7, tr(pdfAmount(round2(total))), "1", 0, "R", false, 0, "")
	pdf.Ln(7)

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// truncate shortens s to at most n runes (rune-safe for umlauts).
func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

// BuildControllingPDF renders a Controlling report with two sections
// (Einnahmen and Ausgaben), each listing per-account sums plus a section
// total, followed by a bold Saldo line.
func BuildControllingPDF(c Controlling, title string) ([]byte, error) {
	pdf, tr := newReportPDF(title, "P")

	headers := []string{"Konto", "Bezeichnung", "Summe"}
	widths := []float64{25, 120, 35}

	renderSection := func(heading string, sums []AccountSum, total float64) {
		pdf.SetFont("Arial", "B", 10)
		pdf.CellFormat(0, 8, tr(heading), "", 1, "L", false, 0, "")
		pdf.SetFont("Arial", "", 9)
		pdfTableHeader(pdf, tr, headers, widths)
		for _, s := range sums {
			pdfPageBreak(pdf, tr, headers, widths, 6)
			pdf.CellFormat(widths[0], 6, tr(fmt.Sprintf("%d", s.Konto)), "1", 0, "L", false, 0, "")
			pdf.CellFormat(widths[1], 6, tr(truncate(s.Name, 70)), "1", 0, "L", false, 0, "")
			pdf.CellFormat(widths[2], 6, tr(pdfAmount(s.Summe)), "1", 0, "R", false, 0, "")
			pdf.Ln(6)
		}
		pdfPageBreak(pdf, tr, headers, widths, 7)
		pdf.SetFont("Arial", "B", 9)
		pdf.CellFormat(widths[0]+widths[1], 7, tr("Summe"), "1", 0, "R", false, 0, "")
		pdf.CellFormat(widths[2], 7, tr(pdfAmount(round2(total))), "1", 0, "R", false, 0, "")
		pdf.Ln(7)
		pdf.SetFont("Arial", "", 9)
		pdf.Ln(3)
	}

	renderSection("Einnahmen", c.Einnahmen, c.EinnahmenGesamt)
	renderSection("Ausgaben", c.Ausgaben, c.AusgabenGesamt)

	// Bold Saldo line.
	pdfPageBreak(pdf, tr, headers, widths, 8)
	pdf.SetFont("Arial", "B", 10)
	pdf.CellFormat(widths[0]+widths[1], 8, tr("Saldo"), "1", 0, "R", false, 0, "")
	pdf.CellFormat(widths[2], 8, tr(pdfAmount(c.Saldo)), "1", 0, "R", false, 0, "")
	pdf.Ln(8)

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// BuildInvoiceListPDF renders the invoices as a landscape table with a Brutto total.
func BuildInvoiceListPDF(rows []CSVRow, title string) ([]byte, error) {
	pdf, tr := newReportPDF(title, "L")

	headers := []string{"Datum", "Auftraggeber", "Rechnungsnr.", "Netto", "MwSt", "Brutto"}
	widths := []float64{22, 90, 45, 30, 30, 30}
	pdfTableHeader(pdf, tr, headers, widths)

	var totalBrutto float64
	for _, r := range rows {
		pdfPageBreak(pdf, tr, headers, widths, 6)
		cells := []struct {
			w     float64
			txt   string
			align string
		}{
			{widths[0], r.Rechnungsdatum, "L"},
			{widths[1], truncate(r.Auftraggeber, 52), "L"},
			{widths[2], truncate(r.Rechnungsnummer, 26), "L"},
			{widths[3], pdfAmount(r.BetragNetto), "R"},
			{widths[4], pdfAmount(r.SteuersatzBetrag), "R"},
			{widths[5], pdfAmount(r.Bruttobetrag), "R"},
		}
		for _, c := range cells {
			pdf.CellFormat(c.w, 6, tr(c.txt), "1", 0, c.align, false, 0, "")
		}
		pdf.Ln(6)
		totalBrutto += r.Bruttobetrag
	}

	pdfPageBreak(pdf, tr, headers, widths, 7)
	pdf.SetFont("Arial", "B", 9)
	pdf.CellFormat(widths[0]+widths[1]+widths[2]+widths[3]+widths[4], 7, tr("Summe Brutto"), "1", 0, "R", false, 0, "")
	pdf.CellFormat(widths[5], 7, tr(pdfAmount(round2(totalBrutto))), "1", 0, "R", false, 0, "")
	pdf.Ln(7)

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// BuildUStVAPDF renders the official UStVA Kennzahlen (Kz 81/86/21/45/84/85/
// 66/67/83) as a form — the document the user hands to the tax advisor. Only
// non-zero Kennzahlen are shown; Kz 83 (Zahllast/Überschuss) is always shown.
func BuildUStVAPDF(u UStVAOfficial, title string) ([]byte, error) {
	pdf, tr := newReportPDF(title, "P")
	wKz, wLabel, wVal := 18.0, 122.0, 35.0

	row := func(kz, label string, value float64, bold bool) {
		style := ""
		if bold {
			style = "B"
		}
		pdf.SetFont("Arial", style, 9)
		pdf.CellFormat(wKz, 6, tr(kz), "1", 0, "L", false, 0, "")
		pdf.CellFormat(wLabel, 6, tr(truncate(label, 80)), "1", 0, "L", false, 0, "")
		pdf.CellFormat(wVal, 6, tr(pdfAmount(value)), "1", 0, "R", false, 0, "")
		pdf.Ln(6)
		pdf.SetFont("Arial", "", 9)
	}
	section := func(h string) {
		pdf.Ln(2)
		pdf.SetFont("Arial", "B", 10)
		pdf.CellFormat(0, 7, tr(h), "", 1, "L", false, 0, "")
		pdf.SetFont("Arial", "", 9)
	}

	section("A. Steuerpflichtige Umsätze")
	if u.Kz81 != 0 {
		row("Kz 81", "Umsätze 19 % (Bemessungsgrundlage)", u.Kz81, false)
		row("", "   davon Umsatzsteuer", u.USt81, false)
	}
	if u.Kz86 != 0 {
		row("Kz 86", "Umsätze 7 % (Bemessungsgrundlage)", u.Kz86, false)
		row("", "   davon Umsatzsteuer", u.USt86, false)
	}
	section("D. Leistungsempfänger als Steuerschuldner (§ 13b UStG)")
	if u.Kz84 != 0 {
		row("Kz 84", "§ 13b Bemessungsgrundlage", u.Kz84, false)
		row("Kz 85", "§ 13b Steuer", u.Kz85, false)
	}
	section("E. Nicht steuerbare Umsätze")
	if u.Kz21 != 0 {
		row("Kz 21", "Innergem. sonstige Leistungen (§ 18b UStG)", u.Kz21, false)
	}
	if u.Kz45 != 0 {
		row("Kz 45", "Übrige nicht steuerbare Umsätze (Ausland)", u.Kz45, false)
	}
	section("F. Abziehbare Vorsteuerbeträge")
	if u.Kz66 != 0 {
		row("Kz 66", "Vorsteuer aus Rechnungen", u.Kz66, false)
	}
	if u.Kz67 != 0 {
		row("Kz 67", "Vorsteuer aus § 13b-Leistungen", u.Kz67, false)
	}
	section("H. Verbleibende Vorauszahlung / Überschuss")
	row("Kz 83", "Zahllast / Überschuss", u.Kz83, true)

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// BuildZMPDF renders the Zusammenfassende Meldung: one row per EU customer
// VAT-ID with its net sum + "Sonstige Leistung", plus the Kontrollsumme and the
// own VAT-ID in the header (when set).
func BuildZMPDF(z ZM, ownVatID, title string) ([]byte, error) {
	pdf, tr := newReportPDF(title, "P")
	if ownVatID != "" {
		pdf.SetFont("Arial", "", 9)
		pdf.CellFormat(0, 6, tr("Eigene USt-IdNr.: "+ownVatID), "", 1, "L", false, 0, "")
		pdf.Ln(1)
	}
	headers := []string{"USt-IdNr. (Kunde)", "Summe (netto)", "Art der Leistung"}
	widths := []float64{60, 40, 70}
	pdfTableHeader(pdf, tr, headers, widths)
	for _, l := range z.Zeilen {
		pdfPageBreak(pdf, tr, headers, widths, 6)
		pdf.CellFormat(widths[0], 6, tr(l.UStIdNr), "1", 0, "L", false, 0, "")
		pdf.CellFormat(widths[1], 6, tr(pdfAmount(l.Netto)), "1", 0, "R", false, 0, "")
		pdf.CellFormat(widths[2], 6, tr("Sonstige Leistung"), "1", 0, "L", false, 0, "")
		pdf.Ln(6)
	}
	pdfPageBreak(pdf, tr, headers, widths, 7)
	pdf.SetFont("Arial", "B", 9)
	pdf.CellFormat(widths[0], 7, tr("Kontrollsumme"), "1", 0, "R", false, 0, "")
	pdf.CellFormat(widths[1], 7, tr(pdfAmount(z.Kontrollsumme)), "1", 0, "R", false, 0, "")
	pdf.CellFormat(widths[2], 7, tr(""), "1", 0, "L", false, 0, "")
	pdf.Ln(7)

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// BuildOpenItemsPDF renders the Offene-Posten-Liste as a portrait PDF with two
// sections: Debitoren (Forderungen) and Kreditoren (Verbindlichkeiten). Each
// section lists Belegnr, Datum, Partner, Betrag, Alter (Tage) and Bucket, and
// closes with a section total.
func BuildOpenItemsPDF(oi OpenItems, title string) ([]byte, error) {
	pdf, tr := newReportPDF(title, "L")

	headers := []string{"Belegnr.", "Datum", "Partner", "Betrag", "Alter (Tage)", "Bucket"}
	widths := []float64{30, 22, 90, 28, 28, 20}

	renderSection := func(heading string, items []OpenItem, total float64) {
		pdf.SetFont("Arial", "B", 10)
		pdf.CellFormat(0, 8, tr(heading), "", 1, "L", false, 0, "")
		pdf.SetFont("Arial", "", 9)
		pdfTableHeader(pdf, tr, headers, widths)

		for _, it := range items {
			pdfPageBreak(pdf, tr, headers, widths, 6)
			cells := []struct {
				w     float64
				txt   string
				align string
			}{
				{widths[0], truncate(it.Belegnummer, 18), "L"},
				{widths[1], it.Datum, "L"},
				{widths[2], truncate(it.Partner, 52), "L"},
				{widths[3], pdfAmount(it.Betrag), "R"},
				{widths[4], fmt.Sprintf("%d", it.AgeDays), "R"},
				{widths[5], it.Bucket, "L"},
			}
			for _, c := range cells {
				pdf.CellFormat(c.w, 6, tr(c.txt), "1", 0, c.align, false, 0, "")
			}
			pdf.Ln(6)
		}

		pdfPageBreak(pdf, tr, headers, widths, 7)
		pdf.SetFont("Arial", "B", 9)
		spanW := widths[0] + widths[1] + widths[2] + widths[4] + widths[5]
		pdf.CellFormat(spanW, 7, tr("Summe"), "1", 0, "R", false, 0, "")
		pdf.CellFormat(widths[3], 7, tr(pdfAmount(round2(total))), "1", 0, "R", false, 0, "")
		pdf.Ln(7)
		pdf.SetFont("Arial", "", 9)
		pdf.Ln(3)
	}

	renderSection("Debitoren (Forderungen)", oi.Forderungen, oi.ForderungenGesamt)
	renderSection("Kreditoren (Verbindlichkeiten)", oi.Verbindlichkeiten, oi.VerbindlichkeitenGesamt)

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// BuildSuSaPDF renders the Summen- und Saldenliste as a portrait PDF with
// columns Konto · Name · Soll · Haben · Saldo and a totals row at the bottom.
func BuildSuSaPDF(bals []AccountBalance, title string) ([]byte, error) {
	pdf, tr := newReportPDF(title, "P")

	headers := []string{"Konto", "Bezeichnung", "Soll", "Haben", "Saldo"}
	widths := []float64{18, 97, 25, 25, 25}
	pdfTableHeader(pdf, tr, headers, widths)

	var totalSoll, totalHaben, totalSaldo float64
	for _, b := range bals {
		pdfPageBreak(pdf, tr, headers, widths, 6)
		pdf.CellFormat(widths[0], 6, tr(fmt.Sprintf("%d", b.Konto)), "1", 0, "L", false, 0, "")
		pdf.CellFormat(widths[1], 6, tr(truncate(b.Name, 57)), "1", 0, "L", false, 0, "")
		pdf.CellFormat(widths[2], 6, tr(pdfAmount(b.SollSumme)), "1", 0, "R", false, 0, "")
		pdf.CellFormat(widths[3], 6, tr(pdfAmount(b.HabenSumme)), "1", 0, "R", false, 0, "")
		pdf.CellFormat(widths[4], 6, tr(pdfAmount(b.Saldo)), "1", 0, "R", false, 0, "")
		pdf.Ln(6)
		totalSoll += b.SollSumme
		totalHaben += b.HabenSumme
		totalSaldo += b.Saldo
	}

	pdfPageBreak(pdf, tr, headers, widths, 7)
	pdf.SetFont("Arial", "B", 9)
	pdf.CellFormat(widths[0]+widths[1], 7, tr("Summe"), "1", 0, "R", false, 0, "")
	pdf.CellFormat(widths[2], 7, tr(pdfAmount(round2(totalSoll))), "1", 0, "R", false, 0, "")
	pdf.CellFormat(widths[3], 7, tr(pdfAmount(round2(totalHaben))), "1", 0, "R", false, 0, "")
	pdf.CellFormat(widths[4], 7, tr(pdfAmount(round2(totalSaldo))), "1", 0, "R", false, 0, "")
	pdf.Ln(7)

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// BuildGuVPDF renders the Gewinn- und Verlustrechnung with an Erlöse section,
// an Aufwand section and a bold Ergebnis line.
func BuildGuVPDF(g GuV, title string) ([]byte, error) {
	pdf, tr := newReportPDF(title, "P")

	headers := []string{"Konto", "Bezeichnung", "Betrag"}
	widths := []float64{18, 107, 35}

	renderSection := func(heading string, posten []AccountBalance, getSumme func(AccountBalance) float64, total float64) {
		pdf.SetFont("Arial", "B", 10)
		pdf.CellFormat(0, 8, tr(heading), "", 1, "L", false, 0, "")
		pdf.SetFont("Arial", "", 9)
		pdfTableHeader(pdf, tr, headers, widths)
		for _, p := range posten {
			pdfPageBreak(pdf, tr, headers, widths, 6)
			pdf.CellFormat(widths[0], 6, tr(fmt.Sprintf("%d", p.Konto)), "1", 0, "L", false, 0, "")
			pdf.CellFormat(widths[1], 6, tr(truncate(p.Name, 63)), "1", 0, "L", false, 0, "")
			pdf.CellFormat(widths[2], 6, tr(pdfAmount(getSumme(p))), "1", 0, "R", false, 0, "")
			pdf.Ln(6)
		}
		pdfPageBreak(pdf, tr, headers, widths, 7)
		pdf.SetFont("Arial", "B", 9)
		pdf.CellFormat(widths[0]+widths[1], 7, tr("Summe"), "1", 0, "R", false, 0, "")
		pdf.CellFormat(widths[2], 7, tr(pdfAmount(round2(total))), "1", 0, "R", false, 0, "")
		pdf.Ln(7)
		pdf.SetFont("Arial", "", 9)
		pdf.Ln(3)
	}

	renderSection("Erlöse", g.ErloesPosten,
		func(p AccountBalance) float64 { return round2(p.HabenSumme - p.SollSumme) },
		g.ErloeseGesamt)
	renderSection("Aufwand", g.AufwandPosten,
		func(p AccountBalance) float64 { return round2(p.SollSumme - p.HabenSumme) },
		g.AufwandGesamt)

	pdfPageBreak(pdf, tr, headers, widths, 8)
	pdf.SetFont("Arial", "B", 10)
	pdf.CellFormat(widths[0]+widths[1], 8, tr("Ergebnis"), "1", 0, "R", false, 0, "")
	pdf.CellFormat(widths[2], 8, tr(pdfAmount(g.Ergebnis)), "1", 0, "R", false, 0, "")
	pdf.Ln(8)

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// BuildAnlagenspiegelPDF renders the Anlagenspiegel (asset register) for a given
// year as a landscape PDF. Columns: Bezeichnung · Anschaffung · AK · ND · AfA(Jahr)
// · Restbuchwert · GWG. A totals row for AfA and Restbuchwert is appended.
func BuildAnlagenspiegelPDF(rows []AnlagenRow, jahr int, title string) ([]byte, error) {
	pdf, tr := newReportPDF(title, "L")

	headers := []string{"Bezeichnung", "Anschaffung", "AK (€)", "ND (J)", "AfA " + fmt.Sprintf("%d", jahr), "Restbuchwert", "GWG"}
	widths := []float64{70, 26, 28, 18, 30, 32, 14}
	pdfTableHeader(pdf, tr, headers, widths)

	var totalAfa, totalRbw float64
	for _, row := range rows {
		pdfPageBreak(pdf, tr, headers, widths, 6)
		gwgLabel := ""
		if row.GWG {
			gwgLabel = "J"
		}
		cells := []struct {
			w     float64
			txt   string
			align string
		}{
			{widths[0], truncate(row.Asset.Bezeichnung, 40), "L"},
			{widths[1], row.Asset.Anschaffungsdatum, "L"},
			{widths[2], pdfAmount(row.Asset.Anschaffungswert), "R"},
			{widths[3], fmt.Sprintf("%d", row.Asset.NutzungsdauerJahre), "R"},
			{widths[4], pdfAmount(row.AfaJahr), "R"},
			{widths[5], pdfAmount(row.Restbuchwert), "R"},
			{widths[6], gwgLabel, "C"},
		}
		for _, c := range cells {
			pdf.CellFormat(c.w, 6, tr(c.txt), "1", 0, c.align, false, 0, "")
		}
		pdf.Ln(6)
		totalAfa += row.AfaJahr
		totalRbw += row.Restbuchwert
	}

	pdfPageBreak(pdf, tr, headers, widths, 7)
	pdf.SetFont("Arial", "B", 9)
	spanW := widths[0] + widths[1] + widths[2] + widths[3]
	pdf.CellFormat(spanW, 7, tr("Summe"), "1", 0, "R", false, 0, "")
	pdf.CellFormat(widths[4], 7, tr(pdfAmount(round2(totalAfa))), "1", 0, "R", false, 0, "")
	pdf.CellFormat(widths[5], 7, tr(pdfAmount(round2(totalRbw))), "1", 0, "R", false, 0, "")
	pdf.CellFormat(widths[6], 7, tr(""), "1", 0, "L", false, 0, "")
	pdf.Ln(7)

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// BuildSalesJournalPDF renders the Rechnungsausgangsbuch — a landscape table of
// all outgoing invoices (Ausgangsrechnungen) in the period with Belegnummer,
// date, customer, revenue account, net, VAT and gross, plus net/gross totals.
func BuildSalesJournalPDF(rows []CSVRow, chart *ChartOfAccounts, title string) ([]byte, error) {
	pdf, tr := newReportPDF(title, "L")

	headers := []string{"Belegnr.", "Rechnungsnr.", "Datum", "Kunde", "Erlöskonto", "Netto", "USt", "Brutto"}
	widths := []float64{24, 38, 20, 66, 46, 26, 26, 26}
	pdfTableHeader(pdf, tr, headers, widths)

	var totalNetto, totalUSt, totalBrutto float64
	for _, r := range rows {
		if !r.Ausgangsrechnung {
			continue
		}
		pdfPageBreak(pdf, tr, headers, widths, 6)
		cells := []struct {
			w     float64
			txt   string
			align string
		}{
			{widths[0], r.Belegnummer, "L"},
			{widths[1], truncate(r.Rechnungsnummer, 22), "L"},
			{widths[2], r.Rechnungsdatum, "L"},
			{widths[3], truncate(r.Auftraggeber, 38), "L"},
			{widths[4], truncate(kontoLabelPDF(chart, r.Gegenkonto), 26), "L"},
			{widths[5], pdfAmount(r.BetragNetto), "R"},
			{widths[6], pdfAmount(r.SteuersatzBetrag), "R"},
			{widths[7], pdfAmount(r.Bruttobetrag), "R"},
		}
		for _, c := range cells {
			pdf.CellFormat(c.w, 6, tr(c.txt), "1", 0, c.align, false, 0, "")
		}
		pdf.Ln(6)
		totalNetto += r.BetragNetto
		totalUSt += r.SteuersatzBetrag
		totalBrutto += r.Bruttobetrag
	}

	pdfPageBreak(pdf, tr, headers, widths, 7)
	pdf.SetFont("Arial", "B", 9)
	pdf.CellFormat(widths[0]+widths[1]+widths[2]+widths[3]+widths[4], 7, tr("Summe"), "1", 0, "R", false, 0, "")
	pdf.CellFormat(widths[5], 7, tr(pdfAmount(round2(totalNetto))), "1", 0, "R", false, 0, "")
	pdf.CellFormat(widths[6], 7, tr(pdfAmount(round2(totalUSt))), "1", 0, "R", false, 0, "")
	pdf.CellFormat(widths[7], 7, tr(pdfAmount(round2(totalBrutto))), "1", 0, "R", false, 0, "")
	pdf.Ln(7)

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
