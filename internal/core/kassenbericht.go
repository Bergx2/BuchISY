package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-pdf/fpdf"
)

// germanMonthLabel turns "2026-06" into "Juni 2026".
func germanMonthLabel(monthLabel string) string {
	parts := strings.SplitN(monthLabel, "-", 2)
	if len(parts) == 2 {
		months := []string{"", "Januar", "Februar", "März", "April", "Mai", "Juni",
			"Juli", "August", "September", "Oktober", "November", "Dezember"}
		if m, err := strconv.Atoi(parts[1]); err == nil && m >= 1 && m <= 12 {
			return months[m] + " " + parts[0]
		}
	}
	return monthLabel
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
	pdf.SetMargins(10, 10, 10)
	tr := pdf.UnicodeTranslatorFromDescriptor("") // UTF-8 -> cp1252 for core fonts
	pdf.AddPage()
	eur := func(v float64) string { return "EUR " + FormatAmount(v, decimalSep) }
	// All fonts 10% smaller than the base sizes (one consistent shrink).
	fs := func(pt float64) float64 { return pt * 0.9 }
	const lineH = 4.4

	// Title + company on ONE line (left); month ("Juni 2026") right-aligned on the
	// same line to save vertical space.
	title := "Kassenbericht - " + book.Konto
	if company != "" {
		title += ": " + company
	}
	pdf.SetFont("Arial", "B", fs(14))
	pdf.CellFormat(130, 9, tr(title), "", 0, "L", false, 0, "")
	pdf.SetFont("Arial", "B", fs(11))
	pdf.CellFormat(60, 9, tr(germanMonthLabel(monthLabel)), "", 1, "R", false, 0, "")

	// Anfangsbestand — right-aligned, bold, same size as Endbestand so they match.
	pdf.SetFont("Arial", "B", fs(12))
	pdf.CellFormat(0, 8, tr("Anfangsbestand: "+eur(book.Anfangsbestand)), "", 1, "R", false, 0, "")
	pdf.Ln(3)

	var sumEin, sumAus float64
	for _, e := range entries {
		sumEin += e.Einnahme
		sumAus += e.Ausgabe
	}

	section := func(t string) {
		pdf.SetFont("Arial", "B", fs(12))
		pdf.CellFormat(0, 7, tr(t), "", 1, "L", false, 0, "")
	}
	headerRow := func(widths []float64, titles, aligns []string) {
		pdf.SetFont("Arial", "B", fs(9))
		pdf.SetFillColor(225, 225, 225) // light-grey header band
		for i, h := range titles {
			pdf.CellFormat(widths[i], 6.5, tr(h), "1", 0, aligns[i], true, 0, "")
		}
		pdf.Ln(-1)
	}
	totalRow := func(label string, v float64) {
		pdf.SetFont("Arial", "B", fs(10))
		pdf.CellFormat(0, 7, tr(label+": "+eur(v)), "", 1, "R", false, 0, "")
	}
	emptyRow := func(txt string) {
		pdf.SetFont("Arial", "I", fs(9))
		pdf.CellFormat(190, 6, tr(txt), "1", 1, "C", false, 0, "") // centered
	}

	type pcol struct {
		text, align string
		w           float64
		wrap        bool
	}
	// dataRow draws one table row. Wrapping cells break onto at most TWO lines (any
	// further overflow is dropped with an ellipsis); non-wrapping cells are clipped
	// to a single line. All text is top-aligned within the (possibly two-line) row.
	dataRow := func(cells []pcol) {
		pdf.SetFont("Arial", "", fs(9))
		const pad = 1.5
		texts := make([]string, len(cells)) // already UTF-8→cp1252 translated
		maxLines := 1
		for i, c := range cells {
			t := tr(c.text)
			if c.wrap && t != "" {
				segs := pdf.SplitLines([]byte(t), c.w-pad)
				if len(segs) >= 2 {
					last := string(segs[1])
					if len(segs) > 2 { // more than 2 lines → clamp with ellipsis
						for pdf.GetStringWidth(last+"...") > c.w-pad && len(last) > 1 {
							last = last[:len(last)-1]
						}
						last += "..."
					}
					t = string(segs[0]) + "\n" + last
					maxLines = 2
				}
			} else { // clip to one line (e.g. the narrow Konto column)
				for c.w > 0 && pdf.GetStringWidth(t) > c.w-pad {
					r := []rune(t)
					if len(r) <= 1 {
						break
					}
					t = string(r[:len(r)-1])
				}
			}
			texts[i] = t
		}
		rowH := float64(maxLines) * lineH
		x0, y0 := pdf.GetX(), pdf.GetY()
		if y0+rowH > 287 {
			pdf.AddPage()
			x0, y0 = pdf.GetX(), pdf.GetY()
		}
		x := x0
		for i, c := range cells {
			pdf.Rect(x, y0, c.w, rowH, "D")
			pdf.SetXY(x, y0)
			if c.wrap {
				pdf.MultiCell(c.w, lineH, texts[i], "", c.align, false) // texts[i] already translated
			} else {
				pdf.CellFormat(c.w, lineH, texts[i], "", 0, c.align, false, 0, "")
			}
			x += c.w
		}
		pdf.SetXY(x0, y0+rowH)
	}

	// --- Einnahmen (top) ---
	section("Einnahmen")
	einW := []float64{22, 140, 28}
	headerRow(einW, []string{"Datum", "Beschreibung", "Einnahme"}, []string{"L", "L", "R"})
	nEin := 0
	for _, e := range entries {
		if e.Einnahme == 0 {
			continue
		}
		nEin++
		dataRow([]pcol{
			{e.Datum, "L", einW[0], false},
			{e.Beschreibung, "L", einW[1], true},
			{eur(e.Einnahme), "R", einW[2], false},
		})
	}
	if nEin == 0 {
		emptyRow("(keine Einnahmen)")
	}
	totalRow("Summe Einnahmen", sumEin)
	pdf.Ln(4)

	// --- Ausgaben (below): Belegnummer | Datum | Empfänger | Verwendungszweck | Konto | Ausgabe ---
	section("Ausgaben")
	ausW := []float64{24, 20, 49, 49, 20, 28}
	headerRow(ausW,
		[]string{"Belegnummer", "Datum", "Lieferant", "Verwendungszweck", "Konto", "Ausgabe"},
		[]string{"L", "L", "L", "L", "L", "R"})
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
		// Booked account: number + name (dataRow clips it to the narrow column).
		konto := e.Buchungskonto
		if konto == "" && e.Gegenkonto != 0 {
			konto = strconv.Itoa(e.Gegenkonto)
		}
		dataRow([]pcol{
			{belegnr, "L", ausW[0], false},
			{e.Datum, "L", ausW[1], false},
			{e.Beschreibung, "L", ausW[2], true},
			{e.Verwendungszweck, "L", ausW[3], true},
			{konto, "L", ausW[4], false},
			{eur(e.Ausgabe), "R", ausW[5], false},
		})
	}
	if nAus == 0 {
		emptyRow("(keine Ausgaben)")
	}
	totalRow("Summe Ausgaben", sumAus)
	pdf.Ln(5)

	pdf.SetFont("Arial", "B", fs(12))
	pdf.CellFormat(0, 8, tr("Endbestand: "+eur(endbestand)), "", 1, "R", false, 0, "")

	return pdf.OutputFileAndClose(path)
}
