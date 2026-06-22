# Belegabgleich / Reconciliation (Phase E10) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Match each receipt (Beleg) to its transaction line on the bank/credit-card statement, auto-link unambiguous exact matches (green ✓), suggest the rest for one-click confirmation, and show the link status in the invoice table.

**Architecture:** Reuses the existing (dormant) link model: `CSVRow.BuchungRef = "<statementFile>|<page>|<lineIdx>"` and `StatementBooking` (numbered statement lines). A pure matcher scores statement lines for an invoice by amount (the strongest signal — for foreign currency the EUR debit incl. fee) + date proximity + supplier-name word overlap, and classifies the outcome as Auto / Vorschlag / Kein. A "Belegabgleich" dialog runs it for a period: auto-links unambiguous exact matches (sets `BuchungRef`), shows suggestions to confirm, and the invoice table gains a 🟢/⚪ status indicator.

**Tech Stack:** Go 1.25, Fyne v2. Reuses `BuchungRef`, `StatementBooking`/`ParseStatementBookings`, `ConvertForeignPayment` (E7), `collectInvoiceRows`, `dbRepo.Update`.

## Global Constraints

- Reuse the existing link: linking an invoice = set `CSVRow.BuchungRef` (already persisted in CSV+DB) and `Update` the row. The reverse (which invoice a statement line belongs to) is COMPUTED from invoices, not separately persisted.
- Matching applies only to **bank** and **creditcard** payment accounts. **Cash** (`AccountTypeCash`) invoices are not statement-matched (they live in the Kassenbuch) — show them as N/A, never as "unmatched".
- The **amount** is the hard signal: a candidate must match the invoice's effective EUR amount within 0.01 (EUR invoices → `Bruttobetrag`; foreign → `round2(Bruttobetrag/Wechselkurs) + Gebuehr`). Date proximity + name overlap only rank among amount-matches.
- **Auto (green):** exactly ONE amount-matching line within ±5 days → link automatically. **Vorschlag (amber):** amount-matches exist but are ambiguous (>1) or date-far → present for confirmation. **Kein (grey):** no amount-match.
- Auto-linking only ever SETS a `BuchungRef`; it never overwrites an already-set one (already-linked invoices are skipped).
- All user-facing strings via `a.bundle.T(...)` in both JSONs (valid JSON). `go build ./... && go test ./... && go vet ./internal/ui/` clean (ignore the pre-existing `clipboard_windows.go:206` warning). Commit per task.

---

### Task 1: Parse the amount on each statement line

**Files:**
- Modify: `internal/core/booking.go` (`StatementBooking.Betrag`)
- Modify: `internal/core/statement_bookings.go` (`ParseStatementBookings` fills it)
- Test: `internal/core/statement_bookings_test.go`

**Interfaces:**
- Produces: `StatementBooking.Betrag float64` (json `betrag,omitempty`) = the absolute value of the last money token in the line text; `ParseLineAmount(text string) float64` (the parser, exported for testing).

- [ ] **Step 1: Write the failing test**

```go
func TestParseLineAmount(t *testing.T) {
	cases := []struct {
		text string
		want float64
	}{
		{"14.01.2026 AMAZON WEB SERVICES EMEA 78,53", 78.53},
		{"03.01. Lastschrift Telekom -1.234,56", 1234.56},
		{"05.01. Gutschrift Kunde 2.000,00 H", 2000.00},
		{"no amount here", 0},
	}
	for _, c := range cases {
		if got := ParseLineAmount(c.text); got != c.want {
			t.Errorf("ParseLineAmount(%q) = %v, want %v", c.text, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run TestParseLineAmount`
Expected: FAIL (undefined ParseLineAmount).

- [ ] **Step 3: Implement**

In `internal/core/booking.go`, add to `StatementBooking` (after `Text`):

```go
	Betrag float64 `json:"betrag,omitempty"` // parsed absolute amount of the line
```

In `internal/core/statement_bookings.go`, add the parser and call it when building each booking (set `b.Betrag = ParseLineAmount(b.Text)` where the `StatementBooking` is assembled):

```go
// lineAmountRe matches German money amounts like "1.234,56" or "78,53".
var lineAmountRe = regexp.MustCompile(`\d{1,3}(?:\.\d{3})*,\d{2}`)

// ParseLineAmount returns the absolute value of the LAST money token in a
// statement line's text (the transaction amount sits at the end of the line),
// or 0 when none is present.
func ParseLineAmount(text string) float64 {
	matches := lineAmountRe.FindAllString(text, -1)
	if len(matches) == 0 {
		return 0
	}
	last := matches[len(matches)-1]
	last = strings.ReplaceAll(last, ".", "")
	last = strings.ReplaceAll(last, ",", ".")
	v, err := strconv.ParseFloat(last, 64)
	if err != nil {
		return 0
	}
	return v
}
```

(`regexp`, `strings`, `strconv` are already imported in statement_bookings.go.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run TestParseLineAmount && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/core/booking.go internal/core/statement_bookings.go internal/core/statement_bookings_test.go
git commit -m "Parse the transaction amount on each statement line"
```

---

### Task 2: Matching engine (core)

**Files:**
- Create: `internal/core/belegabgleich.go`
- Test: `internal/core/belegabgleich_test.go`

**Interfaces:**
- Consumes: `CSVRow`, `StatementBooking`, `round2`.
- Produces: `InvoiceEURAmount(row CSVRow) float64` (EUR → Bruttobetrag; foreign with Wechselkurs>0 → round2(Bruttobetrag/Wechselkurs)+Gebuehr);
  `type MatchKind int` with `MatchNone, MatchSuggest, MatchAuto`;
  `type ScoredLine struct { Line StatementBooking; Score float64 }`;
  `MatchInvoiceToStatement(row CSVRow, lines []StatementBooking) (MatchKind, []ScoredLine)` — amount-matching lines (|Betrag-amount|<=0.01) ranked by date proximity (Bezahldatum/Rechnungsdatum vs line.Date) + supplier-name word overlap (Auftraggeber vs line.Text); MatchAuto when exactly one amount-match within ±5 days, MatchSuggest when amount-matches exist but ambiguous/date-far, MatchNone when none. Best candidate first.

- [ ] **Step 1: Write the failing test**

```go
package core

import "testing"

func TestMatchInvoiceToStatement(t *testing.T) {
	lines := []StatementBooking{
		{Page: 0, LineIdx: 1, Date: "12.01.2026", Text: "Lastschrift Telekom 49,99", Betrag: 49.99},
		{Page: 0, LineIdx: 2, Date: "14.01.2026", Text: "AMAZON WEB SERVICES 78,53", Betrag: 78.53},
		{Page: 0, LineIdx: 3, Date: "20.01.2026", Text: "REWE Markt 78,53", Betrag: 78.53},
	}
	// Unique exact amount + close date + name overlap → Auto.
	auto := CSVRow{Auftraggeber: "AWS", Bezahldatum: "14.01.2026", Bruttobetrag: 78.53, Waehrung: "EUR"}
	// remove the third 78,53 line for the unique case
	kind, cands := MatchInvoiceToStatement(auto, lines[:2])
	if kind != MatchAuto || len(cands) == 0 || cands[0].Line.LineIdx != 2 {
		t.Fatalf("auto: kind=%v cands=%+v", kind, cands)
	}
	// Two lines share 78,53 → ambiguous → Suggest, best (closest date / name) first.
	kind2, cands2 := MatchInvoiceToStatement(auto, lines)
	if kind2 != MatchSuggest || len(cands2) != 2 || cands2[0].Line.LineIdx != 2 {
		t.Errorf("suggest: kind=%v cands=%+v", kind2, cands2)
	}
	// No amount match → None.
	none := CSVRow{Auftraggeber: "X", Bezahldatum: "14.01.2026", Bruttobetrag: 999, Waehrung: "EUR"}
	if k, _ := MatchInvoiceToStatement(none, lines); k != MatchNone {
		t.Errorf("none: kind=%v", k)
	}
	// Foreign currency: EUR debit = round2(89.18/1.1583)+1.54 = 78.53 → matches line 2.
	fx := CSVRow{Auftraggeber: "AWS", Bezahldatum: "14.01.2026", Bruttobetrag: 89.18, Waehrung: "USD", Wechselkurs: 1.1583, Gebuehr: 1.54}
	if !almost(InvoiceEURAmount(fx), 78.53) {
		t.Errorf("InvoiceEURAmount(fx) = %v, want 78.53", InvoiceEURAmount(fx))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run TestMatchInvoiceToStatement`
Expected: FAIL (undefined).

- [ ] **Step 3: Implement**

Create `internal/core/belegabgleich.go`:

```go
package core

import (
	"sort"
	"strings"
	"time"
)

// MatchKind classifies an invoice's reconciliation outcome.
type MatchKind int

const (
	MatchNone MatchKind = iota
	MatchSuggest
	MatchAuto
)

// ScoredLine is a candidate statement line with its rank score (higher = better).
type ScoredLine struct {
	Line  StatementBooking
	Score float64
}

// InvoiceEURAmount returns the amount that should appear on the statement: the
// gross in EUR. For a foreign-currency invoice that is the converted gross plus
// the credit-card fee; otherwise the Bruttobetrag.
func InvoiceEURAmount(row CSVRow) float64 {
	if row.Waehrung != "" && row.Waehrung != "EUR" && row.Wechselkurs > 0 {
		return round2(round2(row.Bruttobetrag/row.Wechselkurs) + row.Gebuehr)
	}
	return round2(row.Bruttobetrag)
}

// MatchInvoiceToStatement ranks the statement lines whose amount matches the
// invoice (within 0.01) by date proximity + supplier-name overlap, and
// classifies the outcome.
func MatchInvoiceToStatement(row CSVRow, lines []StatementBooking) (MatchKind, []ScoredLine) {
	amount := InvoiceEURAmount(row)
	if amount <= 0 {
		return MatchNone, nil
	}
	invDate := row.Bezahldatum
	if invDate == "" {
		invDate = row.Rechnungsdatum
	}
	nameTokens := tokenize(row.Auftraggeber)

	var cands []ScoredLine
	for _, l := range lines {
		if absf(l.Betrag-amount) > 0.01 {
			continue
		}
		days := dayDistance(invDate, l.Date)
		dateScore := 1.0 / (1.0 + float64(days)) // 0 days → 1.0, decays
		nameScore := tokenOverlap(nameTokens, tokenize(l.Text))
		cands = append(cands, ScoredLine{Line: l, Score: dateScore*2 + nameScore})
	}
	if len(cands) == 0 {
		return MatchNone, nil
	}
	sort.SliceStable(cands, func(i, j int) bool { return cands[i].Score > cands[j].Score })

	// Auto: exactly one amount-match, and it is within ±5 days.
	if len(cands) == 1 && dayDistance(invDate, cands[0].Line.Date) <= 5 {
		return MatchAuto, cands
	}
	return MatchSuggest, cands
}

func absf(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// tokenize lowercases and splits a string into word tokens of length >= 3.
func tokenize(s string) []string {
	fields := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && !(r >= 'ä' && r <= 'ÿ')
	})
	var out []string
	for _, f := range fields {
		if len(f) >= 3 {
			out = append(out, f)
		}
	}
	return out
}

// tokenOverlap returns the fraction of a's tokens that appear (as substring) in b.
func tokenOverlap(a, b []string) float64 {
	if len(a) == 0 {
		return 0
	}
	hit := 0
	for _, t := range a {
		for _, u := range b {
			if strings.Contains(u, t) || strings.Contains(t, u) {
				hit++
				break
			}
		}
	}
	return float64(hit) / float64(len(a))
}

// dayDistance returns the absolute day difference between two DD.MM.YYYY (or
// DD.MM.) dates; a missing/short year is treated as the other date's year. A
// huge number is returned when either is unparseable.
func dayDistance(a, b string) int {
	ta, oka := parseFlexDate(a, b)
	tb, okb := parseFlexDate(b, a)
	if !oka || !okb {
		return 9999
	}
	d := ta.Sub(tb).Hours() / 24
	if d < 0 {
		d = -d
	}
	return int(d + 0.5)
}

// parseFlexDate parses "DD.MM.YYYY" or "DD.MM." (taking the year from other).
func parseFlexDate(s, other string) (time.Time, bool) {
	parts := strings.Split(strings.TrimSpace(s), ".")
	if len(parts) < 2 {
		return time.Time{}, false
	}
	year := ""
	if len(parts) >= 3 {
		year = strings.TrimSpace(parts[2])
	}
	if year == "" {
		op := strings.Split(strings.TrimSpace(other), ".")
		if len(op) >= 3 {
			year = strings.TrimSpace(op[2])
		}
	}
	if len(year) == 2 {
		year = "20" + year
	}
	if year == "" {
		return time.Time{}, false
	}
	t, err := time.Parse("2.1.2006", parts[0]+"."+parts[1]+"."+year)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run TestMatchInvoiceToStatement && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/core/belegabgleich.go internal/core/belegabgleich_test.go
git commit -m "Add invoice↔statement matching engine"
```

---

### Task 3: Belegabgleich dialog (UI)

**Files:**
- Create: `internal/ui/belegabgleichview.go`
- Modify: `internal/ui/app.go` (menu item)
- Test: none (UI; covered by build + manual). 

**Interfaces:**
- Consumes: `core.MatchInvoiceToStatement`, `core.ParseStatementBookings`, `a.collectInvoiceRows`, `a.statementFolder`/`a.listStatements`, `a.dbRepo.Update`, `core.BuchungRef`, `a.settings.BankAccounts` (account types).
- Produces: `func (a *App) showBelegabgleich()` from a "Belegabgleich" menu item: auto-links unambiguous matches, lists the suggestions for confirmation, persists `BuchungRef` + reloads.

- [ ] **Step 1: Build the action**

Create `internal/ui/belegabgleichview.go` with `showBelegabgleich()`:
- Collect the current period's invoices: `rows := a.collectInvoiceRows(a.currentYear, int(a.currentMonth), a.currentYear, int(a.currentMonth))` (year mode optional later).
- Build a helper `accountType(name string) string` from `a.settings.BankAccounts`.
- For each invoice with `BuchungRef == ""` whose `Bankkonto`'s account type is bank/creditcard (skip cash + already-linked): gather the candidate statement lines of that Bankkonto. Build them by iterating `a.listStatements(row.Bankkonto)`, `ParseStatementBookings(filepath.Join(a.statementFolder(row.Bankkonto), name))`, keeping each line's source `name` alongside it (a small `{file string; line core.StatementBooking}` slice).
- Run `core.MatchInvoiceToStatement(row, linesOfThatAccount)`:
  - **MatchAuto:** set `row.BuchungRef = core.BuchungRef{StatementFilename: <file of top line>, Page: top.Page, LineIdx: top.LineIdx}.String()` and `a.dbRepo.Update(row.Jahr, row.Monat, row.Dateiname, row)`. Count autoLinked.
  - **MatchSuggest:** collect `{row, topCandidate, file}` for the dialog.
  - **MatchNone:** ignore.
- Show a dialog: a header `a.bundle.T("reconcile.summary", autoLinked, len(suggestions))`, then a scrollable list of suggestions, each row: `"<Auftraggeber> <Betrag> → <candidate.Display()> (<file>)"` + a **Bestätigen** button that sets `row.BuchungRef` for that candidate + `dbRepo.Update` + removes the row from the list. (Keep it simple: one top candidate per suggestion; "andere wählen" can come later.)
- After the dialog (and after auto-links), call `a.loadInvoices()` to refresh the table status.

To track which statement FILE each line came from, since `core.StatementBooking` has no filename, build a small local type in this file:

```go
type stmtLine struct {
	File string
	Line core.StatementBooking
}
```

and a helper that returns `([]core.StatementBooking, map back to file)` — simplest: keep a parallel slice and, after matching returns a `ScoredLine`, find its file by matching (Page, LineIdx, Date) within the account's parsed set, or carry the file by parsing one statement at a time and matching per-file then keeping the best across files.

(Pragmatic approach: match per statement file — for each invoice, loop its account's files, `ParseStatementBookings(file)`, `MatchInvoiceToStatement(row, lines)`, and keep the best outcome across files together with that file name. This avoids a reverse lookup.)

- [ ] **Step 2: Menu item + i18n**

In `internal/ui/app.go`, next to the other menu items, add `fyne.NewMenuItem("Belegabgleich", func() { a.showBelegabgleich() })`. Add i18n keys: `reconcile.title` (de "Belegabgleich"/en "Reconciliation"), `reconcile.summary` (de "%d automatisch verknüpft, %d Vorschläge."/en "%d auto-linked, %d suggestions."), `reconcile.confirm` (de "Bestätigen"/en "Confirm"), `reconcile.none` (de "Keine offenen Vorschläge."/en "No open suggestions.").

- [ ] **Step 3: Build + vet + test + manual smoke**

Run: `go build ./... && go vet ./internal/ui/ && go test ./...`
Expected: clean. Validate both JSONs.

- [ ] **Step 4: Commit**

```bash
git add internal/ui/belegabgleichview.go internal/ui/app.go assets/i18n/de.json assets/i18n/en.json
git commit -m "Add Belegabgleich dialog: auto-link + confirm suggestions"
```

---

### Task 4: Link-status indicator in the invoice table

**Files:**
- Modify: `internal/ui/table.go`
- Test: none (UI; covered by build + manual). 

**Interfaces:**
- Consumes: `row.BuchungRef`, `row.Bankkonto` + account type.
- Produces: the existing `BuchungRef` table column renders a status glyph — 🟢/✓ when `BuchungRef` is set, ⚪/– when not (and a neutral marker for cash accounts).

- [ ] **Step 1: Render a status glyph**

In `internal/ui/table.go`, find the cell renderer for the `"BuchungRef"` column (currently `case "BuchungRef": return row.BuchungRef` ~line 904). Replace the raw ref with a compact status that still conveys the link:

```go
	case "BuchungRef":
		if row.BuchungRef != "" {
			return "✓ " + core.ParseBuchungRef(row.BuchungRef).Display() // linked (green-ish check)
		}
		if a.isCashAccount(row.Bankkonto) {
			return "—" // cash: not statement-matched
		}
		return "○" // unlinked
```

The parse helper is `core.ParseBuchungRef(s string) core.BuchungRef` (confirmed in `internal/core/booking_ref.go:45`). Add a small `isCashAccount(name string) bool` method on `*App` (or `*InvoiceTable`, wherever this cell renderer lives — it's a method on the table that has `it.app`): loop `a.settings.BankAccounts` and return true when the named account has `AccountType == core.AccountTypeCash` (mirror the cash check already used in `app.go:199` / `kassenbuchview.go:26`). Keep it text (no new icon assets) so the existing cell rendering works; the "✓" is the green-arrow equivalent. If the table cell API makes per-cell green color trivial, optionally color the "✓" green; otherwise plain text is fine.

- [ ] **Step 2: Build + vet + test**

Run: `go build ./... && go vet ./internal/ui/ && go test ./...`
Expected: clean.

- [ ] **Step 3: Commit**

```bash
git add internal/ui/table.go
git commit -m "Show link status (✓/○/—) in the Buchung column"
```

---

## Self-Review

- **Spec coverage:** auto-link unambiguous exact matches (green), suggest ambiguous for one-click confirm, none for unmatched (Tasks 2/3); amount-based matching incl. foreign-currency EUR debit (Task 2 `InvoiceEURAmount`), needing the parsed line amount (Task 1); status indicator in the table (Task 4). Cash accounts excluded. Matches the user's request (Beleg ↔ Auszug-Position, grüner Pfeil bei Verknüpfung).
- **Placeholder scan:** Tasks 1/2 fully coded + tested; UI tasks reference concrete anchors (collectInvoiceRows, listStatements/statementFolder/ParseStatementBookings, dbRepo.Update, the BuchungRef column cell) with a pragmatic per-file matching approach to carry the statement filename.
- **Type consistency:** `StatementBooking.Betrag`, `ParseLineAmount`, `InvoiceEURAmount`, `MatchKind`/`ScoredLine`/`MatchInvoiceToStatement`, `BuchungRef.String()`. Consistent.
- **Data integrity:** amount is a hard filter (0.01); auto only on a unique amount-match within ±5 days; auto never overwrites an existing link; linking reuses the persisted `BuchungRef` (no new storage); reverse (statement line → invoice) computed, not stored.
- **Out of scope (later):** cash-book matching; "choose a different line" in the dialog; Claude name-matching for ambiguous cases (user chose heuristic); colored/icon status cell; showing the linked invoice inside the Konten statement view; unlink action.
