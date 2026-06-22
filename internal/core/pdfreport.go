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
	pdf.SetFont("Arial", "B", 9)
	for i, h := range headers {
		pdf.CellFormat(widths[i], 7, tr(h), "1", 0, "L", false, 0, "")
	}
	pdf.Ln(7)
	pdf.SetFont("Arial", "", 9)

	var total float64
	for _, r := range rows {
		pay, ok := r.Buchung.PaymentEntry()
		if !r.Buchung.Balanced() || !ok {
			continue
		}
		for _, e := range r.Buchung.DebitEntries() {
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
