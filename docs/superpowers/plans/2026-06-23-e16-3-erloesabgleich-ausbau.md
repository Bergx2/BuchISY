# E16.3 — Erlös-Abgleich Ausbau: gruppiert/Teilzahlung + Alias + Claude

> REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Steps use `- [ ]`.

**Goal:** Bring the expense-side reconciliation intelligence to the revenue side: grouped payments (one customer credit covering N outgoing invoices, n:1), partial payments (1:n), customer-alias learning, and Claude re-ranking for close calls.

**Tech:** Go 1.25, Fyne. `internal/core`, `internal/ui`. Branch `feat/e16-3-erloesabgleich-ausbau`.

## Global Constraints

- `go build ./...`, `go test ./...`. Commit per task. Co-author trailer: `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`.
- Revenue matches against INCOMING credit lines (`IstGutschrift==true`); expense behaviour must stay byte-identical (existing `belegabgleich_test.go` passes unchanged).
- Amount = `InvoiceEURAmount(row)` (gross received).

---

### Task 1: Core — grouped + partial for credit lines (DRY)

**Files:** `internal/core/belegabgleich.go`, test `internal/core/belegabgleich_test.go`.

**Interfaces:** keep `FindGroupedPayments(invoices, lines, cfg)` and `PartialPaymentLines(row, lines)` with identical behaviour; ADD `FindGroupedRevenuePayments(invoices, lines, cfg)` and `RevenuePartialPaymentLines(row, lines)` that work on credit lines.

- [ ] **Step 1: Test** (append):

```go
func TestFindGroupedRevenuePayments(t *testing.T) {
	cfg := DefaultMatchConfig()
	// One credit (Gutschrift) of 300 = two outgoing invoices 100 + 200.
	invs := []CSVRow{
		{Dateiname: "a.pdf", Ausgangsrechnung: true, Bruttobetrag: 100, Rechnungsdatum: "10.01.2026"},
		{Dateiname: "b.pdf", Ausgangsrechnung: true, Bruttobetrag: 200, Rechnungsdatum: "11.01.2026"},
	}
	credit := []StatementBooking{{Page: 0, LineIdx: 1, Date: "12.01.2026", Betrag: 300, IstGutschrift: true}}
	g := FindGroupedRevenuePayments(invs, credit, cfg)
	if len(g) != 1 || len(g[0].Dateinamen) != 2 {
		t.Fatalf("want one 2-invoice group on the credit, got %+v", g)
	}
	// A DEBIT line of 300 must NOT produce a revenue group.
	debit := []StatementBooking{{Page: 0, LineIdx: 2, Date: "12.01.2026", Betrag: 300, IstGutschrift: false}}
	if g := FindGroupedRevenuePayments(invs, debit, cfg); len(g) != 0 {
		t.Errorf("debit line must not group revenue: %+v", g)
	}
}

func TestRevenuePartialPaymentLines(t *testing.T) {
	row := CSVRow{Ausgangsrechnung: true, Teilzahlung: true, Bruttobetrag: 1000, Rechnungsdatum: "10.01.2026"}
	lines := []StatementBooking{
		{Page: 0, LineIdx: 1, Date: "12.01.2026", Betrag: 400, IstGutschrift: true},  // partial credit → candidate
		{Page: 0, LineIdx: 2, Date: "12.01.2026", Betrag: 400, IstGutschrift: false}, // debit → excluded
	}
	c := RevenuePartialPaymentLines(row, lines)
	if len(c) != 1 || !c[0].Line.IstGutschrift {
		t.Fatalf("want one credit partial line, got %+v", c)
	}
}
```

- [ ] **Step 2:** run → fail.
- [ ] **Step 3:** Refactor: extract the body of `FindGroupedPayments` into `findGroupedPayments(invoices, lines, cfg, wantCredit bool)`, changing the guard `if l.IstGutschrift || l.Betrag <= 0` → `if l.IstGutschrift != wantCredit || l.Betrag <= 0`. Extract `PartialPaymentLines` body into `partialPaymentLines(row, lines, wantCredit bool)`, changing `if l.IstGutschrift { continue }` → `if l.IstGutschrift != wantCredit { continue }`. Then:

```go
func FindGroupedPayments(invoices []CSVRow, lines []StatementBooking, cfg MatchConfig) []GroupMatch {
	return findGroupedPayments(invoices, lines, cfg, false)
}
func FindGroupedRevenuePayments(invoices []CSVRow, lines []StatementBooking, cfg MatchConfig) []GroupMatch {
	return findGroupedPayments(invoices, lines, cfg, true)
}
func PartialPaymentLines(row CSVRow, lines []StatementBooking) []ScoredLine {
	return partialPaymentLines(row, lines, false)
}
func RevenuePartialPaymentLines(row CSVRow, lines []StatementBooking) []ScoredLine {
	return partialPaymentLines(row, lines, true)
}
```

- [ ] **Step 4:** run → pass + full core (existing grouped/partial expense tests unchanged). Commit `E16.3: grouped + partial payment matching for credit lines (revenue)`.

---

### Task 2: UI — wire grouped/partial + alias + Claude into the Erlös-Abgleich

**Files:** `internal/ui/erloesabgleichview.go`.

**Context:** READ `internal/ui/belegabgleichview.go` (the expense dialog) — it has the full mechanism for: (a) rendering grouped (n:1) results (`FindGroupedPayments` → confirm a group link), (b) partial-payment rows (`PartialPaymentLines` → dropdown + confirm), (c) `a.statementAliases.Learn(...)` + `.Save()` alias learning on confirm/auto-link, (d) `a.anthropicExtractor.RankStatementLine(...)` Claude re-ranking of close-call suggestions (Claude mode only, non-fatal). READ `internal/ui/erloesabgleichview.go` (the streamlined revenue dialog from E15.4) — it currently does only single-line auto-link + confirm.

Mirror the four mechanisms into `showErloesAbgleich`, using the REVENUE core functions:
- [ ] **Step 1:** After the single-line matching pass, also run `core.FindGroupedRevenuePayments(unlinkedAusgangsrechnungen, creditLines, cfg)` and render each group as a confirm-row ("N Rechnungen → Gutschrift …"); on confirm, link each invoice in the group to that credit line (BuchungRef) + settle via `WithSettlementAccount` (as E16.2 does) + persist.
- [ ] **Step 2:** For invoices flagged `Teilzahlung`, use `core.RevenuePartialPaymentLines(row, creditLines)` to offer partial-credit candidates (dropdown + confirm), mirroring the expense partial UI.
- [ ] **Step 3:** Add alias learning: on every confirm/auto-link, `a.statementAliases.Learn(row.Auftraggeber, matchedLine.Text)` + `a.statementAliases.Save()` (mirror the expense dialog's calls). The matchConfig already injects learned aliases into scoring.
- [ ] **Step 4:** Add Claude re-ranking for close-call single-line suggestions: when `a.settings.ProcessingMode == "claude"` and the top-2 candidates are within a small score margin, call `a.anthropicExtractor.RankStatementLine(...)` (same signature/usage as the expense dialog) to reorder — non-fatal on error. Mirror the expense guard.
- [ ] **Step 5:** `go build ./... && go test ./...`. Commit `E16.3: Erlös-Abgleich grouped/partial + alias learning + Claude ranking`.

Keep the existing single-line auto-link + the E16.2 settlement intact. If any of the four mechanisms cannot be mirrored cleanly within reasonable scope, implement grouped + alias (highest value) and leave a clear `// TODO E16.3` note for partial/Claude — but prefer all four.

## Self-Review

Coverage: C#4 (grouped + partial) → Task 1 + Task 2 steps 1-2; C#5 (alias + Claude) → Task 2 steps 3-4. Expense path untouched (new functions are additive wrappers; the expense dialog is a separate file). DRY: one shared body per function behind `wantCredit`.
