package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-pdf/fpdf"
)

// formatCashAmount formats an amount with the configured decimal separator.
func formatCashAmount(v float64, decimalSep string) string {
	s := fmt.Sprintf("%.2f", v)
	if decimalSep == "," {
		s = strings.ReplaceAll(s, ".", ",")
	}
	return s
}

// truncateRunes shortens s to at most max runes, adding "..." if cut.
func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max < 4 {
		return string(r[:max])
	}
	return string(r[:max-3]) + "..."
}

// WriteCashReportPDF writes a monthly cash report (Kassenbericht) for one cash
// account to an A4 PORTRAIT PDF at path, laid out like the Kassenbuch view:
// the company name on top, then an Einnahmen section and an Ausgaben section
// (each its own table, amounts prefixed with EUR, no per-row running balance),
// then the closing balance.
func WriteCashReportPDF(path, company string, book CashBook, entries []CashEntry, endbestand float64, monthLabel, decimalSep string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create report directory: %w", err)
	}
	pdf := fpdf.New("P", "mm", "A4", "")
	tr := pdf.UnicodeTranslatorFromDescriptor("") // UTF-8 -> cp1252 for core fonts
	pdf.AddPage()
	eur := func(v float64) string { return "EUR " + formatCashAmount(v, decimalSep) }

	// Header: company, report title, month, opening balance.
	if company != "" {
		pdf.SetFont("Arial", "B", 12)
		pdf.CellFormat(0, 7, tr(company), "", 1, "L", false, 0, "")
	}
	pdf.SetFont("Arial", "B", 14)
	pdf.CellFormat(0, 9, tr("Kassenbericht - "+book.Konto), "", 1, "L", false, 0, "")
	pdf.SetFont("Arial", "", 10)
	pdf.CellFormat(0, 6, tr("Monat: "+monthLabel), "", 1, "L", false, 0, "")
	pdf.CellFormat(0, 6, tr("Anfangsbestand: "+eur(book.Anfangsbestand)), "", 1, "L", false, 0, "")
	pdf.Ln(4)

	var sumEin, sumAus float64
	for _, e := range entries {
		sumEin += e.Einnahme
		sumAus += e.Ausgabe
	}

	section := func(title string) {
		pdf.SetFont("Arial", "B", 12)
		pdf.CellFormat(0, 7, tr(title), "", 1, "L", false, 0, "")
	}
	headerRow := func(widths []float64, titles []string, aligns []string) {
		pdf.SetFont("Arial", "B", 9)
		for i, h := range titles {
			pdf.CellFormat(widths[i], 7, tr(h), "1", 0, aligns[i], false, 0, "")
		}
		pdf.Ln(-1)
	}
	totalRow := func(label string, v float64) {
		pdf.SetFont("Arial", "B", 10)
		pdf.CellFormat(0, 7, tr(label+": "+eur(v)), "", 1, "R", false, 0, "")
	}

	// --- Einnahmen (top) ---
	section("Einnahmen")
	einW := []float64{28, 122, 40}
	headerRow(einW, []string{"Datum", "Beschreibung", "Einnahme"}, []string{"L", "L", "R"})
	pdf.SetFont("Arial", "", 9)
	nEin := 0
	for _, e := range entries {
		if e.Einnahme == 0 {
			continue
		}
		nEin++
		pdf.CellFormat(einW[0], 6, tr(e.Datum), "1", 0, "L", false, 0, "")
		pdf.CellFormat(einW[1], 6, tr(truncateRunes(e.Beschreibung, 70)), "1", 0, "L", false, 0, "")
		pdf.CellFormat(einW[2], 6, tr(eur(e.Einnahme)), "1", 0, "R", false, 0, "")
		pdf.Ln(-1)
	}
	if nEin == 0 {
		pdf.SetFont("Arial", "I", 9)
		pdf.CellFormat(190, 6, tr("(keine Einnahmen)"), "1", 1, "L", false, 0, "")
	}
	totalRow("Summe Einnahmen", sumEin)
	pdf.Ln(4)

	// --- Ausgaben (below) ---
	section("Ausgaben")
	ausW := []float64{28, 24, 98, 40}
	headerRow(ausW, []string{"Belegnummer", "Datum", "Beschreibung", "Ausgabe"}, []string{"L", "L", "L", "R"})
	pdf.SetFont("Arial", "", 9)
	nAus := 0
	for _, e := range entries {
		if e.Ausgabe == 0 {
			continue
		}
		nAus++
		belegnr := e.Belegnummer
		if belegnr == "" {
			belegnr = "-"
		}
		pdf.CellFormat(ausW[0], 6, tr(belegnr), "1", 0, "L", false, 0, "")
		pdf.CellFormat(ausW[1], 6, tr(e.Datum), "1", 0, "L", false, 0, "")
		pdf.CellFormat(ausW[2], 6, tr(truncateRunes(e.Beschreibung, 56)), "1", 0, "L", false, 0, "")
		pdf.CellFormat(ausW[3], 6, tr(eur(e.Ausgabe)), "1", 0, "R", false, 0, "")
		pdf.Ln(-1)
	}
	if nAus == 0 {
		pdf.SetFont("Arial", "I", 9)
		pdf.CellFormat(190, 6, tr("(keine Ausgaben)"), "1", 1, "L", false, 0, "")
	}
	totalRow("Summe Ausgaben", sumAus)
	pdf.Ln(5)

	pdf.SetFont("Arial", "B", 12)
	pdf.CellFormat(0, 8, tr("Endbestand: "+eur(endbestand)), "", 1, "R", false, 0, "")

	return pdf.OutputFileAndClose(path)
}
