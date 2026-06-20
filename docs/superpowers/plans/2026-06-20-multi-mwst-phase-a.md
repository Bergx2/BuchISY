# Mehrere MwSt.-Sätze (Phase A) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A receipt can carry multiple VAT lines (`net / rate% / vat`) plus a Trinkgeld post; entered via a repeatable "+ MwSt." block, extracted by Claude/local heuristics, and round-tripped through CSV and SQLite — while the existing aggregate fields stay valid for old data.

**Architecture:** Introduce a `core.TaxLine` value type and a `[]TaxLine` + `Trinkgeld` on `Meta`/`CSVRow`. Aggregate fields (`BetragNetto`, `SteuersatzBetrag`, `Bruttobetrag`, `SteuersatzProzent`) become derived sums. Detail is persisted as a JSON column `Steuerzeilen` (+ `Trinkgeld`) in CSV and SQLite; loading reconstructs a single line from aggregates when the column is absent (legacy). A reusable Fyne widget renders the repeatable block in both the new-invoice and edit dialogs.

**Tech Stack:** Go 1.25, Fyne v2, modernc.org/sqlite, encoding/json, Anthropic Messages API.

## Global Constraints

- Decimal in CSV always `.`; UI respects `Settings.DecimalSeparator` (`,` default). Copied from CLAUDE.md.
- CSV encoding ISO-8859-1, fields quoted (see `csvrepo.WriteTo`).
- Field renames already in effect: `Auftraggeber`, `Verwendungszweck` (no `Firmenname`/`Kurzbezeichnung`).
- Backward compatibility is mandatory: invoices.csv and SQLite rows written before this change must still load (reconstruct one TaxLine from the aggregate fields).
- All money is `float64` rounded to 2 decimals for display/persistence.
- Tests live next to code as `*_test.go`; run with `go test ./...`. Commit after each task.

---

### Task 1: TaxLine type + summation helpers

**Files:**
- Create: `internal/core/taxline.go`
- Test: `internal/core/taxline_test.go`

**Interfaces:**
- Produces: `type TaxLine struct { Netto, SatzProzent, MwStBetrag float64 }`;
  `SumNetto([]TaxLine) float64`; `SumMwSt([]TaxLine) float64`;
  `ComputeBrutto(lines []TaxLine, trinkgeld float64) float64`;
  `PrimarySatz([]TaxLine) float64`.

- [ ] **Step 1: Write the failing test**

```go
package core

import "testing"

func almost(a, b float64) bool { d := a - b; return d < 0.005 && d > -0.005 }

func TestTaxLineSums(t *testing.T) {
	lines := []TaxLine{
		{Netto: 14.20, SatzProzent: 19, MwStBetrag: 2.70},
		{Netto: 18.69, SatzProzent: 7, MwStBetrag: 1.31},
	}
	if !almost(SumNetto(lines), 32.89) {
		t.Errorf("SumNetto = %v, want 32.89", SumNetto(lines))
	}
	if !almost(SumMwSt(lines), 4.01) {
		t.Errorf("SumMwSt = %v, want 4.01", SumMwSt(lines))
	}
	if !almost(ComputeBrutto(lines, 2.00), 38.90) {
		t.Errorf("ComputeBrutto = %v, want 38.90", ComputeBrutto(lines, 2.00))
	}
	if PrimarySatz(lines) != 19 {
		t.Errorf("PrimarySatz = %v, want 19", PrimarySatz(lines))
	}
	if PrimarySatz(nil) != 0 {
		t.Errorf("PrimarySatz(nil) = %v, want 0", PrimarySatz(nil))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run TestTaxLineSums`
Expected: FAIL (undefined: TaxLine/SumNetto/...).

- [ ] **Step 3: Write minimal implementation**

```go
package core

// TaxLine is one VAT line of a receipt: a net amount taxed at SatzProzent
// percent, yielding MwStBetrag of tax. A receipt may have several.
type TaxLine struct {
	Netto       float64 `json:"netto"`
	SatzProzent float64 `json:"satz_prozent"`
	MwStBetrag  float64 `json:"mwst_betrag"`
}

// SumNetto returns the total net of all lines.
func SumNetto(lines []TaxLine) float64 {
	var s float64
	for _, l := range lines {
		s += l.Netto
	}
	return s
}

// SumMwSt returns the total VAT of all lines.
func SumMwSt(lines []TaxLine) float64 {
	var s float64
	for _, l := range lines {
		s += l.MwStBetrag
	}
	return s
}

// ComputeBrutto returns net + vat over all lines plus the (un-taxed) Trinkgeld.
func ComputeBrutto(lines []TaxLine, trinkgeld float64) float64 {
	return SumNetto(lines) + SumMwSt(lines) + trinkgeld
}

// PrimarySatz returns the VAT rate of the first line (for the legacy
// SteuersatzProzent display field), or 0 when there are no lines.
func PrimarySatz(lines []TaxLine) float64 {
	if len(lines) == 0 {
		return 0
	}
	return lines[0].SatzProzent
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run TestTaxLineSums`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/core/taxline.go internal/core/taxline_test.go
git commit -m "Add TaxLine type and summation helpers"
```

---

### Task 2: TaxLine JSON (de)serialize + reconstruct-from-totals

**Files:**
- Modify: `internal/core/taxline.go`
- Test: `internal/core/taxline_test.go`

**Interfaces:**
- Produces: `MarshalTaxLines([]TaxLine) string` (compact JSON, `""` for empty);
  `ParseTaxLines(string) []TaxLine` (tolerant: `""`/invalid → nil);
  `ReconstructTaxLines(netto, satzProzent, mwst float64) []TaxLine`
  (single line from legacy aggregates; nil when all zero).

- [ ] **Step 1: Write the failing test**

```go
func TestTaxLineJSONAndReconstruct(t *testing.T) {
	lines := []TaxLine{{Netto: 14.20, SatzProzent: 19, MwStBetrag: 2.70}}
	js := MarshalTaxLines(lines)
	got := ParseTaxLines(js)
	if len(got) != 1 || !almost(got[0].Netto, 14.20) || got[0].SatzProzent != 19 {
		t.Fatalf("round-trip failed: %q -> %+v", js, got)
	}
	if MarshalTaxLines(nil) != "" {
		t.Errorf("empty should marshal to empty string")
	}
	if ParseTaxLines("") != nil || ParseTaxLines("not json") != nil {
		t.Errorf("invalid JSON should parse to nil")
	}
	rc := ReconstructTaxLines(14.20, 19, 2.70)
	if len(rc) != 1 || rc[0].SatzProzent != 19 {
		t.Errorf("reconstruct = %+v", rc)
	}
	if ReconstructTaxLines(0, 0, 0) != nil {
		t.Errorf("all-zero reconstruct should be nil")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run TestTaxLineJSONAndReconstruct`
Expected: FAIL (undefined functions).

- [ ] **Step 3: Write minimal implementation**

Add to `internal/core/taxline.go` (add `import "encoding/json"` at top):

```go
// MarshalTaxLines encodes lines as compact JSON; empty input yields "".
func MarshalTaxLines(lines []TaxLine) string {
	if len(lines) == 0 {
		return ""
	}
	b, err := json.Marshal(lines)
	if err != nil {
		return ""
	}
	return string(b)
}

// ParseTaxLines decodes lines from JSON; "" or invalid input yields nil.
func ParseTaxLines(s string) []TaxLine {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var lines []TaxLine
	if err := json.Unmarshal([]byte(s), &lines); err != nil {
		return nil
	}
	return lines
}

// ReconstructTaxLines builds a single TaxLine from the legacy aggregate
// fields, used when a row has no Steuerzeilen detail. Returns nil if the
// aggregates are all zero.
func ReconstructTaxLines(netto, satzProzent, mwst float64) []TaxLine {
	if netto == 0 && satzProzent == 0 && mwst == 0 {
		return nil
	}
	return []TaxLine{{Netto: netto, SatzProzent: satzProzent, MwStBetrag: mwst}}
}
```

Add `"strings"` to the import block of `taxline.go`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run TestTaxLineJSONAndReconstruct`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/core/taxline.go internal/core/taxline_test.go
git commit -m "Add TaxLine JSON (de)serialize and legacy reconstruct"
```

---

### Task 3: Add TaxLines + Trinkgeld to Meta and CSVRow

**Files:**
- Modify: `internal/core/types.go` (Meta struct, CSVRow struct, `ToCSVRow`, `ToMeta`)
- Test: `internal/core/types_test.go` (create)

**Interfaces:**
- Consumes: `TaxLine` (Task 1).
- Produces: `Meta.TaxLines []TaxLine`, `Meta.Trinkgeld float64`,
  `CSVRow.TaxLines []TaxLine`, `CSVRow.Trinkgeld float64`, carried by
  `Meta.ToCSVRow()` and `CSVRow.ToMeta()`.

- [ ] **Step 1: Write the failing test**

```go
package core

import "testing"

func TestMetaTaxLinesRoundTrip(t *testing.T) {
	m := Meta{
		Auftraggeber: "Restaurant",
		TaxLines:     []TaxLine{{Netto: 14.20, SatzProzent: 19, MwStBetrag: 2.70}},
		Trinkgeld:    2.00,
	}
	row := m.ToCSVRow()
	if len(row.TaxLines) != 1 || row.Trinkgeld != 2.00 {
		t.Fatalf("ToCSVRow lost detail: %+v", row)
	}
	back := row.ToMeta()
	if len(back.TaxLines) != 1 || back.Trinkgeld != 2.00 {
		t.Fatalf("ToMeta lost detail: %+v", back)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run TestMetaTaxLinesRoundTrip`
Expected: FAIL (unknown field TaxLines).

- [ ] **Step 3: Write minimal implementation**

In `internal/core/types.go`, add to the `Meta` struct (after `Bruttobetrag`):

```go
	TaxLines  []TaxLine // VAT lines; aggregates above are their sums
	Trinkgeld float64   // tip, no VAT, only part of Bruttobetrag
```

Add the same two fields to `CSVRow` (after `Bruttobetrag`). In `ToCSVRow()` add:

```go
		TaxLines:  m.TaxLines,
		Trinkgeld: m.Trinkgeld,
```

In `ToMeta()` add:

```go
		TaxLines:  r.TaxLines,
		Trinkgeld: r.Trinkgeld,
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run TestMetaTaxLinesRoundTrip && go build ./...`
Expected: PASS and build succeeds.

- [ ] **Step 5: Commit**

```bash
git add internal/core/types.go internal/core/types_test.go
git commit -m "Carry TaxLines and Trinkgeld on Meta and CSVRow"
```

---

### Task 4: CSV columns Steuerzeilen + Trinkgeld (load/save + legacy)

**Files:**
- Modify: `internal/core/csvrepo.go` (DefaultCSVColumns, ColumnDisplayNames, ColumnTranslationKeys, Load loop, rowToRecord)
- Test: `internal/core/csvrepo_test.go`

**Interfaces:**
- Consumes: `MarshalTaxLines`, `ParseTaxLines`, `ReconstructTaxLines`, `SumNetto`, `SumMwSt`, `ComputeBrutto`, `PrimarySatz` (Tasks 1–2).
- Produces: CSV columns `"Steuerzeilen"`, `"Trinkgeld"`; loaded rows have
  `TaxLines`/`Trinkgeld` populated (reconstructed from aggregates if column absent).

- [ ] **Step 1: Write the failing test**

```go
func TestCSVTaxLinesRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invoices.csv")
	repo := NewCSVRepository()
	row := CSVRow{
		Dateiname: "a.pdf", Jahr: "2026", Monat: "06",
		BetragNetto: 32.89, SteuersatzBetrag: 4.01, Bruttobetrag: 38.90,
		TaxLines: []TaxLine{
			{Netto: 14.20, SatzProzent: 19, MwStBetrag: 2.70},
			{Netto: 18.69, SatzProzent: 7, MwStBetrag: 1.31},
		},
		Trinkgeld: 2.00,
	}
	if err := repo.Append(path, row); err != nil {
		t.Fatal(err)
	}
	rows, err := repo.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || len(rows[0].TaxLines) != 2 || rows[0].Trinkgeld != 2.00 {
		t.Fatalf("tax lines not round-tripped: %+v", rows)
	}
}

func TestCSVLegacyReconstructsTaxLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invoices.csv")
	legacy := "Dateiname,BetragNetto,Steuersatz_Prozent,Steuersatz_Betrag\nalt.pdf,10.00,19,1.90\n"
	if err := os.WriteFile(path, []byte(legacy), 0644); err != nil {
		t.Fatal(err)
	}
	rows, err := NewCSVRepository().Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || len(rows[0].TaxLines) != 1 || rows[0].TaxLines[0].SatzProzent != 19 {
		t.Fatalf("legacy row should reconstruct one TaxLine: %+v", rows)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run 'TestCSVTaxLinesRoundTrip|TestCSVLegacyReconstructsTaxLine'`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

In `DefaultCSVColumns` add after `"AnzahlAnhaenge"` (keep order stable):

```go
	"Trinkgeld",
	"Steuerzeilen",
```

In `ColumnDisplayNames` add:

```go
	"Trinkgeld":    "Trinkgeld",
	"Steuerzeilen": "Steuerzeilen (Detail)",
```

In `ColumnTranslationKeys` add:

```go
	"Trinkgeld":    "table.col.trinkgeld",
	"Steuerzeilen": "table.col.taxlines",
```

In the `Load` loop, after the `row := CSVRow{...}` literal is built and before
`rows = append(...)`, add:

```go
		row.Trinkgeld = parseFloat(valueForColumn(record, headerMap, "Trinkgeld"))
		row.TaxLines = ParseTaxLines(valueForColumn(record, headerMap, "Steuerzeilen"))
		if len(row.TaxLines) == 0 {
			// Legacy row without detail: reconstruct one line from aggregates.
			row.TaxLines = ReconstructTaxLines(row.BetragNetto, row.SteuersatzProzent, row.SteuersatzBetrag)
		}
```

In `rowToRecord`'s `valueMap`, add:

```go
		"Trinkgeld":    r.formatFloat(row.Trinkgeld),
		"Steuerzeilen": MarshalTaxLines(row.TaxLines),
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run 'TestCSV' && go build ./...`
Expected: PASS.

- [ ] **Step 5: Add i18n keys + Commit**

Add to `assets/i18n/de.json`: `"table.col.trinkgeld": "Trinkgeld",` and `"table.col.taxlines": "Steuerzeilen",`. To `assets/i18n/en.json`: `"table.col.trinkgeld": "Tip",` and `"table.col.taxlines": "VAT lines",`. Then:

```bash
git add internal/core/csvrepo.go internal/core/csvrepo_test.go assets/i18n/de.json assets/i18n/en.json
git commit -m "Persist TaxLines + Trinkgeld in CSV with legacy reconstruct"
```

---

### Task 5: SQLite columns + migration + insert/update/scan

**Files:**
- Modify: `internal/db/schema.go` (add columns to CREATE TABLE)
- Modify: `internal/db/repository.go` (`initSchema` add idempotent ALTER; `Insert`, `Update`, `List` scan)
- Test: `internal/db/repository_test.go`

**Interfaces:**
- Consumes: `core.CSVRow.TaxLines`/`Trinkgeld`, `core.MarshalTaxLines`, `core.ParseTaxLines`.
- Produces: columns `trinkgeld REAL`, `steuerzeilen TEXT`; round-trip via `Insert`/`List`.

- [ ] **Step 1: Write the failing test**

```go
func TestDBTaxLinesRoundTrip(t *testing.T) {
	repo := newTestRepo(t)
	row := core.CSVRow{
		Dateiname: "a.pdf", Jahr: "2026", Monat: "06",
		TaxLines: []core.TaxLine{
			{Netto: 14.20, SatzProzent: 19, MwStBetrag: 2.70},
			{Netto: 18.69, SatzProzent: 7, MwStBetrag: 1.31},
		},
		Trinkgeld: 2.00,
	}
	if _, err := repo.Insert(row); err != nil {
		t.Fatal(err)
	}
	rows, err := repo.List("2026", "06")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || len(rows[0].TaxLines) != 2 || rows[0].Trinkgeld != 2.00 {
		t.Fatalf("DB did not round-trip tax lines: %+v", rows)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/db/ -run TestDBTaxLinesRoundTrip`
Expected: FAIL (no such column).

- [ ] **Step 3: Write minimal implementation**

In `internal/db/schema.go`, inside the `CREATE TABLE`, add after `ustidnr TEXT,`:

```sql
	trinkgeld REAL,
	steuerzeilen TEXT,
```

In `internal/db/repository.go`, add an idempotent column-migration in `initSchema`
(after the schema exec). First read `initSchema` to find the exec; then append:

```go
	// Add columns introduced after the initial schema (idempotent).
	for _, col := range []string{
		"ALTER TABLE invoices ADD COLUMN trinkgeld REAL",
		"ALTER TABLE invoices ADD COLUMN steuerzeilen TEXT",
	} {
		if _, err := r.db.Exec(col); err != nil &&
			!strings.Contains(err.Error(), "duplicate column name") {
			return fmt.Errorf("failed to add column: %w", err)
		}
	}
	return nil
```

(Add `"strings"` to repository.go imports if missing.)

In `Insert`: add `trinkgeld, steuerzeilen` to the column list and two more `?`
placeholders, and to the args: `row.Trinkgeld, core.MarshalTaxLines(row.TaxLines),`.

In `Update`: add `trinkgeld = ?, steuerzeilen = ?,` to the SET (before `jahr = ?`),
and `row.Trinkgeld, core.MarshalTaxLines(row.TaxLines),` to the args before `row.Jahr`.

In `List`'s SELECT column list and `rows.Scan`: add `trinkgeld, steuerzeilen` and scan
into a temp: declare `var steuerzeilen string` and `&row.Trinkgeld, &steuerzeilen`,
then after scanning set:

```go
		row.TaxLines = core.ParseTaxLines(steuerzeilen)
		if len(row.TaxLines) == 0 {
			row.TaxLines = core.ReconstructTaxLines(row.BetragNetto, row.SteuersatzProzent, row.SteuersatzBetrag)
		}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/db/ -run TestDBTaxLinesRoundTrip && go build ./...`
Expected: PASS. (Existing DBs get the columns via the ALTER migration.)

- [ ] **Step 5: Commit**

```bash
git add internal/db/schema.go internal/db/repository.go internal/db/repository_test.go
git commit -m "Persist TaxLines + Trinkgeld in SQLite with column migration"
```

---

### Task 6: Extraction — Claude array + local single line

**Files:**
- Modify: `internal/anthropic/extractor.go` (prompt JSON schema + result struct + mapping)
- Modify: `internal/core/localextract.go` (populate one TaxLine + aggregates)
- Test: `internal/anthropic/extractor_test.go`

**Interfaces:**
- Consumes: `core.TaxLine`, `core.SumNetto/SumMwSt/ComputeBrutto/PrimarySatz`.
- Produces: extracted `Meta.TaxLines` + `Meta.Trinkgeld`; aggregates set from the lines.

- [ ] **Step 1: Write the failing test**

Add a test that feeds a JSON response (the extractor has an internal parse path;
test the smallest unit you can — if parsing is private, add a thin exported
`parseExtractionJSON(string, ownVATIDs []string) (core.Meta, error)` wrapper and test it):

```go
func TestParseMultipleTaxLines(t *testing.T) {
	js := `{"auftraggeber":"R","steuerzeilen":[{"satz":19,"netto":14.20,"mwst":2.70},{"satz":7,"netto":18.69,"mwst":1.31}],"trinkgeld":2.00}`
	meta, err := parseExtractionJSON(js, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(meta.TaxLines) != 2 || meta.Trinkgeld != 2.00 {
		t.Fatalf("lines = %+v trinkgeld=%v", meta.TaxLines, meta.Trinkgeld)
	}
	if !almostA(meta.BetragNetto, 32.89) || !almostA(meta.Bruttobetrag, 38.90) {
		t.Errorf("aggregates wrong: netto=%v brutto=%v", meta.BetragNetto, meta.Bruttobetrag)
	}
}
```

(Add `func almostA(a, b float64) bool` helper in the test file.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/anthropic/ -run TestParseMultipleTaxLines`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

In the prompt JSON schema (extractor.go), replace the single
`betragnetto/steuersatz_prozent/steuersatz_betrag` description with an array plus tip:

```
  "steuerzeilen": [{"satz": 0.0, "netto": 0.0, "mwst": 0.0}],
  "trinkgeld": 0.0,
```

and add a rule line: `- steuerzeilen: je MwSt.-Satz eine Zeile (satz %, netto, mwst). Bei nur einem Satz genau eine Zeile. trinkgeld: separat ohne MwSt., 0 wenn keins.`

Refactor the response parsing into an exported helper so it is testable:

```go
func parseExtractionJSON(response string, ownVATIDs []string) (core.Meta, error) {
	response = cleanJSONResponse(response)
	var result struct {
		Auftraggeber     *string `json:"auftraggeber"`
		Verwendungszweck *string `json:"verwendungszweck"`
		Rechnungsnummer  *string `json:"rechnungsnummer"`
		VATID            *string `json:"vat_id"`
		Steuerzeilen     []struct {
			Satz  float64 `json:"satz"`
			Netto float64 `json:"netto"`
			MwSt  float64 `json:"mwst"`
		} `json:"steuerzeilen"`
		Trinkgeld   *float64 `json:"trinkgeld"`
		Bruttobetrag *float64 `json:"bruttobetrag"`
		Waehrung    *string  `json:"waehrung"`
		Rechnungsdatum *string `json:"rechnungsdatum"`
		Jahr        *string  `json:"jahr"`
		Monat       *string  `json:"monat"`
	}
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		return core.Meta{}, fmt.Errorf("failed to parse JSON response: %w (response: %s)", err, response)
	}
	meta := core.Meta{}
	// ... copy the existing scalar assignments (Auftraggeber, Verwendungszweck via
	// core.NormalizeVerwendungszweck, Rechnungsnummer, VATID with own-id filtering,
	// Waehrung, Rechnungsdatum, Jahr, Monat) unchanged ...
	for _, z := range result.Steuerzeilen {
		meta.TaxLines = append(meta.TaxLines, core.TaxLine{Netto: z.Netto, SatzProzent: z.Satz, MwStBetrag: z.MwSt})
	}
	if result.Trinkgeld != nil {
		meta.Trinkgeld = *result.Trinkgeld
	}
	meta.BetragNetto = core.SumNetto(meta.TaxLines)
	meta.SteuersatzBetrag = core.SumMwSt(meta.TaxLines)
	meta.SteuersatzProzent = core.PrimarySatz(meta.TaxLines)
	if result.Bruttobetrag != nil && *result.Bruttobetrag > 0 {
		meta.Bruttobetrag = *result.Bruttobetrag
	} else {
		meta.Bruttobetrag = core.ComputeBrutto(meta.TaxLines, meta.Trinkgeld)
	}
	return meta, nil
}
```

Call `parseExtractionJSON` from the existing extract method(s) in place of the inline
unmarshal. Keep the own-VAT-ID filtering for `VATID`.

In `internal/core/localextract.go`, after the heuristics compute `BetragNetto`,
`SteuersatzProzent`, `SteuersatzBetrag`, set:

```go
	meta.TaxLines = ReconstructTaxLines(meta.BetragNetto, meta.SteuersatzProzent, meta.SteuersatzBetrag)
	if meta.Bruttobetrag == 0 {
		meta.Bruttobetrag = ComputeBrutto(meta.TaxLines, meta.Trinkgeld)
	}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/anthropic/ ./internal/core/ && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/anthropic/extractor.go internal/core/localextract.go internal/anthropic/extractor_test.go
git commit -m "Extract multiple VAT lines and Trinkgeld"
```

---

### Task 7: Reusable TaxLines editor widget

**Files:**
- Create: `internal/ui/taxlineseditor.go`

**Interfaces:**
- Consumes: `core.TaxLine`, `Settings.DecimalSeparator`, `parseDecimal`/`formatDecimal`
  helpers already in `internal/ui` (used by the modal), `amountcompute.go`.
- Produces: `type taxLinesEditor struct{ ... }`;
  `newTaxLinesEditor(a *App, lines []core.TaxLine, trinkgeld float64, onChange func()) *taxLinesEditor`;
  methods `Container() fyne.CanvasObject`, `Lines() []core.TaxLine`, `Trinkgeld() float64`,
  `Brutto() float64`.

- [ ] **Step 1: Implement the widget**

Create `internal/ui/taxlineseditor.go`. It holds a `*fyne.Container` (VBox) with one
row per `core.TaxLine` (three `widget.Entry`: Netto, Satz %, MwSt.), an `✕` button per
row, a `+ MwSt.` button, a Trinkgeld entry, and a read-only Brutto label. On every
change it recomputes Brutto via `core.ComputeBrutto(e.Lines(), e.Trinkgeld())` and calls
`onChange`. `Lines()` reads the entries back into `[]core.TaxLine` using the existing
decimal parsing helper. Provide an `addLine()` that appends an empty row and refreshes.
Numbers are formatted/parsed with the app's `DecimalSeparator`.

Reference the existing `internal/ui/amountcompute.go` for auto-completing MwSt. from
Netto×Satz (reuse if a helper exists; otherwise compute `mwst = round2(netto*satz/100)`
when the MwSt. field is empty).

(Full widget code — model it on existing `selectableform.go`/`copyablelabel.go` widget
style in this package; keep it under ~150 lines, one responsibility.)

- [ ] **Step 2: Build to verify it compiles**

Run: `go build ./internal/ui/`
Expected: success.

- [ ] **Step 3: Commit**

```bash
git add internal/ui/taxlineseditor.go
git commit -m "Add reusable repeatable VAT-lines editor widget"
```

---

### Task 8: Wire editor into the new-invoice modal

**Files:**
- Modify: `internal/ui/invoicemodal.go` (replace single net/vat%/vat-amount/gross fields with the editor; read TaxLines/Trinkgeld into the saved Meta)

**Interfaces:**
- Consumes: `newTaxLinesEditor` (Task 7).
- Produces: `saveInvoice` path persists `Meta.TaxLines`/`Trinkgeld`; gross is the editor sum.

- [ ] **Step 1: Replace the VAT block**

In `showConfirmationModal`, replace the `netEntry`/`vatPercentEntry`/`vatAmountEntry`/
`grossEntry` widgets with `ed := newTaxLinesEditor(a, meta.TaxLines, meta.Trinkgeld, updatePreview)`.
Put `ed.Container()` into the form where the old fields were. Keep the currency-conversion
container untouched.

- [ ] **Step 2: Read values on save**

Where the new `core.Meta` is built for saving, set:

```go
		TaxLines:          ed.Lines(),
		Trinkgeld:         ed.Trinkgeld(),
		BetragNetto:       core.SumNetto(ed.Lines()),
		SteuersatzBetrag:  core.SumMwSt(ed.Lines()),
		SteuersatzProzent: core.PrimarySatz(ed.Lines()),
		Bruttobetrag:      ed.Brutto(),
```

Remove now-unused references to the deleted entries (filename preview that read
`grossEntry.Text` should read `ed.Brutto()`).

- [ ] **Step 3: Build + manual smoke**

Run: `go build ./... && go vet ./internal/ui/`
Expected: success. Then `go run ./cmd/buchisy` and confirm the modal shows the repeatable
block (manual check; no automated UI test).

- [ ] **Step 4: Commit**

```bash
git add internal/ui/invoicemodal.go
git commit -m "Use repeatable VAT-lines editor in the new-invoice modal"
```

---

### Task 9: Wire editor into the edit dialog

**Files:**
- Modify: `internal/ui/tableedit.go` (same replacement as Task 8 for `showEditDialog`/`updateInvoice`)

**Interfaces:**
- Consumes: `newTaxLinesEditor` (Task 7).
- Produces: `updateInvoice` persists edited `TaxLines`/`Trinkgeld`.

- [ ] **Step 1: Replace the VAT block in the edit dialog**

In `showEditDialog`, prefill `ed := newTaxLinesEditor(a, meta.TaxLines, meta.Trinkgeld, updateFilenamePreview)`
(meta comes from `row.ToMeta()`, which now carries TaxLines). Replace the four single
fields with `ed.Container()`.

- [ ] **Step 2: Pass values into updateInvoice**

Change `updateInvoice`'s signature to accept `taxLines []core.TaxLine, trinkgeld float64`
instead of `net, vatPercent, vatAmount, gross float64`; inside build `newMeta` with:

```go
		TaxLines:          taxLines,
		Trinkgeld:         trinkgeld,
		BetragNetto:       core.SumNetto(taxLines),
		SteuersatzProzent: core.PrimarySatz(taxLines),
		SteuersatzBetrag:  core.SumMwSt(taxLines),
		Bruttobetrag:      core.ComputeBrutto(taxLines, trinkgeld),
```

Update the call site to pass `ed.Lines(), ed.Trinkgeld()`.

- [ ] **Step 3: Build + manual smoke**

Run: `go build ./... && go test ./...`
Expected: success. Manually edit an existing invoice and confirm lines load/save.

- [ ] **Step 4: Commit**

```bash
git add internal/ui/tableedit.go
git commit -m "Use repeatable VAT-lines editor in the edit dialog"
```

---

## Self-Review

- **Spec coverage (Phase A):** TaxLine model (T1–2), Meta/CSVRow carry (T3), CSV persist + legacy (T4), SQLite persist + migration (T5), multi-line extraction + local single line (T6), repeatable UI in both dialogs incl. Trinkgeld + live Brutto (T7–9). Aggregates stay valid (T3–6). Covered.
- **Placeholders:** Task 7's widget body is described rather than fully transcribed because exact Fyne layout mirrors existing widgets in the package; all data-flow signatures are fixed. Tasks 1–6 contain complete code.
- **Type consistency:** `TaxLine{Netto, SatzProzent, MwStBetrag}`, `SumNetto/SumMwSt/ComputeBrutto/PrimarySatz`, `MarshalTaxLines/ParseTaxLines/ReconstructTaxLines`, editor `Lines()/Trinkgeld()/Brutto()` used consistently across tasks.
- **Out of scope (later phases):** table column for multiple rates beyond the existing summary; DATEV/Lexware export; SKR04; booking engine — covered by phases B/C/D plans.
