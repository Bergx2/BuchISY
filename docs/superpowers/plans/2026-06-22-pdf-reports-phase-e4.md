# PDF-Reports (Phase E4) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Generate printable PDF reports — a Buchungsjournal, a Controlling-Report, and a Belegliste — from the stored data, with correct German characters (umlauts, €).

**Architecture:** Three pure-core builders in `internal/core/pdfreport.go` turn `[]CSVRow` / `[]AccountSum` into PDF bytes using the already-present `github.com/go-pdf/fpdf` library and a cp1252 unicode translator (so core fonts render ä/ö/ü/ß/€ without embedding a TTF). The UI wires file-save: a "PDF" button in the Controlling dialog, a Buchungsjournal PDF written alongside the DATEV/Lexware export, and a "Belegliste (PDF)" menu item.

**Tech Stack:** Go 1.25, `github.com/go-pdf/fpdf` v0.9.0 (already in go.mod), Fyne v2. Reuses D1 bookings, D3 `AggregateBookingsByAccount`, the export folder-pick pattern.

## Global Constraints

- PDF builders are pure `internal/core` (data → `[]byte`); no UI/DB. They may import `github.com/go-pdf/fpdf`.
- All text drawn through the cp1252 translator `tr(...)` so umlauts and € render with the core Arial font (no font embedding).
- Money formatted with German decimal comma; amounts right-aligned in their cells.
- Booking journal shows one row per Soll (debit) entry of each balanced booking, with the single Haben (payment) account as the counter-account — the same decomposition the DATEV export uses; non-bookable invoices are skipped.
- Builders must not panic on empty input or a nil chart (return a valid one-page PDF).
- A valid result starts with the bytes `%PDF`. All new user-facing strings via `a.bundle.T(...)` in both JSONs (valid JSON).
- `go build ./... && go test ./... && go vet ./internal/ui/` clean (ignore the pre-existing `clipboard_windows.go:206` warning). Commit per task.

---

### Task 1: PDF helper + Buchungsjournal builder

**Files:**
- Create: `internal/core/pdfreport.go`
- Test: `internal/core/pdfreport_test.go`

**Interfaces:**
- Consumes: `github.com/go-pdf/fpdf`, `CSVRow.Buchung` (`Balanced`/`PaymentEntry`/`DebitEntries`), `ChartOfAccounts.Find`.
- Produces: `newReportPDF(title, orientation string) (*fpdf.Fpdf, func(string) string)` (sets up an A4 page, title, cp1252 translator); `pdfAmount(v float64) string` (German comma); `BuildBookingJournalPDF(rows []CSVRow, chart *ChartOfAccounts, title string) ([]byte, error)`.

- [ ] **Step 1: Write the failing test**

```go
package core

import "testing"

func TestBuildBookingJournalPDF(t *testing.T) {
	rows := []CSVRow{
		{Rechnungsdatum: "18.06.2026", Rechnungsnummer: "R-1", Auftraggeber: "Matcha Rina (Café)",
			Buchung: Booking{Entries: []BookingEntry{
				{Konto: 6640, Betrag: 12.71, Soll: true},
				{Konto: 1800, Betrag: 12.71, Soll: false},
			}}},
		{Rechnungsdatum: "19.06.2026", Auftraggeber: "Ohne Buchung"}, // skipped
	}
	data, err := BuildBookingJournalPDF(rows, nil, "Buchungsjournal Juni 2026")
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 100 || string(data[:4]) != "%PDF" {
		t.Fatalf("not a PDF (%d bytes)", len(data))
	}
	// must not panic on empty input
	if _, err := BuildBookingJournalPDF(nil, nil, "Leer"); err != nil {
		t.Errorf("empty journal errored: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run TestBuildBookingJournalPDF`
Expected: FAIL (undefined BuildBookingJournalPDF).

- [ ] **Step 3: Implement**

Create `internal/core/pdfreport.go`:

```go
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run TestBuildBookingJournalPDF && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/core/pdfreport.go internal/core/pdfreport_test.go
git commit -m "Add PDF report helper and Buchungsjournal builder"
```

---

### Task 2: Controlling-Report PDF

**Files:**
- Modify: `internal/core/pdfreport.go`
- Test: `internal/core/pdfreport_test.go`

**Interfaces:**
- Consumes: `AccountSum`, `newReportPDF`, `pdfAmount`, `truncate`.
- Produces: `BuildControllingPDF(sums []AccountSum, total float64, title string) ([]byte, error)` — a portrait A4 table Konto · Bezeichnung · Summe + a total row.

- [ ] **Step 1: Write the failing test**

```go
func TestBuildControllingPDF(t *testing.T) {
	sums := []AccountSum{{Konto: 6640, Name: "Bewirtungskosten (abziehbar)", Summe: 1240.00}}
	data, err := BuildControllingPDF(sums, 1240.00, "Controlling 2026")
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 100 || string(data[:4]) != "%PDF" {
		t.Fatalf("not a PDF (%d bytes)", len(data))
	}
	if _, err := BuildControllingPDF(nil, 0, "Leer"); err != nil {
		t.Errorf("empty controlling PDF errored: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run TestBuildControllingPDF`
Expected: FAIL.

- [ ] **Step 3: Implement**

Add to `internal/core/pdfreport.go`:

```go
// BuildControllingPDF renders per-account summed amounts as a portrait table.
func BuildControllingPDF(sums []AccountSum, total float64, title string) ([]byte, error) {
	pdf, tr := newReportPDF(title, "P")

	headers := []string{"Konto", "Bezeichnung", "Summe"}
	widths := []float64{25, 120, 35}
	pdf.SetFont("Arial", "B", 9)
	for i, h := range headers {
		pdf.CellFormat(widths[i], 7, tr(h), "1", 0, "L", false, 0, "")
	}
	pdf.Ln(7)
	pdf.SetFont("Arial", "", 9)

	for _, s := range sums {
		pdf.CellFormat(widths[0], 6, tr(fmt.Sprintf("%d", s.Konto)), "1", 0, "L", false, 0, "")
		pdf.CellFormat(widths[1], 6, tr(truncate(s.Name, 70)), "1", 0, "L", false, 0, "")
		pdf.CellFormat(widths[2], 6, tr(pdfAmount(s.Summe)), "1", 0, "R", false, 0, "")
		pdf.Ln(6)
	}

	pdf.SetFont("Arial", "B", 9)
	pdf.CellFormat(widths[0]+widths[1], 7, tr("Summe"), "1", 0, "R", false, 0, "")
	pdf.CellFormat(widths[2], 7, tr(pdfAmount(round2(total))), "1", 0, "R", false, 0, "")
	pdf.Ln(7)

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run TestBuildControllingPDF && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/core/pdfreport.go internal/core/pdfreport_test.go
git commit -m "Add Controlling-Report PDF builder"
```

---

### Task 3: Belegliste PDF

**Files:**
- Modify: `internal/core/pdfreport.go`
- Test: `internal/core/pdfreport_test.go`

**Interfaces:**
- Consumes: `CSVRow`, `newReportPDF`, `pdfAmount`, `truncate`.
- Produces: `BuildInvoiceListPDF(rows []CSVRow, title string) ([]byte, error)` — a landscape table Datum · Auftraggeber · Rechnungsnummer · Netto · MwSt · Brutto + a Brutto total.

- [ ] **Step 1: Write the failing test**

```go
func TestBuildInvoiceListPDF(t *testing.T) {
	rows := []CSVRow{{Rechnungsdatum: "18.06.2026", Auftraggeber: "Müller GmbH", Rechnungsnummer: "R-1", BetragNetto: 100, SteuersatzBetrag: 19, Bruttobetrag: 119}}
	data, err := BuildInvoiceListPDF(rows, "Belegliste Juni 2026")
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 100 || string(data[:4]) != "%PDF" {
		t.Fatalf("not a PDF (%d bytes)", len(data))
	}
	if _, err := BuildInvoiceListPDF(nil, "Leer"); err != nil {
		t.Errorf("empty list PDF errored: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run TestBuildInvoiceListPDF`
Expected: FAIL.

- [ ] **Step 3: Implement**

Add to `internal/core/pdfreport.go`:

```go
// BuildInvoiceListPDF renders the invoices as a landscape table with a Brutto total.
func BuildInvoiceListPDF(rows []CSVRow, title string) ([]byte, error) {
	pdf, tr := newReportPDF(title, "L")

	headers := []string{"Datum", "Auftraggeber", "Rechnungsnr.", "Netto", "MwSt", "Brutto"}
	widths := []float64{22, 90, 45, 30, 30, 30}
	pdf.SetFont("Arial", "B", 9)
	for i, h := range headers {
		pdf.CellFormat(widths[i], 7, tr(h), "1", 0, "L", false, 0, "")
	}
	pdf.Ln(7)
	pdf.SetFont("Arial", "", 9)

	var totalBrutto float64
	for _, r := range rows {
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run TestBuildInvoiceListPDF && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/core/pdfreport.go internal/core/pdfreport_test.go
git commit -m "Add Belegliste PDF builder"
```

---

### Task 4: Wire PDF exports into the UI

**Files:**
- Modify: `internal/ui/controllingview.go` (PDF button)
- Modify: `internal/ui/bookingexport.go` (write the journal PDF alongside DATEV/Lexware)
- Modify: `internal/ui/app.go` (Belegliste PDF menu item) + a small shared save helper
- Test: none (UI; covered by build + manual). 

**Interfaces:**
- Consumes: `core.BuildBookingJournalPDF`, `core.BuildControllingPDF`, `core.BuildInvoiceListPDF`, the folder/save pattern, `a.chart`, `a.collectInvoiceRows`.
- Produces: a "PDF" button in the Controlling dialog; a `Buchungsjournal_<period>.pdf` written by `writeBookingExport`; a "Belegliste (PDF)" menu item.

- [ ] **Step 1: Add a save-PDF helper**

In `internal/ui/bookingexport.go` (or a new `internal/ui/pdfexport.go`), add a helper that asks for a file and writes PDF bytes:

```go
func (a *App) savePDF(defaultName string, data []byte) {
	d := dialog.NewFileSave(func(w fyne.URIWriteCloser, err error) {
		if w == nil {
			return
		}
		defer w.Close()
		if err != nil {
			a.showError(a.bundle.T("error.processing.title"), err.Error())
			return
		}
		if _, werr := w.Write(data); werr != nil {
			a.showError(a.bundle.T("error.processing.title"), werr.Error())
		}
	}, a.window)
	d.SetFileName(defaultName)
	d.Show()
}
```

- [ ] **Step 2: Controlling PDF button**

In `internal/ui/controllingview.go`, add a "PDF" button (i18n `report.pdf`) to the dialog that builds and saves the controlling PDF for the current view:

```go
	pdfBtn := widget.NewButton(a.bundle.T("report.pdf"), func() {
		title := a.bundle.T("controlling.title")
		data, err := core.BuildControllingPDF(sums, total, title)
		if err != nil {
			a.showError(a.bundle.T("error.processing.title"), err.Error())
			return
		}
		a.savePDF("Controlling.pdf", data)
	})
```

Place `pdfBtn` in the dialog (e.g. the top bar next to the toggle).

- [ ] **Step 3: Journal PDF alongside the export**

In `internal/ui/bookingexport.go` `writeBookingExport`, after the DATEV + Lexware files are written successfully (and before/after marking), also build and write the journal:

```go
		journal, jerr := core.BuildBookingJournalPDF(exportable, a.chart, "Buchungsjournal "+period)
		if jerr == nil {
			if werr := os.WriteFile(filepath.Join(uri.Path(), "Buchungsjournal_"+period+".pdf"), journal, 0644); werr != nil {
				a.logger.Warn("journal PDF write failed: %v", werr)
			}
		}
```

- [ ] **Step 4: Belegliste PDF menu item**

In `internal/ui/app.go`, next to the "USt-Voranmeldung" item, add `fyne.NewMenuItem("Belegliste (PDF)", func() { a.showBelegListePDF() })`, and add:

```go
func (a *App) showBelegListePDF() {
	period := fmt.Sprintf("%04d-%02d", a.currentYear, int(a.currentMonth))
	rows := a.collectInvoiceRows(a.currentYear, int(a.currentMonth), a.currentYear, int(a.currentMonth))
	data, err := core.BuildInvoiceListPDF(rows, "Belegliste "+period)
	if err != nil {
		a.showError(a.bundle.T("error.processing.title"), err.Error())
		return
	}
	a.savePDF("Belegliste_"+period+".pdf", data)
}
```

(Put `showBelegListePDF` wherever fits in app.go; ensure `fmt` and `core` are imported there.)

Add i18n key `report.pdf` (de "PDF" / en "PDF") to both JSONs.

- [ ] **Step 5: Build + vet + test**

Run: `go build ./... && go vet ./internal/ui/ && go test ./...`
Expected: clean. Validate both JSONs.

- [ ] **Step 6: Commit**

```bash
git add internal/ui/controllingview.go internal/ui/bookingexport.go internal/ui/app.go internal/ui/pdfexport.go assets/i18n/de.json assets/i18n/en.json
git commit -m "Wire PDF reports: Controlling button, journal alongside export, Belegliste menu"
```

---

## Self-Review

- **Spec coverage:** Buchungsjournal (Task 1 + journal-on-export in Task 4), Controlling-Report (Task 2 + button in Task 4), Belegliste (Task 3 + menu in Task 4). German characters via the cp1252 translator. Covered.
- **Placeholder scan:** every builder is fully coded; the UI task gives the helper + each wiring snippet with concrete anchors (`writeBookingExport`, the Controlling dialog, the menu).
- **Type consistency:** `newReportPDF(title,orientation)`, `pdfAmount`, `truncate`, `kontoLabelPDF`, `BuildBookingJournalPDF(rows,chart,title)`, `BuildControllingPDF(sums,total,title)`, `BuildInvoiceListPDF(rows,title)`, `savePDF(name,data)` — consistent across tasks.
- **Data integrity:** journal uses the same balanced/PaymentEntry/DebitEntries decomposition as the DATEV export; amounts comma-formatted + right-aligned; rune-safe truncation for umlauts; builders return a valid `%PDF` even for empty input.
- **Out of scope:** backup + plausibility warnings + new categories (E5).
