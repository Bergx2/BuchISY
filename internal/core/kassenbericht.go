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

// WriteCashReportPDF writes a monthly cash report (Kassenbericht) for one
// cash account to a landscape A4 PDF at path.
func WriteCashReportPDF(path string, book CashBook, entries []CashEntry, endbestand float64, monthLabel, decimalSep string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create report directory: %w", err)
	}
	pdf := fpdf.New("L", "mm", "A4", "")
	tr := pdf.UnicodeTranslatorFromDescriptor("") // UTF-8 -> cp1252 for core fonts
	pdf.AddPage()

	pdf.SetFont("Arial", "B", 14)
	pdf.CellFormat(0, 10, tr("Kassenbericht - "+book.Konto), "", 1, "L", false, 0, "")
	pdf.SetFont("Arial", "", 10)
	pdf.CellFormat(0, 6, tr("Monat: "+monthLabel), "", 1, "L", false, 0, "")
	pdf.CellFormat(0, 6, tr("Anfangsbestand: "+formatCashAmount(book.Anfangsbestand, decimalSep)), "", 1, "L", false, 0, "")
	pdf.Ln(3)

	widths := []float64{25, 80, 75, 30, 30, 32}
	headers := []string{"Datum", "Beschreibung", "Beleg", "Einnahme", "Ausgabe", "Saldo"}
	pdf.SetFont("Arial", "B", 9)
	for i, h := range headers {
		pdf.CellFormat(widths[i], 7, tr(h), "1", 0, "C", false, 0, "")
	}
	pdf.Ln(-1)

	pdf.SetFont("Arial", "", 9)
	for _, e := range entries {
		einnahme := ""
		if e.Einnahme != 0 {
			einnahme = formatCashAmount(e.Einnahme, decimalSep)
		}
		ausgabe := ""
		if e.Ausgabe != 0 {
			ausgabe = formatCashAmount(e.Ausgabe, decimalSep)
		}
		pdf.CellFormat(widths[0], 6, tr(e.Datum), "1", 0, "L", false, 0, "")
		pdf.CellFormat(widths[1], 6, tr(truncateRunes(e.Beschreibung, 50)), "1", 0, "L", false, 0, "")
		pdf.CellFormat(widths[2], 6, tr(truncateRunes(e.Beleg, 48)), "1", 0, "L", false, 0, "")
		pdf.CellFormat(widths[3], 6, tr(einnahme), "1", 0, "R", false, 0, "")
		pdf.CellFormat(widths[4], 6, tr(ausgabe), "1", 0, "R", false, 0, "")
		pdf.CellFormat(widths[5], 6, tr(formatCashAmount(e.Saldo, decimalSep)), "1", 0, "R", false, 0, "")
		pdf.Ln(-1)
	}

	pdf.Ln(3)
	pdf.SetFont("Arial", "B", 11)
	pdf.CellFormat(0, 7, tr("Endbestand: "+formatCashAmount(endbestand, decimalSep)), "", 1, "L", false, 0, "")

	return pdf.OutputFileAndClose(path)
}
