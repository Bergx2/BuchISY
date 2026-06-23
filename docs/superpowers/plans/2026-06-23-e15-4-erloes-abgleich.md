# E15.4 — Erlös-Abgleich (Ausgangsrechnungen ↔ Bank-Gutschriften)

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Steps use `- [ ]`.

**Goal:** Reconcile outgoing invoices (Ausgangsrechnungen) against INCOMING bank credits (Gutschriften, `IstGutschrift==true`) — the mirror of the existing expense Belegabgleich, which excludes credits.

**Architecture:** DRY-refactor the matcher to a shared core with a `wantCredit` flag; add `MatchRevenueToStatement`. A new focused dialog reuses the statement-parse + `BuchungRef` link mechanism from `belegabgleichview.go`, filtered to Ausgangsrechnungen and credit lines.

**Tech Stack:** Go 1.25, Fyne. `internal/core`, `internal/ui`.

## Global Constraints

- Go 1.25; `go build ./...`, `go test ./...`. Branch `feat/e15-4-erloes-abgleich`. Commit per task. Co-author trailer: `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`.
- The expense matcher behaviour MUST stay byte-identical (existing `belegabgleich_test.go` must pass unchanged). The refactor only extracts the body + inverts ONE line behind a flag.
- Revenue match amount = `InvoiceEURAmount(row)` (gross received). Match only credit lines (`IstGutschrift==true`).
- Links reuse `CSVRow.BuchungRef` = `core.BuchungRef{StatementFilename,Page,LineIdx}.String()`, persisted via `a.dbRepo.Update(jahr,monat,dateiname,row)` — same as the expense dialog. The table ✓/○/— status already keys on BuchungRef.
- i18n keys in BOTH de.json + en.json, valid JSON, BOM-free.

---

### Task 1: Core — `MatchRevenueToStatement` (DRY refactor)

**Files:**
- Modify: `internal/core/belegabgleich.go`
- Test: `internal/core/belegabgleich_test.go`

**Interfaces:**
- Produces: `MatchRevenueToStatement(row CSVRow, lines []StatementBooking, cfg MatchConfig) (MatchKind, []ScoredLine)`. `MatchInvoiceToStatement` keeps its exact signature + behaviour.

- [ ] **Step 1: Write the failing test** — append to `belegabgleich_test.go`:

```go
func TestMatchRevenueToStatement(t *testing.T) {
	cfg := DefaultMatchConfig()
	row := CSVRow{Auftraggeber: "Acme Ltd", Ausgangsrechnung: true, Bruttobetrag: 1190, Rechnungsdatum: "10.01.2026"}
	lines := []StatementBooking{
		{Page: 0, LineIdx: 1, Date: "12.01.2026", Text: "Acme Ltd Zahlung", Betrag: 1190, IstGutschrift: true},  // incoming credit → match
		{Page: 0, LineIdx: 2, Date: "12.01.2026", Text: "Acme Ltd", Betrag: 1190, IstGutschrift: false},          // debit → must NOT match
	}
	kind, cands := MatchRevenueToStatement(row, lines, cfg)
	if kind == MatchNone || len(cands) != 1 {
		t.Fatalf("want one credit-line match, got kind=%v cands=%+v", kind, cands)
	}
	if !cands[0].Line.IstGutschrift || cands[0].Line.LineIdx != 1 {
		t.Errorf("matched the wrong line: %+v", cands[0].Line)
	}
	// And the expense matcher must still ignore the credit line:
	exKind, exCands := MatchInvoiceToStatement(row, lines, cfg)
	for _, c := range exCands {
		if c.Line.IstGutschrift {
			t.Errorf("expense matcher must never return a credit line: %+v", c)
		}
	}
	_ = exKind
}
```

- [ ] **Step 2: Run → fail**: `go test ./internal/core/ -run TestMatchRevenueToStatement` (`MatchRevenueToStatement` undefined).

- [ ] **Step 3: Refactor** in `belegabgleich.go`: rename the current `MatchInvoiceToStatement` body into an unexported `matchToStatement(row CSVRow, lines []StatementBooking, cfg MatchConfig, wantCredit bool) (MatchKind, []ScoredLine)`, changing ONLY the credit guard from `if l.IstGutschrift { continue }` to `if l.IstGutschrift != wantCredit { continue }`. Then:

```go
// MatchInvoiceToStatement matches an expense invoice to statement DEBIT lines
// (credits excluded). cfg controls window, foreign tolerance, alias boosts.
func MatchInvoiceToStatement(row CSVRow, lines []StatementBooking, cfg MatchConfig) (MatchKind, []ScoredLine) {
	return matchToStatement(row, lines, cfg, false)
}

// MatchRevenueToStatement matches an outgoing invoice (Ausgangsrechnung) to
// incoming bank CREDIT lines (Gutschriften) by amount + date + customer-name
// overlap — the mirror of MatchInvoiceToStatement.
func MatchRevenueToStatement(row CSVRow, lines []StatementBooking, cfg MatchConfig) (MatchKind, []ScoredLine) {
	return matchToStatement(row, lines, cfg, true)
}
```

Keep the doc comment update on `matchToStatement` (it now matches debit OR credit per `wantCredit`).

- [ ] **Step 4: Run → pass**: `go test ./internal/core/` (full package — `TestMatchRevenueToStatement` AND all existing belegabgleich tests must pass).
- [ ] **Step 5: Commit** `E15.4: MatchRevenueToStatement (mirror matcher on credit lines)`.

---

### Task 2: UI — Erlös-Abgleich dialog + menu

**Files:**
- Create: `internal/ui/erloesabgleichview.go`
- Modify: `internal/ui/app.go` (menu ~line 730, after "Belegabgleich")
- Modify: `assets/i18n/de.json`, `en.json`

**Interfaces:**
- Consumes: `core.MatchRevenueToStatement`, and the SAME helpers the expense dialog uses: `a.listStatements(acct)`, `a.statementFolder(acct)`, `core.ParseStatementBookings(path)`, `a.matchConfig()`, `core.BuchungRef{...}.String()`, `a.dbRepo.Update`, `a.loadInvoices()`. The package-private helper types `stmtLine` / `scoredWithFile` (in `belegabgleichview.go`) are in the same package — reuse them.

- [ ] **Step 1:** READ `internal/ui/belegabgleichview.go` `showBelegabgleich` (lines ~83–300) to mirror the parse-cache + per-file match + greedy auto-link + BuchungRef-persist + suggestion-row mechanism. Then create `func (a *App) showErloesAbgleich()` in `internal/ui/erloesabgleichview.go`, a STREAMLINED version that:
  - Lists rows for the current period (`a.dbRepo.List(year, month)`), keeping only **`row.Ausgangsrechnung && row.BuchungRef == "" && accountType(row.Bankkonto) == core.AccountTypeBank`** (revenue arrives on a bank account).
  - Builds the same parse-once cache per `row.Bankkonto` (statements parsed via `core.ParseStatementBookings`).
  - For each invoice, runs `core.MatchRevenueToStatement(row, linesForFile, cfg)` per statement file, accumulating candidates with their file (reuse `scoredWithFile`); downgrade to Suggest if 2+ files auto-match (same guard as expense).
  - Greedy auto-links `MatchAuto` results, claiming each credit line at most once (`claimed` map keyed by `BuchungRef.String()`); writes `row.BuchungRef` + `a.dbRepo.Update(...)`.
  - Renders the remaining `MatchSuggest` results as confirm-rows (label = customer + amount + matched credit line; a confirm button that sets `BuchungRef` + `Update`; a `widget.Select` candidate dropdown when ≥2 candidates, updating the label on change — mirror the expense dialog's suggestion row, including the E12 fix that the label updates with the dropdown).
  - Shows the results in a `dialog.NewCustom` (like `showBelegabgleich`), and calls `a.loadInvoices()` at the end so linked rows show ✓.
  - SCOPE: skip grouped/partial-payment and Claude re-ranking for this first version (note it in a comment). Auto-link + confirm-suggestions is the deliverable.
- [ ] **Step 2:** Add the menu item in `app.go` right after the "Belegabgleich" item (~line 732): `fyne.NewMenuItem("Erlös-Abgleich", func() { a.showErloesAbgleich() }),`.
- [ ] **Step 3:** i18n (de + en): `erloesabgleich.title` ("Erlös-Abgleich" / "Revenue reconciliation"), `erloesabgleich.heading`, `erloesabgleich.autolinked` ("%d automatisch verknüpft" / "%d auto-linked"), `erloesabgleich.none` ("Keine offenen Ausgangsrechnungen mit passender Gutschrift" / "No open outgoing invoices with a matching credit"), `erloesabgleich.confirm` ("Verknüpfen" / "Link"). Reuse existing keys (`common.close`) where possible.
- [ ] **Step 4:** `go build ./... && go test ./...` → pass. Commit `E15.4: Erlös-Abgleich dialog + menu`.

---

## Self-Review

**Spec coverage:** core mirror matcher → Task 1; revenue reconciliation UI → Task 2. Grouped/partial/Claude-ranking explicitly deferred (the expense dialog's extras), noted.

**Placeholder scan:** complete code for Task 1; Task 2 references the existing `showBelegabgleich` mechanism the implementer reads + mirrors (same package, same helpers).

**Type consistency:** `MatchRevenueToStatement(row, lines, cfg)` matches `MatchInvoiceToStatement`'s signature. `BuchungRef`, `stmtLine`, `scoredWithFile`, `MatchConfig` reused unchanged.

**Regression guard:** the refactor must leave `MatchInvoiceToStatement` behaviour identical — the existing `belegabgleich_test.go` is the gate.
