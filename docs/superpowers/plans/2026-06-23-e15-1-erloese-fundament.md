# E15.1 — Erlöse/Ausgangsrechnungen Fundament — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make an Ausgangsrechnung (outgoing invoice) produce a correct revenue booking (Soll payment, Haben Erlöskonto + Umsatzsteuer) and have DATEV/Lexware export it correctly in both directions.

**Architecture:** Mirror the existing expense path. A new `Ausgangsrechnung` flag on the invoice drives a new `BuildRevenueBooking` (the mirror of `BuildBooking`) using per-profile `umsatzsteuer_konten`. The two exporters are generalized via a new `Booking.PaymentAndCounters(isRevenue)` that returns the single payment/base entry plus its counter entries, so one code path serves incoming (1 Haben base) and outgoing (1 Soll base).

**Tech Stack:** Go 1.25, Fyne, modernc.org/sqlite. Packages: `internal/core`, `internal/db`, `internal/ui`.

## Global Constraints

- Go 1.25; CGO enabled (Fyne). Build: `go build ./...`; test: `go test ./...`.
- Decimal handling via existing `round2`; amounts rounded to 2 places.
- DB migrations are idempotent `ALTER TABLE ... ADD COLUMN`, errors swallowed on `"duplicate column name"` (see `repository.go:initSchema`).
- New struct fields are threaded through BOTH `Meta.ToCSVRow` and `CSVRow.ToMeta` (a dropped copy silently loses data on round-trip).
- CSV columns must be added to all five parallel structures in `csvrepo.go` (DefaultCSVColumns, ColumnDisplayNames, ColumnTranslationKeys, Load read path, rowToRecord valueMap).
- Commit after each task. Co-author trailer: `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`.
- Branch: `feat/e15-erloese` (already created; spec committed there).

---

### Task 1: `Ausgangsrechnung` flag on the domain model

**Files:**
- Modify: `internal/core/types.go` (Meta struct ~line 10, CSVRow struct ~line 172, ToCSVRow ~line 206, ToMeta ~line 246)
- Test: `internal/core/types_test.go`

**Interfaces:**
- Produces: `Meta.Ausgangsrechnung bool`, `CSVRow.Ausgangsrechnung bool`, copied by `Meta.ToCSVRow()` and `CSVRow.ToMeta()`.

- [ ] **Step 1: Write the failing test** — append to `internal/core/types_test.go`:

```go
func TestAusgangsrechnungRoundTrip(t *testing.T) {
	m := Meta{Auftraggeber: "Kunde", Ausgangsrechnung: true}
	if !m.ToCSVRow().Ausgangsrechnung {
		t.Error("ToCSVRow dropped Ausgangsrechnung")
	}
	r := CSVRow{Auftraggeber: "Kunde", Ausgangsrechnung: true}
	if !r.ToMeta().Ausgangsrechnung {
		t.Error("ToMeta dropped Ausgangsrechnung")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run TestAusgangsrechnungRoundTrip`
Expected: FAIL — `unknown field Ausgangsrechnung`.

- [ ] **Step 3: Add the field + conversions**

In `types.go`, add to `Meta` (near `Teilzahlung`): `Ausgangsrechnung bool // true = outgoing/revenue invoice (Erlös)`.
Add to `CSVRow` (near `Teilzahlung`): `Ausgangsrechnung bool`.
In `ToCSVRow` return literal add: `Ausgangsrechnung:   m.Ausgangsrechnung,`.
In `ToMeta` return literal add: `Ausgangsrechnung:   r.Ausgangsrechnung,`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run TestAusgangsrechnungRoundTrip`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/core/types.go internal/core/types_test.go
git commit -m "E15.1: add Ausgangsrechnung flag to Meta/CSVRow"
```

---

### Task 2: DB column + migration + CRUD threading

**Files:**
- Modify: `internal/db/schema.go` (CREATE TABLE ~line 33)
- Modify: `internal/db/repository.go` (migration list ~line 80; Insert ~line 92-120; Update ~line 138-198; List SELECT ~line 240-248 + scan ~line 274-290)
- Test: `internal/db/belegnummer_test.go` (add a test) or new `internal/db/ausgangsrechnung_test.go`

**Interfaces:**
- Consumes: `core.CSVRow.Ausgangsrechnung` (Task 1).
- Produces: persisted `ausgangsrechnung` column; `List` populates `row.Ausgangsrechnung`.

- [ ] **Step 1: Write the failing test** — create `internal/db/ausgangsrechnung_test.go`:

```go
package db

import (
	"testing"

	"github.com/bergx2/buchisy/internal/core"
)

func TestAusgangsrechnungPersists(t *testing.T) {
	repo := newTestRepo(t)
	if _, err := repo.Insert(core.CSVRow{Dateiname: "out.pdf", Jahr: "2026", Monat: "06", Ausgangsrechnung: true}); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	rows, err := repo.List("2026", "06")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || !rows[0].Ausgangsrechnung {
		t.Fatalf("Ausgangsrechnung not persisted: %+v", rows)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/db/ -run TestAusgangsrechnungPersists`
Expected: FAIL — `no such column: ausgangsrechnung` (List/Insert reference it after edits) or scan mismatch. (Before edits it fails to compile only if column referenced; expect it to fail once Step 3 partially done — so run after Step 3 too.)

- [ ] **Step 3: Schema + migration + CRUD**

In `schema.go` CREATE TABLE, after `belegnummer TEXT DEFAULT '',` add:
```
	ausgangsrechnung INTEGER DEFAULT 0,
```
In `repository.go` migration slice (after the belegnummer ALTER) add:
```go
		"ALTER TABLE invoices ADD COLUMN ausgangsrechnung INTEGER DEFAULT 0",
```
Insert: add `ausgangsrechnung` to the column list, one more `?`, and `row.Ausgangsrechnung` to args (last position, after `row.Belegnummer`).
Update: add `ausgangsrechnung = ?,` to SET and `row.Ausgangsrechnung` to args (after `row.Belegnummer`).
List: add `ausgangsrechnung` to SELECT; declare `var ausgangsrechnung sql.NullInt64`; add `&ausgangsrechnung` to `Scan` (after `&belegnummer`); after scan: `row.Ausgangsrechnung = ausgangsrechnung.Int64 != 0`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/db/ -run TestAusgangsrechnungPersists`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/db/schema.go internal/db/repository.go internal/db/ausgangsrechnung_test.go
git commit -m "E15.1: persist ausgangsrechnung column (schema, migration, CRUD)"
```

---

### Task 3: CSV column + table column + i18n

**Files:**
- Modify: `internal/core/csvrepo.go` (DefaultCSVColumns ~16, ColumnDisplayNames ~50, ColumnTranslationKeys ~84, Load ~263, rowToRecord ~466)
- Modify: `internal/ui/table.go` (valueForColumn ~848, columnWidthMap ~162)
- Modify: `assets/i18n/de.json`, `assets/i18n/en.json`
- Test: `internal/core/csvrepo_test.go`

**Interfaces:**
- Consumes: `core.CSVRow.Ausgangsrechnung`.
- Produces: CSV column id `"Ausgangsrechnung"`.

- [ ] **Step 1: Write the failing test** — append to `internal/core/csvrepo_test.go`:

```go
func TestCSVAusgangsrechnungColumn(t *testing.T) {
	r := NewCSVRepository()
	dir := t.TempDir()
	path := dir + "/invoices.csv"
	if err := r.Rewrite(path, []CSVRow{{Dateiname: "a.pdf", Ausgangsrechnung: true}}); err != nil {
		t.Fatal(err)
	}
	rows, err := r.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || !rows[0].Ausgangsrechnung {
		t.Fatalf("Ausgangsrechnung not round-tripped via CSV: %+v", rows)
	}
}
```
(If `NewCSVRepository`/`Rewrite` signatures differ, mirror an existing test in this file.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run TestCSVAusgangsrechnungColumn`
Expected: FAIL — column not written/read, `Ausgangsrechnung` stays false.

- [ ] **Step 3: Add the column everywhere**

`csvrepo.go`:
- `DefaultCSVColumns`: add `"Ausgangsrechnung",` (place after `"Teilzahlung",`).
- `ColumnDisplayNames`: `"Ausgangsrechnung": "Ausgangsrechnung",`.
- `ColumnTranslationKeys`: `"Ausgangsrechnung": "table.col.ausgangsrechnung",`.
- `Load` `CSVRow{...}` literal: add `Ausgangsrechnung: parseBool(valueForColumn(record, headerMap, "Ausgangsrechnung")),`.
- `rowToRecord` valueMap: `"Ausgangsrechnung": formatBool(row.Ausgangsrechnung),`.

`table.go`:
- `valueForColumn` switch: `case "Ausgangsrechnung":` → `if row.Ausgangsrechnung { return "✓" }; return ""`.
- `columnWidthMap`: `"Ausgangsrechnung": 110,`.

`assets/i18n/de.json`: add `"table.col.ausgangsrechnung": "Ausgangsrechnung",` (next to `table.col.belegnummer`).
`assets/i18n/en.json`: add `"table.col.ausgangsrechnung": "Outgoing invoice",`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run TestCSVAusgangsrechnungColumn && go build ./...`
Expected: PASS + build OK.

- [ ] **Step 5: Commit**

```bash
git add internal/core/csvrepo.go internal/ui/table.go assets/i18n/de.json assets/i18n/en.json internal/core/csvrepo_test.go
git commit -m "E15.1: Ausgangsrechnung CSV + table column + i18n"
```

---

### Task 4: `umsatzsteuer_konten` config + lookup + merge

**Files:**
- Modify: `internal/core/buchungsregeln.go` (BookingRules struct ~27; add `UmsatzsteuerKonto`)
- Modify: `internal/core/bookingrulesstore.go` (`mergeBundledIntoSaved` ~48)
- Modify: `assets/buchungsregeln.json`
- Test: `internal/core/buchungsregeln_test.go` (create if absent) or `bookingrulesstore_test.go`

**Interfaces:**
- Produces: `BookingRules.UmsatzsteuerKonten map[string]int` (json `umsatzsteuer_konten`) and `BookingRules.UmsatzsteuerKonto(satzProzent float64) (int, bool)`.

- [ ] **Step 1: Write the failing test** — append to `internal/core/bookingrulesstore_test.go`:

```go
func TestUmsatzsteuerKonto(t *testing.T) {
	r := &BookingRules{UmsatzsteuerKonten: map[string]int{"19": 1776}}
	if k, ok := r.UmsatzsteuerKonto(19); !ok || k != 1776 {
		t.Errorf("UmsatzsteuerKonto(19) = %d,%v, want 1776,true", k, ok)
	}
	if _, ok := r.UmsatzsteuerKonto(7); ok {
		t.Error("UmsatzsteuerKonto(7) should be (0,false) when unset")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run TestUmsatzsteuerKonto`
Expected: FAIL — `UmsatzsteuerKonten`/`UmsatzsteuerKonto` undefined.

- [ ] **Step 3: Add field, lookup, merge, bundled default**

`buchungsregeln.go` `BookingRules` struct: add field
```go
	UmsatzsteuerKonten map[string]int `json:"umsatzsteuer_konten,omitempty"`
```
Add method (mirror `VorsteuerKonto`):
```go
// UmsatzsteuerKonto returns the output-VAT account for a VAT rate (percent).
func (r *BookingRules) UmsatzsteuerKonto(satzProzent float64) (int, bool) {
	k, ok := r.UmsatzsteuerKonten[strconv.Itoa(int(satzProzent+0.5))]
	return k, ok
}
```
`bookingrulesstore.go` `mergeBundledIntoSaved`: after the vorsteuer merge loop, add the mirror (gap-fill saved from bundled):
```go
	if saved.UmsatzsteuerKonten == nil {
		saved.UmsatzsteuerKonten = map[string]int{}
	}
	for k, v := range bundled.UmsatzsteuerKonten {
		if _, ok := saved.UmsatzsteuerKonten[k]; !ok {
			saved.UmsatzsteuerKonten[k] = v
		}
	}
```
`assets/buchungsregeln.json`: add top-level (SKR04 seed, mirrors vorsteuer 1406/1401):
```json
  "umsatzsteuer_konten": { "19": 3806, "7": 3801 },
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run 'TestUmsatzsteuerKonto|TestBookingRules'`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/core/buchungsregeln.go internal/core/bookingrulesstore.go assets/buchungsregeln.json internal/core/bookingrulesstore_test.go
git commit -m "E15.1: per-profile umsatzsteuer_konten config + lookup + merge"
```

---

### Task 5: `BuildRevenueBooking`

**Files:**
- Modify: `internal/core/buchung.go` (add function after `BuildBooking`)
- Test: `internal/core/buchung_test.go`

**Interfaces:**
- Consumes: `BookingRules.UmsatzsteuerKonto` (Task 4), `SumNetto`, `SumMwSt`, `round2`, `TaxLine`.
- Produces: `BuildRevenueBooking(rules *BookingRules, lines []TaxLine, revenueAccount, paymentAccount int) (Booking, error)`.

- [ ] **Step 1: Write the failing test** — append to `internal/core/buchung_test.go`:

```go
func TestBuildRevenueBooking(t *testing.T) {
	rules := &BookingRules{UmsatzsteuerKonten: map[string]int{"19": 1776}}
	lines := []TaxLine{{Netto: 6500, SatzProzent: 19, MwStBetrag: 1235}}
	b, err := BuildRevenueBooking(rules, lines, 8400, 1200)
	if err != nil {
		t.Fatal(err)
	}
	if !b.Balanced() {
		t.Fatalf("not balanced: %+v", b.Entries)
	}
	// Soll payment = gross 7735; Haben Erlös 6500 + USt 1235.
	want := map[int]struct {
		betrag float64
		soll   bool
	}{1200: {7735, true}, 8400: {6500, false}, 1776: {1235, false}}
	if len(b.Entries) != 3 {
		t.Fatalf("got %d entries, want 3: %+v", len(b.Entries), b.Entries)
	}
	for _, e := range b.Entries {
		w, ok := want[e.Konto]
		if !ok || e.Betrag != w.betrag || e.Soll != w.soll {
			t.Errorf("entry %+v unexpected", e)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run TestBuildRevenueBooking`
Expected: FAIL — `BuildRevenueBooking` undefined.

- [ ] **Step 3: Implement** — add to `buchung.go`:

```go
// BuildRevenueBooking turns an outgoing invoice's tax lines into a balanced
// revenue Booking: Soll paymentAccount (gross received), Haben revenueAccount
// (net) + Umsatzsteuer per rate. The mirror of BuildBooking. paymentAccount is
// computed as the sum of the Haben side, so the booking always balances even if
// a rate's Umsatzsteuer account is unconfigured.
func BuildRevenueBooking(rules *BookingRules, lines []TaxLine, revenueAccount, paymentAccount int) (Booking, error) {
	if len(lines) == 0 {
		return Booking{}, fmt.Errorf("keine Steuerzeilen für Erlösbuchung")
	}
	entries := []BookingEntry{
		{Konto: revenueAccount, Betrag: round2(SumNetto(lines)), Soll: false},
	}
	for _, l := range lines {
		if l.MwStBetrag == 0 {
			continue
		}
		if konto, ok := rules.UmsatzsteuerKonto(l.SatzProzent); ok {
			entries = append(entries, BookingEntry{Konto: konto, Betrag: round2(l.MwStBetrag), Soll: false})
		}
	}
	var habenSum float64
	for _, e := range entries {
		habenSum += e.Betrag
	}
	// Payment (Soll) = Σ Haben, prepended so it reads payment-first.
	entries = append([]BookingEntry{{Konto: paymentAccount, Betrag: round2(habenSum), Soll: true}}, entries...)
	return Booking{Entries: entries}, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run TestBuildRevenueBooking`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/core/buchung.go internal/core/buchung_test.go
git commit -m "E15.1: BuildRevenueBooking (mirror of BuildBooking)"
```

---

### Task 6: `Booking.PaymentAndCounters`

**Files:**
- Modify: `internal/core/buchung.go` (add method near `PaymentEntry`)
- Test: `internal/core/buchung_test.go`

**Interfaces:**
- Produces: `func (b Booking) PaymentAndCounters(isRevenue bool) (BookingEntry, []BookingEntry, bool)`.

- [ ] **Step 1: Write the failing test**:

```go
func TestPaymentAndCounters(t *testing.T) {
	// Incoming: 1 Haben (payment) + 2 Soll.
	in := Booking{Entries: []BookingEntry{
		{Konto: 4980, Betrag: 100, Soll: true},
		{Konto: 1576, Betrag: 19, Soll: true},
		{Konto: 1200, Betrag: 119, Soll: false},
	}}
	base, counters, ok := in.PaymentAndCounters(false)
	if !ok || base.Konto != 1200 || len(counters) != 2 {
		t.Fatalf("incoming: base=%v counters=%d ok=%v", base, len(counters), ok)
	}
	// Revenue: 1 Soll (payment) + 2 Haben.
	rev := Booking{Entries: []BookingEntry{
		{Konto: 1200, Betrag: 119, Soll: true},
		{Konto: 8400, Betrag: 100, Soll: false},
		{Konto: 1776, Betrag: 19, Soll: false},
	}}
	base, counters, ok = rev.PaymentAndCounters(true)
	if !ok || base.Konto != 1200 || len(counters) != 2 {
		t.Fatalf("revenue: base=%v counters=%d ok=%v", base, len(counters), ok)
	}
	// Ambiguous base side (2 Soll for revenue) → ok=false.
	bad := Booking{Entries: []BookingEntry{{Konto: 1, Soll: true}, {Konto: 2, Soll: true}}}
	if _, _, ok := bad.PaymentAndCounters(true); ok {
		t.Error("two Soll entries for revenue must yield ok=false")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run TestPaymentAndCounters`
Expected: FAIL — undefined.

- [ ] **Step 3: Implement** — add to `buchung.go`:

```go
// PaymentAndCounters splits a booking into its single payment/base entry and
// the counter entries that post against it. For an incoming invoice the base is
// the single Haben (Zahlungskonto); for a revenue invoice (isRevenue) it is the
// single Soll. ok is false unless the base side has exactly one entry and there
// is at least one counter — the exporters skip the booking in that case.
func (b Booking) PaymentAndCounters(isRevenue bool) (BookingEntry, []BookingEntry, bool) {
	var base BookingEntry
	baseCount := 0
	counters := make([]BookingEntry, 0, len(b.Entries))
	for _, e := range b.Entries {
		isBase := (isRevenue && e.Soll) || (!isRevenue && !e.Soll)
		if isBase {
			base = e
			baseCount++
		} else {
			counters = append(counters, e)
		}
	}
	if baseCount != 1 || len(counters) == 0 {
		return BookingEntry{}, nil, false
	}
	return base, counters, true
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run TestPaymentAndCounters`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/core/buchung.go internal/core/buchung_test.go
git commit -m "E15.1: Booking.PaymentAndCounters for both booking directions"
```

---

### Task 7: Generalize DATEV export

**Files:**
- Modify: `internal/core/datevexport.go` (`BuildDATEVStapel` loop ~52-67)
- Test: `internal/core/datevexport_test.go`

**Interfaces:**
- Consumes: `Booking.PaymentAndCounters` (Task 6), `CSVRow.Ausgangsrechnung` (Task 1).

- [ ] **Step 1: Write the failing test** — append to `datevexport_test.go`:

```go
func TestDATEVRevenueRow(t *testing.T) {
	rows := []CSVRow{{
		Rechnungsdatum: "10.12.2025", Belegnummer: "2025-0002", Auftraggeber: "Symeo",
		Ausgangsrechnung: true,
		Buchung: Booking{Entries: []BookingEntry{
			{Konto: 1200, Betrag: 7735, Soll: true},
			{Konto: 8400, Betrag: 6500, Soll: false},
			{Konto: 1776, Betrag: 1235, Soll: false},
		}},
	}}
	data, exported, skipped := BuildDATEVStapel(DATEVHeader{WJBeginn: "20250101"}, rows)
	if exported != 2 || skipped != 0 {
		t.Fatalf("exported=%d skipped=%d (want 2,0)", exported, skipped)
	}
	s := string(data)
	// Erlös line: 6500 credited (H) on 8400 against base 1200.
	if !strings.Contains(s, `6500,00;"H";"EUR";;;;8400;1200;;1012;"2025-0002"`) {
		t.Errorf("revenue Erlös row missing:\n%s", s)
	}
	// USt line: 1235 credited (H) on 1776 against 1200.
	if !strings.Contains(s, `1235,00;"H";"EUR";;;;1776;1200;;1012;"2025-0002"`) {
		t.Errorf("revenue USt row missing:\n%s", s)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run TestDATEVRevenueRow`
Expected: FAIL — current code books all rows as "S" against the single Haben; revenue row skipped (PaymentEntry finds 1 Haben? it would find 1 Haben=1200 and treat 8400/1776... actually current uses DebitEntries (the 1 Soll 1200) → wrong). Expect mismatch.

- [ ] **Step 3: Rework the loop** — replace the body of the `for _, r := range rows` loop in `BuildDATEVStapel` (the part from `pay, ok := ...` through the inner `for _, e := range r.Buchung.DebitEntries()`):

```go
		base, counters, ok := r.Buchung.PaymentAndCounters(r.Ausgangsrechnung)
		if !r.Buchung.Balanced() || !ok {
			skipped++
			continue
		}
		beleg := datevBeleg(r.Rechnungsdatum)
		belegfeld1 := r.Belegnummer
		if belegfeld1 == "" {
			belegfeld1 = r.Rechnungsnummer
		}
		belegfeld1 = datevClean(belegfeld1, 36)
		belegfeld2 := datevClean(r.Rechnungsnummer, 36)
		text := datevClean(strings.TrimSpace(r.Auftraggeber+" "+r.Verwendungszweck), 60)
		for _, e := range counters {
			sh := "S"
			if !e.Soll {
				sh = "H"
			}
			b.WriteString(fmt.Sprintf(`%s;"%s";"EUR";;;;%d;%d;;%s;"%s";"%s";;"%s"`+"\r\n",
				datevAmount(e.Betrag), sh, e.Konto, base.Konto, beleg, belegfeld1, belegfeld2, text))
			exported++
		}
```

- [ ] **Step 4: Run tests to verify both directions pass**

Run: `go test ./internal/core/ -run 'TestDATEV'`
Expected: PASS — `TestDATEVRevenueRow`, `TestBuildDATEVStapel` (incoming, still "S"), `TestDATEVBelegnummerFields` all green.

- [ ] **Step 5: Commit**

```bash
git add internal/core/datevexport.go internal/core/datevexport_test.go
git commit -m "E15.1: DATEV export handles revenue bookings (per-entry S/H)"
```

---

### Task 8: Generalize Lexware export

**Files:**
- Modify: `internal/core/lexwareexport.go` (`BuildLexwareCSV` loop ~14-27)
- Test: `internal/core/lexwareexport_test.go`

**Interfaces:**
- Consumes: `Booking.PaymentAndCounters`, `CSVRow.Ausgangsrechnung`.

- [ ] **Step 1: Write the failing test** — append:

```go
func TestLexwareRevenueRow(t *testing.T) {
	rows := []CSVRow{{
		Rechnungsdatum: "10.12.2025", Belegnummer: "2025-0002", Auftraggeber: "Symeo",
		Ausgangsrechnung: true,
		Buchung: Booking{Entries: []BookingEntry{
			{Konto: 1200, Betrag: 7735, Soll: true},
			{Konto: 8400, Betrag: 6500, Soll: false},
			{Konto: 1776, Betrag: 1235, Soll: false},
		}},
	}}
	data, exported, _ := BuildLexwareCSV(rows)
	s := string(data)
	if exported != 2 {
		t.Fatalf("exported=%d, want 2", exported)
	}
	// Erlös: Soll 1200 (bank), Haben 8400.
	if !strings.Contains(s, "10.12.2025;2025-0002;Symeo;6500,00;1200;8400") {
		t.Errorf("revenue Erlös line missing:\n%s", s)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run TestLexwareRevenueRow`
Expected: FAIL.

- [ ] **Step 3: Rework the loop** — replace the body of the `for _, r := range rows` loop in `BuildLexwareCSV`:

```go
		base, counters, ok := r.Buchung.PaymentAndCounters(r.Ausgangsrechnung)
		if !r.Buchung.Balanced() || !ok {
			skipped++
			continue
		}
		text := lexClean(strings.TrimSpace(r.Auftraggeber + " " + r.Verwendungszweck))
		belegRef := r.Belegnummer
		if belegRef == "" {
			belegRef = r.Rechnungsnummer
		}
		beleg := lexClean(belegRef)
		for _, e := range counters {
			soll, haben := e.Konto, base.Konto
			if !e.Soll {
				soll, haben = base.Konto, e.Konto
			}
			amount := strings.Replace(fmt.Sprintf("%.2f", e.Betrag), ".", ",", 1)
			b.WriteString(fmt.Sprintf("%s;%s;%s;%s;%d;%d\r\n",
				r.Rechnungsdatum, beleg, text, amount, soll, haben))
			exported++
		}
```

- [ ] **Step 4: Run tests to verify it passes**

Run: `go test ./internal/core/ -run 'TestLexware'`
Expected: PASS — revenue + existing incoming test green.

- [ ] **Step 5: Commit**

```bash
git add internal/core/lexwareexport.go internal/core/lexwareexport_test.go
git commit -m "E15.1: Lexware export handles revenue bookings"
```

---

### Task 9: UI — persist flag, revenue booking preview, Erlöskonto

**Files:**
- Modify: `internal/ui/bookinghelpers.go` (add `computeRevenueBooking`)
- Modify: `internal/ui/invoicemodal.go` (set `meta.Ausgangsrechnung`; branch booking computation; label; recompute on toggle)
- Modify: `internal/ui/tableedit.go` (same branch + preserve flag)
- Test: `internal/ui/` build + manual (Fyne UI not unit-tested here)

**Interfaces:**
- Consumes: `core.BuildRevenueBooking`, `Settings.PaymentAccountSKR04`, `core.Meta.Ausgangsrechnung`.
- Produces: `func (a *App) computeRevenueBooking(lines []core.TaxLine, revenueAccount int, bankAccountName string) (core.Booking, bool, string)`.

- [ ] **Step 1: Add `computeRevenueBooking`** in `bookinghelpers.go` (mirror `computeInvoiceBooking`):

```go
// computeRevenueBooking builds the revenue booking for an outgoing invoice:
// Soll Zahlungskonto, Haben Erlöskonto + Umsatzsteuer. Returns (booking, ok, msg).
func (a *App) computeRevenueBooking(lines []core.TaxLine, revenueAccount int, bankAccountName string) (core.Booking, bool, string) {
	if len(lines) == 0 {
		return core.Booking{}, false, a.bundle.T("booking.no.lines")
	}
	payment, ok := a.settings.PaymentAccountSKR04(bankAccountName)
	if !ok {
		return core.Booking{}, false, a.bundle.T("booking.no.payment.account")
	}
	b, err := core.BuildRevenueBooking(a.bookingRules, lines, revenueAccount, payment)
	if err != nil {
		return core.Booking{}, false, err.Error()
	}
	return b, true, ""
}
```

- [ ] **Step 2: Build to verify it compiles**

Run: `go build ./internal/ui/`
Expected: OK (function unused yet is fine — it's a method; Go allows unused methods).

- [ ] **Step 3: Wire the modal** in `invoicemodal.go`:
  - In the booking-preview recompute closure and in the save handler's booking block, branch on `ausgangsrechnungCheck.Checked`: when true call `a.computeRevenueBooking(ed.Lines(), selectedAccount, bankAccountSelect.Selected)`; else the existing `a.computeInvoiceBooking(...)`. (The category dropdown is irrelevant for revenue.)
  - Add `ausgangsrechnungCheck.OnChanged = func(bool) { recomputeBooking() }` (use the existing recompute function name in scope) and also trigger the filename preview if needed.
  - In `saveInvoice`, set `meta.Ausgangsrechnung = ausgangsrechnung` (the bool param already passed in) right after the `meta` struct is built.
  - Optional polish: when `ausgangsrechnungCheck.Checked`, set the account-picker button label to `a.bundle.T("modal.erloeskonto")` ("Erlöskonto") instead of "Gegenkonto"; falls back to existing label otherwise.

- [ ] **Step 4: Preserve flag on edit** in `tableedit.go`:
  - In the `newMeta := core.Meta{...}` literal add `Ausgangsrechnung: originalRow.Ausgangsrechnung,` (or bind to an edit checkbox if the edit dialog exposes one; if not, preserve the original).
  - Branch the booking computation the same way as the modal when the row is an Ausgangsrechnung.

- [ ] **Step 5: Add i18n + build + full test**

Add to `assets/i18n/de.json`: `"modal.erloeskonto": "Erlöskonto",`. To `en.json`: `"modal.erloeskonto": "Revenue account",`.

Run: `go build ./... && go test ./...`
Expected: build OK, all tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/ui/bookinghelpers.go internal/ui/invoicemodal.go internal/ui/tableedit.go assets/i18n/de.json assets/i18n/en.json
git commit -m "E15.1: UI books outgoing invoices as revenue (Erlöskonto + USt)"
```

---

### Task 10: Rollout data step + verification (no repo code)

**Files:** none in repo. Profile data + manual verification.

- [ ] **Step 1:** Add `umsatzsteuer_konten` to the Bergx2 profile rules file `%APPDATA%/BuchISY/profiles/Bergx2 GmbH/buchungsregeln.json` (else it inherits the bundled SKR04 3806/3801): add `"umsatzsteuer_konten": { "19": 1776 }`. **Do this only with the app closed** (BuchISY rewrites profile files on save).
- [ ] **Step 2:** Rebuild the exe: `go build -ldflags "-H=windowsgui" -o dist/BuchISY.exe ./cmd/buchisy`.
- [ ] **Step 3:** Manual smoke test: mark an invoice as Ausgangsrechnung, confirm the preview shows Soll Zahlungskonto / Haben Erlöskonto + USt, save, export DATEV + Lexware, confirm the revenue rows.

---

## Self-Review

**Spec coverage:** §1 Datenmodell → Task 1+2+3. §2 Config → Task 4. §3 BuildRevenueBooking → Task 5. §4 Export-Verallgemeinerung → Task 6+7+8. §5 UI → Task 9. §6 Tests → folded into each task. Rollout data step (§Config note) → Task 10. All covered.

**Placeholder scan:** No TBD/TODO; every code step shows complete code. Task 9 (Fyne UI) describes wiring precisely against named widgets from this session's reading (`ausgangsrechnungCheck`, `bankAccountSelect`, `selectedAccount`, `ed.Lines()`); the implementer confirms the exact recompute-closure name in `invoicemodal.go`.

**Type consistency:** `Ausgangsrechnung bool` used identically across Meta/CSVRow/DB/CSV. `UmsatzsteuerKonto(float64)(int,bool)` mirrors `VorsteuerKonto`. `BuildRevenueBooking(*BookingRules,[]TaxLine,int,int)(Booking,error)` and `PaymentAndCounters(bool)(BookingEntry,[]BookingEntry,bool)` consistent between definition (Tasks 5/6) and consumers (Tasks 7/8/9).
