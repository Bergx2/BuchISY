# Buchungswissen pro Beleg (Phase C) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Provide the pure-core booking knowledge for BuchISY: a booking data model, a bundled rules base, a deterministic booking builder (standard expense + Bewirtung 70/30), per-receipt booking storage, and per-company booking templates — the foundation Phase D's UI/engine/export will consume.

**Architecture:** New `internal/core` types `BookingEntry` (one Soll/Haben line) and `Booking` (a receipt's balanced set of entries + free-text info). A bundled `assets/buchungsregeln.json` (rules + Vorsteuer accounts) drives `BuildBooking`, which turns a receipt's `TaxLine`s into balanced entries per category. The booking persists alongside the invoice (CSV + SQLite, like Phase A's TaxLines), and a per-profile `BookingTemplateStore` remembers company→category/account so Phase D can pre-fill known vendors deterministically.

**Tech Stack:** Go 1.25, encoding/json, go:embed. No UI, no Claude, no export in this phase.

## Global Constraints

- Pure `internal/core` logic only — no UI, no Anthropic, no export here (those are Phase D).
- NEVER invent account numbers or tax rules. The rules base uses accounts confirmed by the source receipt and the user's chart: Bewirtung 70 % abziehbar = **6640**, 30 % nicht abziehbar = **6644**; Vorsteuer 19 % = **1406**, 7 % = **1401**. Bewirtung is 70 % deductible (operating-expense split only), Vorsteuer is 100 % deductible.
- Money is `float64`, rounded to 2 decimals; a `Booking` must balance: Σ Soll == Σ Haben (within 0.005).
- Do NOT collide with the existing `core.StatementBooking` (bank-statement line) — new types are `BookingEntry` / `Booking`.
- `Steuerschluessel` (DATEV BU key) stays optional/empty in this phase (assigned at export time in Phase D).
- Backward compatibility: a receipt without a stored booking loads with an empty `Booking` (no crash, no fabricated entries).
- Tests next to code; `go test ./...`. Commit per task.

---

### Task 1: BookingEntry + Booking types

**Files:**
- Create: `internal/core/buchung.go`
- Test: `internal/core/buchung_test.go`

**Interfaces:**
- Produces: `type BookingEntry struct { Konto int; Betrag float64; Soll bool; Steuerschluessel string }`;
  `type Booking struct { Entries []BookingEntry; Info string }`;
  `(b Booking) SollSum() float64`; `(b Booking) HabenSum() float64`;
  `(b Booking) Balanced() bool` (|SollSum-HabenSum| < 0.005 AND len(Entries) > 0);
  `(b Booking) IsEmpty() bool` (len(Entries) == 0 && Info == "").

- [ ] **Step 1: Write the failing test**

```go
package core

import "testing"

func TestBookingBalance(t *testing.T) {
	b := Booking{Entries: []BookingEntry{
		{Konto: 6640, Betrag: 12.71, Soll: true},
		{Konto: 6644, Betrag: 5.44, Soll: true},
		{Konto: 1406, Betrag: 1.26, Soll: true},
		{Konto: 1401, Betrag: 0.59, Soll: true},
		{Konto: 1800, Betrag: 20.00, Soll: false},
	}}
	if !almost(b.SollSum(), 20.00) || !almost(b.HabenSum(), 20.00) {
		t.Fatalf("sums: soll=%v haben=%v", b.SollSum(), b.HabenSum())
	}
	if !b.Balanced() {
		t.Error("should be balanced")
	}
	if (Booking{}).Balanced() {
		t.Error("empty booking is not balanced")
	}
	if !(Booking{}).IsEmpty() {
		t.Error("zero booking should be empty")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run TestBookingBalance`
Expected: FAIL (undefined Booking/BookingEntry). Note: `almost` already exists in `internal/core` (`taxline_test.go`).

- [ ] **Step 3: Write minimal implementation**

```go
package core

import "math"

// BookingEntry is one line of a double-entry booking: an amount posted to an
// account on the debit (Soll=true) or credit (Soll=false) side.
type BookingEntry struct {
	Konto            int     `json:"konto"`
	Betrag           float64 `json:"betrag"`
	Soll             bool    `json:"soll"` // true = Soll (debit), false = Haben (credit)
	Steuerschluessel string  `json:"steuerschluessel,omitempty"`
}

// Booking is the set of entries that posts a single receipt, plus a free-text
// rationale/notes ("Buchungswissen").
type Booking struct {
	Entries []BookingEntry `json:"entries,omitempty"`
	Info    string         `json:"info,omitempty"`
}

// SollSum returns the total of the debit entries.
func (b Booking) SollSum() float64 {
	var s float64
	for _, e := range b.Entries {
		if e.Soll {
			s += e.Betrag
		}
	}
	return s
}

// HabenSum returns the total of the credit entries.
func (b Booking) HabenSum() float64 {
	var s float64
	for _, e := range b.Entries {
		if !e.Soll {
			s += e.Betrag
		}
	}
	return s
}

// Balanced reports whether debits equal credits (within rounding) and there is
// at least one entry.
func (b Booking) Balanced() bool {
	return len(b.Entries) > 0 && math.Abs(b.SollSum()-b.HabenSum()) < 0.005
}

// IsEmpty reports whether the booking carries no entries and no info.
func (b Booking) IsEmpty() bool {
	return len(b.Entries) == 0 && b.Info == ""
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run TestBookingBalance && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/core/buchung.go internal/core/buchung_test.go
git commit -m "Add Booking and BookingEntry types with balance check"
```

---

### Task 2: Rules base — bundled JSON + loader

**Files:**
- Create: `assets/buchungsregeln.json`
- Modify: `assets/embed.go`
- Create: `internal/core/buchungsregeln.go`
- Test: `internal/core/buchungsregeln_test.go`

**Interfaces:**
- Produces: `type BookingRule struct { Kategorie, Name string; AbziehbarProzent float64; KontoAbziehbar, KontoNichtAbziehbar int }`;
  `type BookingRules struct { VorsteuerKonten map[string]int; Regeln []BookingRule }`;
  `ParseBookingRules(data []byte) (*BookingRules, error)`;
  `(r *BookingRules) Rule(kategorie string) (BookingRule, bool)`;
  `(r *BookingRules) VorsteuerKonto(satzProzent float64) (int, bool)` (looks up by integer percent, e.g. 19.0 → key "19");
  `assets.BuchungsregelnJSON []byte`.

- [ ] **Step 1: Create the bundled rules**

Create `assets/buchungsregeln.json`:

```json
{
  "vorsteuer_konten": { "19": 1406, "7": 1401 },
  "regeln": [
    { "kategorie": "standard", "name": "Standard-Aufwand" },
    { "kategorie": "bewirtung", "name": "Bewirtung (§ 4 Abs. 5 EStG)", "abziehbar_prozent": 70, "konto_abziehbar": 6640, "konto_nicht_abziehbar": 6644 }
  ]
}
```

In `assets/embed.go`, add after the SKR04 embed:

```go
// BuchungsregelnJSON is the bundled booking rules base (Vorsteuer accounts +
// category rules like the Bewirtung 70/30 split).
//
//go:embed buchungsregeln.json
var BuchungsregelnJSON []byte
```

- [ ] **Step 2: Write the failing test**

Create `internal/core/buchungsregeln_test.go`:

```go
package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseBookingRules(t *testing.T) {
	data, _ := os.ReadFile(filepath.Join("..", "..", "assets", "buchungsregeln.json"))
	r, err := ParseBookingRules(data)
	if err != nil {
		t.Fatal(err)
	}
	bew, ok := r.Rule("bewirtung")
	if !ok || bew.AbziehbarProzent != 70 || bew.KontoAbziehbar != 6640 || bew.KontoNichtAbziehbar != 6644 {
		t.Fatalf("bewirtung rule = %+v", bew)
	}
	if _, ok := r.Rule("standard"); !ok {
		t.Error("standard rule missing")
	}
	if k, ok := r.VorsteuerKonto(19); !ok || k != 1406 {
		t.Errorf("VorsteuerKonto(19) = %d,%v", k, ok)
	}
	if k, ok := r.VorsteuerKonto(7); !ok || k != 1401 {
		t.Errorf("VorsteuerKonto(7) = %d,%v", k, ok)
	}
	if _, ok := r.VorsteuerKonto(0); ok {
		t.Error("VorsteuerKonto(0) should be false")
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/core/ -run TestParseBookingRules`
Expected: FAIL (undefined ParseBookingRules).

- [ ] **Step 4: Write minimal implementation**

Create `internal/core/buchungsregeln.go`:

```go
package core

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// BookingRule describes how a booking category posts a receipt's net amounts.
// AbziehbarProzent / the two Konto fields are only used by categories that
// split (e.g. Bewirtung); they are zero for the plain "standard" rule.
type BookingRule struct {
	Kategorie           string  `json:"kategorie"`
	Name                string  `json:"name"`
	AbziehbarProzent    float64 `json:"abziehbar_prozent,omitempty"`
	KontoAbziehbar      int     `json:"konto_abziehbar,omitempty"`
	KontoNichtAbziehbar int     `json:"konto_nicht_abziehbar,omitempty"`
}

// BookingRules is the bundled rules base: Vorsteuer accounts keyed by integer
// percent ("19","7") and the list of category rules.
type BookingRules struct {
	VorsteuerKonten map[string]int `json:"vorsteuer_konten"`
	Regeln          []BookingRule  `json:"regeln"`
}

// ParseBookingRules decodes the rules base JSON.
func ParseBookingRules(data []byte) (*BookingRules, error) {
	var r BookingRules
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("failed to parse booking rules: %w", err)
	}
	return &r, nil
}

// Rule returns the rule for a category (case-sensitive key match).
func (r *BookingRules) Rule(kategorie string) (BookingRule, bool) {
	for _, rule := range r.Regeln {
		if rule.Kategorie == kategorie {
			return rule, true
		}
	}
	return BookingRule{}, false
}

// VorsteuerKonto returns the Vorsteuer account for a VAT rate (percent). The
// rate is matched as an integer key, so 19.0 → "19".
func (r *BookingRules) VorsteuerKonto(satzProzent float64) (int, bool) {
	k, ok := r.VorsteuerKonten[strconv.Itoa(int(satzProzent+0.5))]
	return k, ok
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/core/ -run TestParseBookingRules && go build ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add assets/buchungsregeln.json assets/embed.go internal/core/buchungsregeln.go internal/core/buchungsregeln_test.go
git commit -m "Add bundled booking rules base and loader"
```

---

### Task 3: BuildBooking — deterministic booking builder

**Files:**
- Modify: `internal/core/buchung.go`
- Test: `internal/core/buchung_test.go`

**Interfaces:**
- Consumes: `TaxLine`, `BookingRule`, `BookingRules.VorsteuerKonto`, `round2`.
- Produces: `BuildBooking(rules *BookingRules, kategorie string, lines []TaxLine, trinkgeld float64, expenseAccount, paymentAccount int) (Booking, error)`.

Behavior — for every category the booking always:
- posts each line's VAT to its Vorsteuer account (Soll), looked up by rate; lines whose rate has no Vorsteuer account post no VAT entry,
- credits the payment account (Haben) with the gross = Σ(net+vat) + trinkgeld.
Category "standard": posts (Σ net + trinkgeld) to expenseAccount (Soll).
Category "bewirtung": splits (Σ net + trinkgeld) into AbziehbarProzent % on KontoAbziehbar (Soll) and the remainder on KontoNichtAbziehbar (Soll) — the remainder is computed by subtraction so rounding never unbalances the booking.
Returns an error if kategorie is unknown.

- [ ] **Step 1: Write the failing test**

```go
func TestBuildBookingBewirtung(t *testing.T) {
	rules, _ := ParseBookingRules([]byte(`{"vorsteuer_konten":{"19":1406,"7":1401},"regeln":[{"kategorie":"standard","name":"Standard"},{"kategorie":"bewirtung","name":"Bewirtung","abziehbar_prozent":70,"konto_abziehbar":6640,"konto_nicht_abziehbar":6644}]}`))
	lines := []TaxLine{
		{Netto: 6.64, SatzProzent: 19, MwStBetrag: 1.26},
		{Netto: 8.41, SatzProzent: 7, MwStBetrag: 0.59},
	}
	b, err := BuildBooking(rules, "bewirtung", lines, 3.10, 0, 1800)
	if err != nil {
		t.Fatal(err)
	}
	if !b.Balanced() || !almost(b.HabenSum(), 20.00) {
		t.Fatalf("not balanced / haben != 20: %+v (haben=%v)", b, b.HabenSum())
	}
	got := map[int]float64{}
	for _, e := range b.Entries {
		if e.Soll {
			got[e.Konto] += e.Betrag
		}
	}
	// net+trinkgeld = 18.15; 70% = 12.71 (6640), remainder 5.44 (6644); VSt 1.26/0.59.
	if !almost(got[6640], 12.71) || !almost(got[6644], 5.44) {
		t.Errorf("split wrong: 6640=%v 6644=%v", got[6640], got[6644])
	}
	if !almost(got[1406], 1.26) || !almost(got[1401], 0.59) {
		t.Errorf("vorsteuer wrong: 1406=%v 1401=%v", got[1406], got[1401])
	}
}

func TestBuildBookingStandard(t *testing.T) {
	rules, _ := ParseBookingRules([]byte(`{"vorsteuer_konten":{"19":1406},"regeln":[{"kategorie":"standard","name":"Standard"}]}`))
	lines := []TaxLine{{Netto: 100, SatzProzent: 19, MwStBetrag: 19}}
	b, err := BuildBooking(rules, "standard", lines, 0, 6815, 1800)
	if err != nil {
		t.Fatal(err)
	}
	got := map[int]float64{}
	for _, e := range b.Entries {
		if e.Soll {
			got[e.Konto] += e.Betrag
		}
	}
	if !almost(got[6815], 100) || !almost(got[1406], 19) || !almost(b.HabenSum(), 119) {
		t.Errorf("standard booking wrong: %+v", b)
	}
	if _, err := BuildBooking(rules, "unbekannt", lines, 0, 6815, 1800); err == nil {
		t.Error("unknown category should error")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run TestBuildBooking`
Expected: FAIL (undefined BuildBooking).

- [ ] **Step 3: Write minimal implementation**

Add to `internal/core/buchung.go`:

```go
// BuildBooking turns a receipt's tax lines into a balanced Booking per the
// category rule. expenseAccount is the Soll account for the "standard" case;
// paymentAccount is the Haben account (Zahlungskonto). Returns an error for an
// unknown category.
func BuildBooking(rules *BookingRules, kategorie string, lines []TaxLine, trinkgeld float64, expenseAccount, paymentAccount int) (Booking, error) {
	rule, ok := rules.Rule(kategorie)
	if !ok {
		return Booking{}, fmt.Errorf("unbekannte Buchungskategorie: %s", kategorie)
	}

	netTotal := round2(SumNetto(lines) + trinkgeld)
	var entries []BookingEntry

	switch kategorie {
	case "bewirtung":
		abz := round2(netTotal * rule.AbziehbarProzent / 100)
		nicht := round2(netTotal - abz)
		entries = append(entries,
			BookingEntry{Konto: rule.KontoAbziehbar, Betrag: abz, Soll: true},
			BookingEntry{Konto: rule.KontoNichtAbziehbar, Betrag: nicht, Soll: true},
		)
	default: // "standard"
		entries = append(entries, BookingEntry{Konto: expenseAccount, Betrag: netTotal, Soll: true})
	}

	// Vorsteuer per rate (Soll).
	for _, l := range lines {
		if l.MwStBetrag == 0 {
			continue
		}
		if konto, ok := rules.VorsteuerKonto(l.SatzProzent); ok {
			entries = append(entries, BookingEntry{Konto: konto, Betrag: round2(l.MwStBetrag), Soll: true})
		}
	}

	// Payment (Haben) = gross.
	gross := round2(SumNetto(lines) + SumMwSt(lines) + trinkgeld)
	entries = append(entries, BookingEntry{Konto: paymentAccount, Betrag: gross, Soll: false})

	return Booking{Entries: entries}, nil
}
```

`fmt` is already imported by `buchung.go` (Task 1 added `math`; add `"fmt"` to the import block here).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run TestBuildBooking && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/core/buchung.go internal/core/buchung_test.go
git commit -m "Add BuildBooking: standard + Bewirtung 70/30 with Vorsteuer split"
```

---

### Task 4: Persist the booking with the invoice (CSV + SQLite)

**Files:**
- Modify: `internal/core/types.go` (Meta + CSVRow + conversions)
- Modify: `internal/core/csvrepo.go` (column + load/save)
- Modify: `internal/db/schema.go` + `internal/db/repository.go` (column + migration + insert/update/scan)
- Test: `internal/core/csvrepo_test.go`, `internal/db/repository_test.go`

**Interfaces:**
- Consumes: `Booking`, JSON (de)serialize.
- Produces: `Meta.Buchung Booking` and `CSVRow.Buchung Booking`, round-tripped through CSV (`Buchung` JSON column) and SQLite (`buchung TEXT`).

- [ ] **Step 1: Write the failing test (CSV + DB round-trip)**

Add to `internal/core/csvrepo_test.go`:

```go
func TestCSVBookingRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invoices.csv")
	repo := NewCSVRepository()
	row := CSVRow{Dateiname: "a.pdf", Jahr: "2026", Monat: "06",
		Buchung: Booking{Entries: []BookingEntry{{Konto: 6640, Betrag: 12.71, Soll: true}, {Konto: 1800, Betrag: 12.71, Soll: false}}, Info: "Bewirtung"}}
	if err := repo.Append(path, row); err != nil {
		t.Fatal(err)
	}
	rows, _ := repo.Load(path)
	if len(rows) != 1 || len(rows[0].Buchung.Entries) != 2 || rows[0].Buchung.Info != "Bewirtung" {
		t.Fatalf("booking not round-tripped: %+v", rows)
	}
}
```

Add to `internal/db/repository_test.go`:

```go
func TestDBBookingRoundTrip(t *testing.T) {
	repo := newTestRepo(t)
	if _, err := repo.Insert(core.CSVRow{Dateiname: "a.pdf", Jahr: "2026", Monat: "06",
		Buchung: core.Booking{Entries: []core.BookingEntry{{Konto: 6640, Betrag: 12.71, Soll: true}}, Info: "x"}}); err != nil {
		t.Fatal(err)
	}
	rows, _ := repo.List("2026", "06")
	if len(rows) != 1 || len(rows[0].Buchung.Entries) != 1 || rows[0].Buchung.Info != "x" {
		t.Fatalf("DB booking round-trip failed: %+v", rows)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/core/ -run TestCSVBookingRoundTrip; go test ./internal/db/ -run TestDBBookingRoundTrip`
Expected: FAIL (unknown field Buchung).

- [ ] **Step 3: Implement**

In `internal/core/types.go`:
- Add `Buchung Booking` to `Meta` (after `BuchungRef`) and to `CSVRow` (after `BuchungRef`).
- In `ToCSVRow()` add `Buchung: m.Buchung,`; in `ToMeta()` add `Buchung: r.Buchung,`.

In `internal/core/buchung.go` add JSON helpers:

```go
// MarshalBooking encodes a booking as compact JSON ("" when empty).
func MarshalBooking(b Booking) string {
	if b.IsEmpty() {
		return ""
	}
	data, err := json.Marshal(b)
	if err != nil {
		return ""
	}
	return string(data)
}

// ParseBooking decodes a booking from JSON ("" / invalid → empty Booking).
func ParseBooking(s string) Booking {
	s = strings.TrimSpace(s)
	if s == "" {
		return Booking{}
	}
	var b Booking
	if err := json.Unmarshal([]byte(s), &b); err != nil {
		return Booking{}
	}
	return b
}
```

(Add `"encoding/json"` and `"strings"` to `buchung.go` imports.)

In `internal/core/csvrepo.go`: add `"Buchung"` to `DefaultCSVColumns` (after `"BuchungRef"`), to `ColumnDisplayNames` (`"Buchung": "Buchungssatz"`), `ColumnTranslationKeys` (`"Buchung": "table.col.buchung"`); in `Load` after the row literal: `row.Buchung = ParseBooking(valueForColumn(record, headerMap, "Buchung"))`; in `rowToRecord` valueMap: `"Buchung": MarshalBooking(row.Buchung),`. Add i18n keys `table.col.buchung` to de.json ("Buchungssatz") and en.json ("Booking").

In `internal/db/schema.go`: add `buchung TEXT,` after `steuerzeilen TEXT,`. In `internal/db/repository.go` `initSchema` ALTER loop add `"ALTER TABLE invoices ADD COLUMN buchung TEXT DEFAULT ''"`. In `Insert`/`Update`/`List` add the `buchung` column + placeholder + arg (`core.MarshalBooking(row.Buchung)` on write; on read scan into `sql.NullString` then `row.Buchung = core.ParseBooking(ns.String)`), keeping column/placeholder/arg counts aligned.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/core/ ./internal/db/ && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/core/types.go internal/core/buchung.go internal/core/csvrepo.go internal/core/csvrepo_test.go internal/db/schema.go internal/db/repository.go internal/db/repository_test.go assets/i18n/de.json assets/i18n/en.json
git commit -m "Persist the per-receipt booking in CSV and SQLite"
```

---

### Task 5: Per-company booking template store

**Files:**
- Create: `internal/core/bookingtemplate.go`
- Test: `internal/core/bookingtemplate_test.go`

**Interfaces:**
- Produces: `type BookingTemplate struct { Kategorie string; ExpenseKonto int }`;
  `type BookingTemplateStore struct {…}`; `NewBookingTemplateStore(configDir string) *BookingTemplateStore`;
  `(s *BookingTemplateStore) Load() error`; `(s *BookingTemplateStore) Get(company string) (BookingTemplate, bool)`;
  `(s *BookingTemplateStore) Set(company string, t BookingTemplate) error` (persists to `<configDir>/booking_templates.json`).

- [ ] **Step 1: Write the failing test**

```go
package core

import (
	"path/filepath"
	"testing"
)

func TestBookingTemplateStore(t *testing.T) {
	dir := t.TempDir()
	s := NewBookingTemplateStore(dir)
	if err := s.Load(); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Get("Matcha Rina"); ok {
		t.Error("expected no template yet")
	}
	if err := s.Set("Matcha Rina", BookingTemplate{Kategorie: "bewirtung", ExpenseKonto: 6640}); err != nil {
		t.Fatal(err)
	}
	// A fresh store over the same dir must read the persisted template.
	s2 := NewBookingTemplateStore(dir)
	if err := s2.Load(); err != nil {
		t.Fatal(err)
	}
	got, ok := s2.Get("Matcha Rina")
	if !ok || got.Kategorie != "bewirtung" || got.ExpenseKonto != 6640 {
		t.Fatalf("template not persisted: %+v %v", got, ok)
	}
	_ = filepath.Join(dir, "booking_templates.json")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run TestBookingTemplateStore`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

```go
package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// BookingTemplate is the remembered booking pattern for a company: which
// category to use and (for "standard") which expense account.
type BookingTemplate struct {
	Kategorie    string `json:"kategorie"`
	ExpenseKonto int    `json:"expense_konto"`
}

// BookingTemplateStore persists company→BookingTemplate per profile.
type BookingTemplateStore struct {
	path      string
	templates map[string]BookingTemplate
}

// NewBookingTemplateStore creates a store rooted at configDir.
func NewBookingTemplateStore(configDir string) *BookingTemplateStore {
	return &BookingTemplateStore{
		path:      filepath.Join(configDir, "booking_templates.json"),
		templates: map[string]BookingTemplate{},
	}
}

// Load reads the persisted templates (a missing file is not an error).
func (s *BookingTemplateStore) Load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return nil // no file yet
	}
	m := map[string]BookingTemplate{}
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("failed to parse booking templates: %w", err)
	}
	s.templates = m
	return nil
}

// Get returns the template remembered for company.
func (s *BookingTemplateStore) Get(company string) (BookingTemplate, bool) {
	t, ok := s.templates[company]
	return t, ok
}

// Set remembers and persists a template for company.
func (s *BookingTemplateStore) Set(company string, t BookingTemplate) error {
	s.templates[company] = t
	data, err := json.MarshalIndent(s.templates, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(s.path, data, 0644); err != nil {
		return fmt.Errorf("failed to save booking templates: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run TestBookingTemplateStore && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/core/bookingtemplate.go internal/core/bookingtemplate_test.go
git commit -m "Add per-company booking template store"
```

---

## Self-Review

- **Spec coverage (Phase C / Baustein C):** per-receipt booking knowledge stored with the invoice (T4: `Booking` + `Info` in CSV/DB); reusable company→booking templates (T5); bundled rules base for the common cases incl. Bewirtung 70/30 + Vorsteuer (T2); the deterministic booking logic that turns a receipt into balanced entries (T1, T3). Covered. (Capturing printed booking hints from the receipt text and the Claude/UI engine are Phase D, where the extractor/booking dialog live.)
- **Placeholder scan:** Task 3's prose explicitly resolves the `int` vs `float64` parameter ambiguity to `int`; all code blocks are complete. No TBD/TODO.
- **Type consistency:** `BookingEntry{Konto,Betrag,Soll,Steuerschluessel}`, `Booking{Entries,Info}` with `SollSum/HabenSum/Balanced/IsEmpty`, `BookingRule`/`BookingRules` with `Rule`/`VorsteuerKonto`, `BuildBooking(rules, kategorie, lines, trinkgeld, expenseAccount int, paymentAccount int)`, `MarshalBooking/ParseBooking`, `BookingTemplate{Kategorie,ExpenseKonto}`, `BookingTemplateStore.Load/Get/Set` — consistent across tasks.
- **Account/data integrity:** booking rules use only confirmed accounts (6640/6644/1406/1401); the Bewirtung remainder is computed by subtraction so the booking always balances; Steuerschluessel deferred to Phase D.
- **Out of scope (Phase D):** booking proposal UI + confirm dialog, capturing the receipt's printed booking hints, Claude category suggestion, DATEV/Lexware export, controlling.
