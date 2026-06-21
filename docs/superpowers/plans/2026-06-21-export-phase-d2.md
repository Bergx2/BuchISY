# DATEV- & Lexware-Export (Phase D2) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Export the stored bookings of a month or a whole year as a DATEV EXTF Buchungsstapel CSV and a native Lexware booking CSV, so the Steuerberater can import them.

**Architecture:** Two pure-core exporters turn `[]core.CSVRow` (each carrying its `Booking` from D1) into export bytes. The DATEV exporter writes an EXTF header line + column header + one Buchungssatz row per non-payment booking entry (each posted against the payment account). The Lexware exporter writes a simple semicolon CSV. Optional DATEV identifiers (Berater-/Mandanten-Nr., Wirtschaftsjahr-Beginn) live in Settings and default to empty. A small export dialog (month / year buttons) collects the rows and writes both files.

**Tech Stack:** Go 1.25, Fyne v2, encoding/csv-style manual writing (semicolon, CP1252 for DATEV). Reuses D1 bookings, `a.dbRepo`, the existing `collectInvoiceRows`/export-folder pattern in `internal/ui/csvexport.go`.

## Global Constraints

- Pure-core exporters (`internal/core`), no UI/DB deps; the UI task only collects rows + writes files.
- NEVER invent account numbers or amounts — rows come verbatim from each `Booking`'s entries.
- A booking is exported only when `Booking.Balanced()` is true; invoices without a bookable booking are SKIPPED and their count is reported to the user (no silent drop).
- Each booking is exported as: for every NON-payment entry (the Soll expense/Vorsteuer lines), one row `Umsatz=entry.Betrag, S/H=S, Konto=entry.Konto, Gegenkonto=<the single Haben/payment account>`. The payment entry itself is the Gegenkonto, not its own row. (D1 bookings are always N Soll vs 1 Haben.)
- DATEV identifiers are OPTIONAL: empty Berater/Mandant/WJ still produces a structurally valid EXTF file. Never block export on missing IDs.
- DATEV decimal = comma, amounts unsigned (sign is the S/H flag). Belegdatum format `DDMM` of the Rechnungsdatum. DATEV file encoding = Windows-1252.
- Money `float64`, 2 decimals. All user-facing strings via `a.bundle.T(...)` in both JSONs (valid JSON).
- `go build ./... && go test ./... && go vet ./internal/ui/` clean (ignore the pre-existing `clipboard_windows.go:206` warning). Commit per task.

---

### Task 1: DATEV settings + the single Haben helper

**Files:**
- Modify: `internal/core/types.go` (Settings fields + a Booking helper)
- Test: `internal/core/buchung_test.go`

**Interfaces:**
- Produces: `Settings.DatevBeraterNr string`, `Settings.DatevMandantNr string`, `Settings.DatevWJBeginn string` (json `datev_berater_nr` / `datev_mandant_nr` / `datev_wj_beginn`, all `,omitempty`);
  `(b Booking) PaymentEntry() (BookingEntry, bool)` — returns the single Haben (credit) entry, `(…,false)` if there isn't exactly one Haben;
  `(b Booking) DebitEntries() []BookingEntry` — the Soll entries.

- [ ] **Step 1: Write the failing test**

```go
func TestBookingPaymentSplit(t *testing.T) {
	b := Booking{Entries: []BookingEntry{
		{Konto: 6640, Betrag: 12.71, Soll: true},
		{Konto: 6644, Betrag: 5.44, Soll: true},
		{Konto: 1800, Betrag: 18.15, Soll: false},
	}}
	pay, ok := b.PaymentEntry()
	if !ok || pay.Konto != 1800 {
		t.Fatalf("payment = %+v ok=%v", pay, ok)
	}
	if len(b.DebitEntries()) != 2 {
		t.Errorf("want 2 debit entries, got %d", len(b.DebitEntries()))
	}
	// two Haben → not a clean single-payment booking
	b2 := Booking{Entries: []BookingEntry{{Konto: 1, Betrag: 1, Soll: false}, {Konto: 2, Betrag: 1, Soll: false}}}
	if _, ok := b2.PaymentEntry(); ok {
		t.Error("two Haben entries should yield ok=false")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run TestBookingPaymentSplit`
Expected: FAIL (undefined methods).

- [ ] **Step 3: Implement**

In `internal/core/types.go` `Settings` struct (after `OwnVATID`), add:

```go
	DatevBeraterNr string `json:"datev_berater_nr,omitempty"` // optional DATEV consultant number
	DatevMandantNr string `json:"datev_mandant_nr,omitempty"` // optional DATEV client number
	DatevWJBeginn  string `json:"datev_wj_beginn,omitempty"`  // fiscal-year start YYYYMMDD (optional)
```

In `internal/core/buchung.go`, add:

```go
// PaymentEntry returns the single Haben (credit) entry of the booking — the
// Zahlungskonto side. ok is false unless there is exactly one Haben entry.
func (b Booking) PaymentEntry() (BookingEntry, bool) {
	var found BookingEntry
	n := 0
	for _, e := range b.Entries {
		if !e.Soll {
			found = e
			n++
		}
	}
	if n != 1 {
		return BookingEntry{}, false
	}
	return found, true
}

// DebitEntries returns the Soll (debit) entries — the expense/Vorsteuer lines.
func (b Booking) DebitEntries() []BookingEntry {
	out := make([]BookingEntry, 0, len(b.Entries))
	for _, e := range b.Entries {
		if e.Soll {
			out = append(out, e)
		}
	}
	return out
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run TestBookingPaymentSplit && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/core/types.go internal/core/buchung.go internal/core/buchung_test.go
git commit -m "Add DATEV settings and booking payment/debit split helpers"
```

---

### Task 2: DATEV EXTF Buchungsstapel exporter

**Files:**
- Create: `internal/core/datevexport.go`
- Test: `internal/core/datevexport_test.go`

**Interfaces:**
- Consumes: `CSVRow` (uses `.Buchung`, `.Rechnungsdatum`, `.Rechnungsnummer`, `.Auftraggeber`, `.Verwendungszweck`), `Booking.Balanced/PaymentEntry/DebitEntries`, `Settings.DatevBeraterNr/DatevMandantNr/DatevWJBeginn`.
- Produces: `type DATEVHeader struct { BeraterNr, MandantNr, WJBeginn, ErzeugtAm, DatumVon, DatumBis string }`;
  `BuildDATEVStapel(h DATEVHeader, rows []CSVRow) (data []byte, exported int, skipped int)` — returns the EXTF file bytes (Windows-1252, semicolon, CRLF), the count of booking rows written, and the count of invoices skipped (no balanced booking).

The EXTF structure (each field semicolon-separated, text fields wrapped in `"`):
- **Header line** (the EXTF metadata, exactly these 24 positions; empty where not set):
  `"EXTF";700;21;"Buchungsstapel";13;<ErzeugtAm>;;;;;"<BeraterNr>";"<MandantNr>";<WJBeginn>;4;<DatumVon>;<DatumBis>;"";"";"";"";0;"EUR";"";"";"";""`
- **Column-header line** (the booking-field names): `Umsatz (ohne Soll/Haben-Kz);Soll/Haben-Kennzeichen;WKZ Umsatz;Kurs;Basis-Umsatz;WKZ Basis-Umsatz;Konto;Gegenkonto (ohne BU-Schlüssel);BU-Schlüssel;Belegdatum;Belegfeld 1;Belegfeld 2;Skonto;Buchungstext`
- **One data line per Soll entry** of every balanced booking, with the booking's payment entry as the Gegenkonto:
  `<Umsatz>;"S";"EUR";;;;<Konto>;<Gegenkonto>;;<Belegdatum>;"<Belegfeld1>";;;"<Buchungstext>"`
  where `Umsatz` = entry.Betrag with comma decimal, `Konto` = entry.Konto, `Gegenkonto` = paymentEntry.Konto, `Belegdatum` = DDMM from Rechnungsdatum (DD.MM.YYYY → "DDMM"), `Belegfeld1` = Rechnungsnummer (truncated to 36 chars, `"` stripped), `Buchungstext` = Auftraggeber + " " + Verwendungszweck (truncated to 60, `"` stripped).

- [ ] **Step 1: Write the failing test**

```go
package core

import (
	"strings"
	"testing"
)

func TestBuildDATEVStapel(t *testing.T) {
	rows := []CSVRow{
		{Rechnungsdatum: "18.06.2026", Rechnungsnummer: "MC9C7PFZ-103052", Auftraggeber: "Matcha Rina",
			Buchung: Booking{Entries: []BookingEntry{
				{Konto: 6640, Betrag: 12.71, Soll: true},
				{Konto: 6644, Betrag: 5.44, Soll: true},
				{Konto: 1406, Betrag: 1.26, Soll: true},
				{Konto: 1401, Betrag: 0.59, Soll: true},
				{Konto: 1800, Betrag: 20.00, Soll: false},
			}}},
		{Rechnungsdatum: "19.06.2026", Auftraggeber: "Ohne Buchung"}, // no booking → skipped
	}
	data, exported, skipped := BuildDATEVStapel(DATEVHeader{BeraterNr: "", MandantNr: "", WJBeginn: "20260101", ErzeugtAm: "20260621120000000", DatumVon: "20260601", DatumBis: "20260630"}, rows)
	if exported != 4 || skipped != 1 {
		t.Fatalf("exported=%d skipped=%d (want 4,1)", exported, skipped)
	}
	s := string(data)
	if !strings.HasPrefix(s, `"EXTF";700;21;"Buchungsstapel"`) {
		t.Errorf("missing EXTF header: %q", s[:40])
	}
	// One data row carries 12,71 booked on 6640 against 1800, Beleg 1806.
	if !strings.Contains(s, `12,71;"S";"EUR";;;;6640;1800;;1806;"MC9C7PFZ-103052"`) {
		t.Errorf("expected 6640 data row not found:\n%s", s)
	}
	// The payment account 1800 is never its own data row (only a Gegenkonto).
	if strings.Contains(s, `;"S";"EUR";;;;1800;`) {
		t.Error("payment account must not be a debit row")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run TestBuildDATEVStapel`
Expected: FAIL (undefined BuildDATEVStapel).

- [ ] **Step 3: Implement**

Create `internal/core/datevexport.go`:

```go
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
```

(The Windows-1252 re-encoding of these bytes happens in the UI write step, Task 4.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run TestBuildDATEVStapel && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/core/datevexport.go internal/core/datevexport_test.go
git commit -m "Add DATEV EXTF Buchungsstapel exporter"
```

---

### Task 3: Lexware booking CSV exporter

**Files:**
- Create: `internal/core/lexwareexport.go`
- Test: `internal/core/lexwareexport_test.go`

**Interfaces:**
- Consumes: `CSVRow` (`.Buchung`, `.Rechnungsdatum`, `.Rechnungsnummer`, `.Auftraggeber`, `.Verwendungszweck`).
- Produces: `BuildLexwareCSV(rows []CSVRow) (data []byte, exported int, skipped int)` — a semicolon CSV with a header row `Datum;Belegnr;Buchungstext;Betrag;Sollkonto;Habenkonto` and one line per Soll entry (Sollkonto = entry.Konto, Habenkonto = payment account, Betrag = comma decimal, Datum = DD.MM.YYYY as-is). Same skip rule as DATEV.

- [ ] **Step 1: Write the failing test**

```go
package core

import (
	"strings"
	"testing"
)

func TestBuildLexwareCSV(t *testing.T) {
	rows := []CSVRow{
		{Rechnungsdatum: "18.06.2026", Rechnungsnummer: "R-1", Auftraggeber: "Matcha Rina", Verwendungszweck: "Bewirtung",
			Buchung: Booking{Entries: []BookingEntry{
				{Konto: 6640, Betrag: 12.71, Soll: true},
				{Konto: 1800, Betrag: 12.71, Soll: false},
			}}},
		{Rechnungsdatum: "19.06.2026"}, // no booking → skipped
	}
	data, exported, skipped := BuildLexwareCSV(rows)
	if exported != 1 || skipped != 1 {
		t.Fatalf("exported=%d skipped=%d (want 1,1)", exported, skipped)
	}
	s := string(data)
	if !strings.HasPrefix(s, "Datum;Belegnr;Buchungstext;Betrag;Sollkonto;Habenkonto") {
		t.Errorf("missing header: %q", s)
	}
	if !strings.Contains(s, "18.06.2026;R-1;Matcha Rina Bewirtung;12,71;6640;1800") {
		t.Errorf("data line wrong:\n%s", s)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run TestBuildLexwareCSV`
Expected: FAIL (undefined BuildLexwareCSV).

- [ ] **Step 3: Implement**

Create `internal/core/lexwareexport.go`:

```go
package core

import (
	"fmt"
	"strings"
)

// BuildLexwareCSV renders the bookings of rows as a simple Lexware import CSV
// (semicolon-separated). Returns the bytes, rows written, invoices skipped.
func BuildLexwareCSV(rows []CSVRow) ([]byte, int, int) {
	var b strings.Builder
	b.WriteString("Datum;Belegnr;Buchungstext;Betrag;Sollkonto;Habenkonto\r\n")
	exported, skipped := 0, 0
	for _, r := range rows {
		pay, ok := r.Buchung.PaymentEntry()
		if !r.Buchung.Balanced() || !ok {
			skipped++
			continue
		}
		text := lexClean(strings.TrimSpace(r.Auftraggeber + " " + r.Verwendungszweck))
		beleg := lexClean(r.Rechnungsnummer)
		for _, e := range r.Buchung.DebitEntries() {
			amount := strings.Replace(fmt.Sprintf("%.2f", e.Betrag), ".", ",", 1)
			b.WriteString(fmt.Sprintf("%s;%s;%s;%s;%d;%d\r\n",
				r.Rechnungsdatum, beleg, text, amount, e.Konto, pay.Konto))
			exported++
		}
	}
	return []byte(b.String()), exported, skipped
}

// lexClean strips the field separator and newlines from a free-text field.
func lexClean(s string) string {
	s = strings.ReplaceAll(s, ";", ",")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run TestBuildLexwareCSV && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/core/lexwareexport.go internal/core/lexwareexport_test.go
git commit -m "Add Lexware booking CSV exporter"
```

---

### Task 4: Export dialog + DATEV settings field + file writing

**Files:**
- Modify: `internal/ui/csvexport.go` (or a new `internal/ui/bookingexport.go`) for the export dialog
- Modify: `internal/ui/settings.go` (DATEV identifier inputs)
- Modify: a menu/button host to open the export (wherever `showCSVExportDialog` is invoked from)
- Test: none (UI; covered by build + manual). 

**Interfaces:**
- Consumes: `a.collectInvoiceRows(fromY, fromM, toY, toM) []core.CSVRow` (existing), `core.BuildDATEVStapel`, `core.BuildLexwareCSV`, the existing export-folder/save pattern in `csvexport.go`, `a.settings.Datev*`.
- Produces: a "Buchungen exportieren" action that, for the current month or the whole year, writes `DATEV-EXTF_<period>.csv` (Windows-1252) and `Lexware-Buchungen_<period>.csv` (UTF-8) to the chosen folder, and shows a summary (`exported` rows, `skipped` invoices).

- [ ] **Step 1: Settings — DATEV identifier inputs**

In `internal/ui/settings.go`, near the accounts/CSV section, add three `widget.NewEntry()` bound to `Datev BeraterNr / MandantNr / WJBeginn`, labeled via i18n keys `settings.datev.berater` ("DATEV Berater-Nr."), `settings.datev.mandant` ("DATEV Mandanten-Nr."), `settings.datev.wj` ("Wirtschaftsjahr-Beginn (TTMMJJJJ)" / en "Fiscal year start (DDMMYYYY)"). On save, write the entry texts into `newSettings.DatevBeraterNr/DatevMandantNr/DatevWJBeginn` exactly as the other settings fields persist. Add a helper note label (i18n `settings.datev.hint`: de "Optional — leer lassen, falls noch nicht bekannt." / en "Optional — leave blank if unknown.").

- [ ] **Step 2: Export dialog**

Add `func (a *App) showBookingExportDialog()` modeled on `showCSVExportDialog`: two buttons — `export.month` ("Aktuellen Monat") and `export.year` ("Ganzes Jahr"). On click compute the range (month: `a.currentYear,a.currentMonth..same`; year: `a.currentYear,1..a.currentYear,12`), then:

```go
	rows := a.collectInvoiceRows(fromY, fromM, toY, toM)
	wj := a.settings.DatevWJBeginn
	h := core.DATEVHeader{
		BeraterNr: a.settings.DatevBeraterNr, MandantNr: a.settings.DatevMandantNr, WJBeginn: wj,
		ErzeugtAm: "", // optional; leave empty (DATEV tolerates) or fill from a passed-in timestamp
		DatumVon:  fmt.Sprintf("%04d%02d01", fromY, fromM),
		DatumBis:  fmt.Sprintf("%04d%02d31", toY, toM),
	}
	datevBytes, dExp, dSkip := core.BuildDATEVStapel(h, rows)
	lexBytes, _, _ := core.BuildLexwareCSV(rows)
```

Pick a TARGET FOLDER with `dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error){ ... }, a.window)` — the exact pattern already used in `internal/ui/settings.go:43`. In the callback (guard `uri == nil` = cancel, and `err`), write BOTH files into `uri.Path()` via `os.WriteFile(filepath.Join(uri.Path(), name), data, 0644)`:
- `DATEV-EXTF_<period>.csv` re-encoded to Windows-1252 via `golang.org/x/text/encoding/charmap` (`enc, encErr := charmap.Windows1252.NewEncoder().Bytes(datevBytes)`; on `encErr` fall back to `datevBytes`).
- `Lexware-Buchungen_<period>.csv` as UTF-8 (`lexBytes` directly).

`<period>` = `2026-06` (month) or `2026` (year). On any write error show `a.showError(...)`. On success show an info dialog `a.bundle.T("export.done", dExp, dSkip)` (de "%d Buchungszeilen exportiert, %d Belege ohne Buchung übersprungen." / en "%d booking rows exported, %d invoices without a booking skipped."). Imports needed in the export file: `os`, `path/filepath`, `golang.org/x/text/encoding/charmap`, `fyne.io/fyne/v2`, `fyne.io/fyne/v2/dialog`.

- [ ] **Step 3: Wire the action into the UI**

Add a menu item that calls `a.showBookingExportDialog()` right next to the existing `fyne.NewMenuItem("CSV-Export", func() { a.showCSVExportDialog() })` in `internal/ui/app.go` (~line 716). Label via i18n `export.bookings` ("Buchungen exportieren" / en "Export bookings"). Use `a.bundle.T("export.bookings")` for the menu label if neighboring items are translated; if that menu uses literal German strings (as "CSV-Export" suggests), match that local style with a literal "Buchungen exportieren" — follow whatever the surrounding menu items do.

- [ ] **Step 4: Build + vet + manual**

Run: `go build ./... && go vet ./internal/ui/ && go test ./...`
Expected: clean (only the pre-existing clipboard warning). Validate both JSONs parse.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/csvexport.go internal/ui/bookingexport.go internal/ui/settings.go assets/i18n/de.json assets/i18n/en.json
git commit -m "Add booking export dialog (DATEV + Lexware) and DATEV settings"
```

---

## Self-Review

- **Spec coverage (Phase D / Baustein D — export part):** DATEV EXTF Buchungsstapel (Task 2) + native Lexware CSV (Task 3) of the stored D1 bookings; optional DATEV identifiers in settings (Tasks 1/4); month + year export with a skipped-count summary (Task 4). Controlling is Phase D3 (separate plan).
- **Placeholder scan:** the DATEV header line, column header, and row format are spelled out verbatim; the UI task references concrete existing anchors (`collectInvoiceRows`, `saveExportCSV`, `showCSVExportDialog`) with explicit "reuse the same mechanism" instructions, not vague directives.
- **Type consistency:** `BuildDATEVStapel(DATEVHeader, []CSVRow)(([]byte,int,int))`, `BuildLexwareCSV([]CSVRow)([]byte,int,int)`, `Booking.PaymentEntry()(BookingEntry,bool)`, `Booking.DebitEntries()[]BookingEntry`, `Settings.Datev{BeraterNr,MandantNr,WJBeginn}` — consistent across tasks.
- **Data integrity:** rows come verbatim from bookings; only `Balanced()` bookings with exactly one Haben are exported; skipped invoices are counted and surfaced (no silent drop); payment account is never emitted as its own debit row; DATEV amounts unsigned with the S/H flag carrying the sign.
- **Open verification for the user (noted, not blocking):** the exact Lexware column variant and DATEV import acceptance should be confirmed on the first real import with the Steuerberater; the IDs can be filled in settings any time.
